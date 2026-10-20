package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type config struct {
	ClientID         string `toml:"clientId"`
	ClientSecret     string `toml:"clientSecret"`
	RefreshToken     string `toml:"refreshToken"`
	SpreadsheetID    string `toml:"spreadsheetId"`
	TwilioAuthToken  string `toml:"twilioAuthToken"`
	TwilioAccountSID string `toml:"twilioAccountSid"`
	TwilioFromNumber string `toml:"twilioFromNumber"`
	TwilioToNumber   string `toml:"twilioToNumber"`
}

func loadConfig(path string, explicitPath, dryRun bool) (config, error) {
	var cfg config
	data, err := os.ReadFile(path)
	if err != nil && (explicitPath || !errors.Is(err, os.ErrNotExist)) {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err == nil {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			// TOML errors can include the source text, including credentials.
			return cfg, errors.New("invalid TOML configuration; check the syntax and string values")
		}
	}
	fields := []struct {
		key   string
		env   string
		value *string
	}{
		{"clientId", "GOOGLE_CLIENT_ID", &cfg.ClientID},
		{"clientSecret", "GOOGLE_CLIENT_SECRET", &cfg.ClientSecret},
		{"refreshToken", "GOOGLE_REFRESH_TOKEN", &cfg.RefreshToken},
		{"spreadsheetId", "GOOGLE_SPREADSHEET_ID", &cfg.SpreadsheetID},
		{"twilioAuthToken", "TWILIO_AUTH_TOKEN", &cfg.TwilioAuthToken},
		{"twilioAccountSid", "TWILIO_ACCOUNT_SID", &cfg.TwilioAccountSID},
		{"twilioFromNumber", "TWILIO_FROM_NUMBER", &cfg.TwilioFromNumber},
		{"twilioToNumber", "TWILIO_TO_NUMBER", &cfg.TwilioToNumber},
	}
	var missing []string
	for i, field := range fields {
		// Also accept the original program's unqualified Ballerina env names.
		if value, ok := os.LookupEnv("BAL_CONFIG_VAR_" + strings.ToUpper(field.key)); ok {
			*field.value = value
		}
		if value, ok := os.LookupEnv(field.env); ok {
			*field.value = value
		}
		if dryRun && i >= 4 {
			continue
		}
		if strings.TrimSpace(*field.value) == "" || strings.HasPrefix(*field.value, "<") {
			missing = append(missing, field.key+" ("+field.env+")")
		}
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing configuration: %s; use Config.toml or environment variables", strings.Join(missing, ", "))
	}
	return cfg, nil
}
