package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "GOOGLE_REFRESH_TOKEN", "GOOGLE_SPREADSHEET_ID",
		"TWILIO_AUTH_TOKEN", "TWILIO_ACCOUNT_SID", "TWILIO_FROM_NUMBER", "TWILIO_TO_NUMBER",
		"BAL_CONFIG_VAR_CLIENTID", "BAL_CONFIG_VAR_CLIENTSECRET", "BAL_CONFIG_VAR_REFRESHTOKEN", "BAL_CONFIG_VAR_SPREADSHEETID",
		"BAL_CONFIG_VAR_TWILIOAUTHTOKEN", "BAL_CONFIG_VAR_TWILIOACCOUNTSID", "BAL_CONFIG_VAR_TWILIOFROMNUMBER", "BAL_CONFIG_VAR_TWILIOTONUMBER",
	} {
		value, exists := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if exists {
				_ = os.Setenv(name, value)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

func TestConfigFileAndEnvironmentPrecedence(t *testing.T) {
	clearConfigEnvironment(t)
	path := filepath.Join(t.TempDir(), "Config.toml")
	data := `clientId = "file-client"
clientSecret = "file-secret"
refreshToken = "file-refresh"
spreadsheetId = "file-sheet"
twilioAuthToken = "file-token"
twilioAccountSid = "ACfile"
twilioFromNumber = "+14155238886" # configuration with inline comments
twilioToNumber = "+94770000000"
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BAL_CONFIG_VAR_CLIENTID", "alternate-client")
	t.Setenv("GOOGLE_CLIENT_ID", "go-client")
	t.Setenv("BAL_CONFIG_VAR_SPREADSHEETID", "alternate-sheet")
	cfg, err := loadConfig(path, true, false)
	if err != nil || cfg.ClientID != "go-client" || cfg.SpreadsheetID != "alternate-sheet" || cfg.TwilioAccountSID != "ACfile" {
		t.Fatalf("unexpected config %#v, error %v", cfg, err)
	}
}

func TestConfigMissingValuesAndDryRun(t *testing.T) {
	clearConfigEnvironment(t)
	path := filepath.Join(t.TempDir(), "missing.toml")
	for key, value := range map[string]string{
		"GOOGLE_CLIENT_ID": "client", "GOOGLE_CLIENT_SECRET": "secret", "GOOGLE_REFRESH_TOKEN": "refresh", "GOOGLE_SPREADSHEET_ID": "sheet",
	} {
		t.Setenv(key, value)
	}
	if _, err := loadConfig(path, false, true); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path, false, false); err == nil || !strings.Contains(err.Error(), "twilioAuthToken") {
		t.Fatalf("expected missing Twilio config, got %v", err)
	}
	if _, err := loadConfig(path, true, true); err == nil {
		t.Fatal("explicit missing config must fail")
	}
	t.Setenv("GOOGLE_CLIENT_SECRET", "<PLACEHOLDER>")
	if _, err := loadConfig(path, false, true); err == nil || strings.Contains(err.Error(), "<PLACEHOLDER>") {
		t.Fatalf("expected missing secret without printing it, got %v", err)
	}
}

func TestMalformedConfigDoesNotExposeSecret(t *testing.T) {
	clearConfigEnvironment(t)
	path := filepath.Join(t.TempDir(), "Config.toml")
	if err := os.WriteFile(path, []byte(`clientSecret = "secret-value`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(path, true, false); err == nil || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("unexpected error %v", err)
	}
}
