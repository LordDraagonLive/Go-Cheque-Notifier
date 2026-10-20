package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingSender struct {
	messages []string
	failAt   int
}

func (sender *recordingSender) Send(_ context.Context, body string) error {
	sender.messages = append(sender.messages, body)
	if sender.failAt == len(sender.messages) {
		return errors.New("delivery failed")
	}
	return nil
}

func fixedNow(t *testing.T, value string) time.Time {
	t.Helper()
	now, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return now
}

// These golden files define the message templates, including final newlines.
func TestFourMessageContract(t *testing.T) {
	data := fixtureReader{
		receivedSheet: {
			{"1002", "Nimal Silva", "Rs12,500.50", "City Bank", "2026-10-21"},
			{"1001", "John Doe", "Rs5,000", "ABC Bank", "2026-10-24"},
			{"1003", "Asha Perera", "Rs25.00", "ABC Bank", "2026-10-24"},
		},
		issuedSheet: {
			{"2002", "Jane Smith", "Rs7,500", "XYZ Bank", "2026-10-21"},
			{"2001", "Office Supplies", "Rs3,250.00", "XYZ Bank", "2026-10-24"},
		},
	}
	sender := &recordingSender{}
	if err := run(context.Background(), data, sender, fixedNow(t, "2026-10-21T08:00:00+05:30")); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 4 {
		t.Fatalf("got %d messages, want 4", len(sender.messages))
	}
	for index, file := range []string{"received-future.txt", "received-today.txt", "issued-future.txt", "issued-today.txt"} {
		want, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if got := sender.messages[index]; got != string(want) {
			t.Errorf("message %d differs from %s\ngot:  %q\nwant: %q", index, file, got, want)
		}
	}
}

func TestSriLankaDateBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, now, today, future string
	}{
		{"before local midnight", "2026-10-20T18:29:59Z", "2026-10-20", "2026-10-23"},
		{"at local midnight", "2026-10-20T18:30:00Z", "2026-10-21", "2026-10-24"},
		{"year rollover", "2026-12-30T20:00:00Z", "2026-12-31", "2027-01-03"},
		{"leap year", "2028-02-26T20:00:00Z", "2028-02-27", "2028-03-01"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := fixtureReader{receivedSheet: {
				{"today", "Payee", "Rs1", "Bank", tt.today},
				{"future", "Payee", "Rs2", "Bank", tt.future},
			}, issuedSheet: {}}
			sender := &recordingSender{}
			if err := run(context.Background(), data, sender, fixedNow(t, tt.now)); err != nil {
				t.Fatal(err)
			}
			if len(sender.messages) != 2 || !strings.Contains(sender.messages[0], "Cheque No : future\n") ||
				!strings.Contains(sender.messages[1], "Cheque No : today\n") {
				t.Fatalf("wrong date groups: %q", sender.messages)
			}
		})
	}
}

func TestSkippedRowsAndNoMatches(t *testing.T) {
	data := fixtureReader{receivedSheet: {
		{}, {"incomplete", "row", "bad amount", "bad date"},
		{"past", "Payee", "Rs1", "Bank", "2026-10-20"},
		{"tomorrow", "Payee", "Rs2", "Bank", "2026-10-22"},
		{"two days", "Payee", "Rs3", "Bank", "2026-10-23"},
		{"four days", "Payee", "Rs4", "Bank", "2026-10-25"},
	}, issuedSheet: {}}
	sender := &recordingSender{}
	if err := run(context.Background(), data, sender, fixedNow(t, "2026-10-21T08:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sender.messages, []string{noChequesText}) {
		t.Fatalf("unexpected messages: %q", sender.messages)
	}
}

func TestFutureOnlySuppressesNoChequesMessage(t *testing.T) {
	data := fixtureReader{receivedSheet: {}, issuedSheet: {{"future", "Payee", "Rs1", "Bank", "2026-10-24"}}}
	sender := &recordingSender{}
	if err := run(context.Background(), data, sender, fixedNow(t, "2026-10-21T08:00:00Z")); err != nil {
		t.Fatal(err)
	}
	if len(sender.messages) != 1 || !strings.HasPrefix(sender.messages[0], "🔔 *Reminder*: Ensure Funds") {
		t.Fatalf("unexpected messages: %q", sender.messages)
	}
}

func TestMalformedCompleteRowsFailEvenWhenNotDue(t *testing.T) {
	for _, row := range [][]string{
		{"1001", "Payee", "Rs1", "Bank", "21-Apr-2025"},
		{"1001", "Payee", "Rs1", "Bank", "2026-02-30"},
		{"1001", "Payee", "LKR 5,000", "Bank", "2025-04-21"},
		{"1001", "Payee", "Rs 5,000", "Bank", "2025-04-21"},
	} {
		sender := &recordingSender{}
		err := run(context.Background(), fixtureReader{receivedSheet: {row}, issuedSheet: {}}, sender, time.Now())
		if err == nil || !strings.Contains(err.Error(), "Cheques Received row 2") || len(sender.messages) != 0 {
			t.Fatalf("row %q: got error %v and messages %q", row, err, sender.messages)
		}
	}
}

type observedReader struct {
	data  fixtureReader
	reads []string
}

func (reader *observedReader) ReadSheet(ctx context.Context, sheet string) ([][]string, error) {
	reader.reads = append(reader.reads, sheet)
	return reader.data.ReadSheet(ctx, sheet)
}

func TestFailureStopsFurtherReadsAndSends(t *testing.T) {
	reader := &observedReader{data: fixtureReader{receivedSheet: {
		{"1001", "Payee", "Rs1", "Bank", "2026-10-24"},
		{"1002", "Payee", "Rs1", "Bank", "2026-10-21"},
	}, issuedSheet: {}}}
	sender := &recordingSender{failAt: 1}
	err := run(context.Background(), reader, sender, fixedNow(t, "2026-10-21T08:00:00Z"))
	if err == nil || len(sender.messages) != 1 || !reflect.DeepEqual(reader.reads, []string{receivedSheet}) {
		t.Fatalf("got error %v, reads %q, messages %q", err, reader.reads, sender.messages)
	}
}

func TestSecondSheetFailureKeepsFirstSheetDelivery(t *testing.T) {
	data := fixtureReader{receivedSheet: {{"1001", "Payee", "Rs1", "Bank", "2026-10-21"}}}
	sender := &recordingSender{}
	err := run(context.Background(), data, sender, fixedNow(t, "2026-10-21T08:00:00Z"))
	if err == nil || len(sender.messages) != 1 {
		t.Fatalf("got error %v, messages %q", err, sender.messages)
	}
}

func TestOfflinePreviewAndCLIValidation(t *testing.T) {
	var output, diagnostics bytes.Buffer
	err := execute(context.Background(), []string{"-dry-run", "-fixture", "testdata/demo.json", "-now", "2026-10-21T08:00:00+05:30"}, &output, &diagnostics)
	if err != nil || strings.Count(output.String(), "--------------------------------------------------") != 4 {
		t.Fatalf("preview error %v, output %q", err, output.String())
	}
	for _, args := range [][]string{
		{"-fixture", "testdata/demo.json"}, {"-now", "2026-10-21T08:00:00Z"},
		{"-dry-run", "-now", "invalid"}, {"unexpected"},
		{"-dry-run", "-fixture", "does-not-exist.json"},
	} {
		if err := execute(context.Background(), args, &output, &diagnostics); err == nil {
			t.Errorf("arguments %q should fail", args)
		}
	}
	if err := execute(context.Background(), []string{"-help"}, &output, &diagnostics); err != nil {
		t.Fatal(err)
	}
}
