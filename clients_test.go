package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConfig() config {
	return config{
		ClientID: "client-id", ClientSecret: "client-secret", RefreshToken: "refresh-token", SpreadsheetID: "sheet-id",
		TwilioAuthToken: "auth-token", TwilioAccountSID: "ACtest", TwilioFromNumber: "+14155238886", TwilioToNumber: "+94770000000",
	}
}

func TestGoogleAndTwilioHTTPContract(t *testing.T) {
	var mu sync.Mutex
	var events, messages []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			events = append(events, "token")
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Method != "POST" || r.Form.Get("grant_type") != "refresh_token" ||
				r.Form.Get("client_id") != "client-id" || r.Form.Get("client_secret") != "client-secret" ||
				r.Form.Get("refresh_token") != "refresh-token" {
				t.Errorf("unexpected OAuth request: %s %v", r.Method, r.Form)
			}
			fmt.Fprint(w, `{"access_token":"access-token","expires_in":3600}`)
		case "/sheets/sheet-id/values/Cheques Received!B2:F":
			events = append(events, "received")
			if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer access-token" || r.URL.RawQuery != "" {
				t.Errorf("unexpected Sheets request: %s %v", r.Method, r.URL)
			}
			fmt.Fprint(w, `{"values":[[1002,"Nimal Silva","Rs12,500.50","City Bank","2026-10-21"],[1001,"John Doe","Rs5,000","ABC Bank","2026-10-24"],[1003,"Asha Perera","Rs25.00","ABC Bank","2026-10-24"]]}`)
		case "/sheets/sheet-id/values/Cheques Issued!B2:F":
			events = append(events, "issued")
			if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer access-token" {
				t.Errorf("unexpected issued-sheet request")
			}
			fmt.Fprint(w, `{"values":[[2002,"Jane Smith","Rs7,500","XYZ Bank","2026-10-21"],[2001,"Office Supplies","Rs3,250.00","XYZ Bank","2026-10-24"]]}`)
		case "/twilio/ACtest/Messages.json":
			events = append(events, "send")
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			user, password, ok := r.BasicAuth()
			if r.Method != "POST" || !ok || user != "ACtest" || password != "auth-token" ||
				r.Form.Get("To") != "whatsapp:+94770000000" || r.Form.Get("From") != "whatsapp:+14155238886" ||
				r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("unexpected Twilio request")
			}
			messages = append(messages, r.Form.Get("Body"))
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"sid":"SMtest","status":"queued"}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	api := newAPIClient(testConfig())
	api.tokenURL, api.sheetsURL, api.twilioURL = server.URL+"/token", server.URL+"/sheets", server.URL+"/twilio"
	if err := run(context.Background(), api, api, fixedNow(t, "2026-10-21T08:00:00+05:30")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(events, []string{"token", "received", "send", "send", "issued", "send", "send"}) {
		t.Fatalf("unexpected request order: %q", events)
	}
	for index, file := range []string{"received-future.txt", "received-today.txt", "issued-future.txt", "issued-today.txt"} {
		want, err := os.ReadFile("testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		if messages[index] != string(want) {
			t.Errorf("Twilio body %d differs from %s", index, file)
		}
	}
}

func TestAPIErrorResponses(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		stage  string
	}{
		{"OAuth failure", 401, `{"error":"secret-value"}`, "token"},
		{"OAuth bad JSON", 200, `not json secret-value`, "token"},
		{"OAuth missing token", 200, `{"expires_in":3600}`, "token"},
		{"OAuth missing expiry", 200, `{"access_token":"secret-value"}`, "token"},
		{"Sheets failure", 403, `{"error":"secret-value"}`, "sheets"},
		{"Sheets invalid JSON", 200, `not json secret-value`, "sheets"},
		{"Sheets invalid cell", 200, `{"values":[[true]]}`, "sheets"},
		{"Sheets null cell", 200, `{"values":[[null]]}`, "sheets"},
		{"Twilio failure", 429, `{"message":"secret-value"}`, "twilio"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			api := newAPIClient(testConfig())
			api.tokenURL, api.sheetsURL, api.twilioURL = server.URL, server.URL, server.URL
			api.accessToken, api.tokenExpires = "cached-token", time.Now().Add(time.Hour)
			var err error
			switch tt.stage {
			case "token":
				api.tokenExpires = time.Now().Add(-time.Hour)
				_, err = api.token(context.Background())
			case "sheets":
				_, err = api.ReadSheet(context.Background(), receivedSheet)
			case "twilio":
				err = api.Send(context.Background(), "test message")
			}
			server.Close()
			if err == nil || strings.Contains(err.Error(), "secret-value") || calls != 1 {
				t.Fatalf("got error %v after %d calls", err, calls)
			}
		})
	}
}

func TestMissingSheetValuesMeansEmptySheet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{}`) }))
	defer server.Close()
	api := newAPIClient(testConfig())
	api.sheetsURL, api.accessToken, api.tokenExpires = server.URL, "cached-token", time.Now().Add(time.Hour)
	rows, err := api.ReadSheet(context.Background(), receivedSheet)
	if err != nil || len(rows) != 0 {
		t.Fatalf("got rows %v and error %v", rows, err)
	}
}

func TestCancelledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	api := newAPIClient(testConfig())
	if _, err := api.ReadSheet(ctx, receivedSheet); err == nil {
		t.Fatal("cancelled read should fail")
	}
}
