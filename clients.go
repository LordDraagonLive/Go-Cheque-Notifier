package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type apiClient struct {
	config       config
	http         *http.Client
	tokenURL     string
	sheetsURL    string
	twilioURL    string
	accessToken  string
	tokenExpires time.Time
}

func newAPIClient(cfg config) *apiClient {
	return &apiClient{
		config:    cfg,
		http:      &http.Client{Timeout: 30 * time.Second},
		tokenURL:  "https://oauth2.googleapis.com/token",
		sheetsURL: "https://sheets.googleapis.com/v4/spreadsheets",
		twilioURL: "https://api.twilio.com/2010-04-01/Accounts",
	}
}

func (api *apiClient) token(ctx context.Context) (string, error) {
	if api.accessToken != "" && time.Now().Add(30*time.Second).Before(api.tokenExpires) {
		return api.accessToken, nil
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {api.config.ClientID},
		"client_secret": {api.config.ClientSecret},
		"refresh_token": {api.config.RefreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := api.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("Google OAuth request: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, "Google OAuth"); err != nil {
		return "", err
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil || token.AccessToken == "" {
		return "", errors.New("Google OAuth returned an invalid token response")
	}
	if token.ExpiresIn <= 0 {
		return "", errors.New("Google OAuth returned an invalid token lifetime")
	}
	api.accessToken = token.AccessToken
	api.tokenExpires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return api.accessToken, nil
}

func (api *apiClient) ReadSheet(ctx context.Context, sheet string) ([][]string, error) {
	token, err := api.token(ctx)
	if err != nil {
		return nil, err
	}
	endpoint := api.sheetsURL + "/" + url.PathEscape(api.config.SpreadsheetID) + "/values/" + url.PathEscape(sheet+"!B2:F")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := api.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Google Sheets request: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, "Google Sheets"); err != nil {
		return nil, err
	}
	var data struct {
		Values [][]json.RawMessage `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, errors.New("Google Sheets returned an invalid values response")
	}
	rows := make([][]string, len(data.Values))
	for i, values := range data.Values {
		rows[i] = make([]string, len(values))
		for j, value := range values {
			var text string
			if err := json.Unmarshal(value, &text); err == nil && string(value) != "null" {
				rows[i][j] = text
				continue
			}
			// Sheets may return numeric cells. Use their exact decimal representation.
			text, err := decimalString(string(value))
			if err != nil {
				return nil, fmt.Errorf("Google Sheets row %d column %d is not text or a decimal number", i+2, j+2)
			}
			rows[i][j] = text
		}
	}
	return rows, nil
}

func (api *apiClient) Send(ctx context.Context, message string) error {
	log.Print("Sending WhatsApp message: " + message)
	form := url.Values{
		"To":   {"whatsapp:" + api.config.TwilioToNumber},
		"From": {"whatsapp:" + api.config.TwilioFromNumber},
		"Body": {message},
	}
	endpoint := api.twilioURL + "/" + url.PathEscape(api.config.TwilioAccountSID) + "/Messages.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(api.config.TwilioAccountSID, api.config.TwilioAuthToken)
	resp, err := api.http.Do(req)
	if err != nil {
		return fmt.Errorf("Twilio request: %w", err)
	}
	defer resp.Body.Close()
	if err := checkStatus(resp, "Twilio"); err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func checkStatus(resp *http.Response, service string) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Do not include response bodies that could echo tokens or cheque data.
		return fmt.Errorf("%s request failed: HTTP %d %s", service, resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return nil
}
