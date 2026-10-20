package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	receivedSheet = "Cheques Received"
	issuedSheet   = "Cheques Issued"
	noChequesText = "✅ No incoming or issued cheques due today. You're good to go!"
)

var sriLanka = time.FixedZone("Asia/Colombo", 19800)

type cheque struct {
	Number string
	Payee  string
	Amount string // Decimal text; never convert money through float64.
	Bank   string
	Date   string
}

type sheetReader interface {
	ReadSheet(context.Context, string) ([][]string, error)
}

type messageSender interface {
	Send(context.Context, string) error
}

func run(ctx context.Context, reader sheetReader, sender messageSender, now time.Time) error {
	today := now.In(sriLanka).Format(time.DateOnly)
	future := now.In(sriLanka).AddDate(0, 0, 3).Format(time.DateOnly)
	noCheques := true
	for _, sheet := range []string{receivedSheet, issuedSheet} {
		rows, err := reader.ReadSheet(ctx, sheet)
		if err != nil {
			return fmt.Errorf("read %s: %w", sheet, err)
		}
		var todayCheques, futureCheques []cheque
		for index, row := range rows {
			if len(row) < 5 {
				continue
			}
			// Parse every complete row before filtering, as in the Ballerina source.
			if _, err := time.Parse(time.DateOnly, row[4]); err != nil {
				return fmt.Errorf("%s row %d: cheque date must use YYYY-MM-DD", sheet, index+2)
			}
			amount, err := transformAmount(row[2])
			if err != nil {
				return fmt.Errorf("%s row %d: %w", sheet, index+2, err)
			}
			item := cheque{Number: row[0], Payee: row[1], Amount: amount, Bank: row[3], Date: row[4]}
			if row[4] == today {
				todayCheques = append(todayCheques, item)
			} else if row[4] == future {
				futureCheques = append(futureCheques, item)
			}
		}
		// Send each sheet before reading the next one, preserving partial-failure behavior.
		for _, group := range []struct {
			items []cheque
			due   string
		}{{futureCheques, "in 3 days"}, {todayCheques, "today"}} {
			if len(group.items) == 0 {
				continue
			}
			noCheques = false
			if err := sender.Send(ctx, formatMessage(sheet, group.items, group.due)); err != nil {
				return fmt.Errorf("send %s %s: %w", sheet, group.due, err)
			}
		}
	}
	if noCheques {
		return sender.Send(ctx, noChequesText)
	}
	return nil
}

func formatMessage(sheet string, cheques []cheque, due string) string {
	prefix := "🚨 *Urgent*"
	if due == "in 3 days" {
		prefix = "🔔 *Reminder*"
	}
	var header, action string
	if sheet == receivedSheet {
		if due == "in 3 days" {
			header = "Deposit Cheques Soon"
			action = "The following cheques can be deposited in 3 days:"
		} else {
			header = "Deposit Cheques Today"
			action = "The following cheques should be deposited today:"
		}
	} else if due == "in 3 days" {
		header = "Ensure Funds for Issued Cheques"
		action = "Make sure the following issued cheques have sufficient balance before they are cashed:"
	} else {
		header = "Funds Required for Issued Cheques Today"
		action = "Ensure your bank has enough funds for these issued cheques today:"
	}
	var message strings.Builder
	fmt.Fprintf(&message, "%s: %s\n--------------------------------------------------\n%s\n\n", prefix, header, action)
	for _, item := range cheques {
		fmt.Fprintf(&message, "🧾 Cheque No : %s\n👤 Payee     : %s\n💰 Amount    : %s\n🏦 Bank      : %s\n📅 Date      : %s\n\n",
			item.Number, item.Payee, item.Amount, item.Bank, item.Date)
	}
	return message.String()
}
