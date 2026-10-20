package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := execute(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "cheque-notifier:", err)
		os.Exit(1)
	}
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("cheque-notifier", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "Config.toml", "TOML configuration file")
	dryRun := flags.Bool("dry-run", false, "print messages without sending WhatsApp notifications")
	fixturePath := flags.String("fixture", "", "read local JSON sheet data (requires -dry-run)")
	instant := flags.String("now", "", "use an RFC3339 time for a reproducible dry run (requires -dry-run)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() > 0 {
		return errors.New("unexpected positional arguments; use -help for usage")
	}
	if (*fixturePath != "" || *instant != "") && !*dryRun {
		return errors.New("-fixture and -now require -dry-run")
	}
	now := time.Now()
	if *instant != "" {
		var err error
		now, err = time.Parse(time.RFC3339, *instant)
		if err != nil {
			return errors.New("-now must be an RFC3339 timestamp such as 2026-10-21T08:00:00+05:30")
		}
	}
	var reader sheetReader
	var sender messageSender = previewSender{stdout}
	if *fixturePath != "" {
		file, err := os.Open(*fixturePath)
		if err != nil {
			return fmt.Errorf("open fixture: %w", err)
		}
		defer file.Close()
		var data fixtureReader
		decoder := json.NewDecoder(file)
		if err := decoder.Decode(&data); err != nil {
			return fmt.Errorf("decode fixture: %w", err)
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return errors.New("fixture must contain a single JSON object")
		}
		reader = data
	} else {
		explicitConfig := false
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "config" {
				explicitConfig = true
			}
		})
		cfg, err := loadConfig(*configPath, explicitConfig, *dryRun)
		if err != nil {
			return err
		}
		api := newAPIClient(cfg)
		reader = api
		if !*dryRun {
			sender = api
		}
	}
	return run(ctx, reader, sender, now)
}

type fixtureReader map[string][][]string

func (data fixtureReader) ReadSheet(_ context.Context, sheet string) ([][]string, error) {
	rows, ok := data[sheet]
	if !ok {
		return nil, fmt.Errorf("fixture is missing sheet %q", sheet)
	}
	return rows, nil
}

type previewSender struct{ writer io.Writer }

func (sender previewSender) Send(_ context.Context, message string) error {
	if _, err := io.WriteString(sender.writer, message); err != nil {
		return err
	}
	if len(message) == 0 || message[len(message)-1] != '\n' {
		_, err := io.WriteString(sender.writer, "\n")
		return err
	}
	return nil
}
