// Package sms is the one file that knows the SMS provider's request format
// (docs/09 section 3.1, open question Q2). It posts a JSON body to SMS_API_URL
// with the key from the secret store. Another provider means changing this file
// only; the service above it does not change.
package sms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gp/internal/notify"
	"gp/internal/shared/secrets"
)

const (
	keyName = "sms/api-key"
	timeout = 8 * time.Second
	maxBody = 16 << 10
)

type Client struct {
	url, sender string
	secrets     secrets.Getter
	http        *http.Client
}

// New builds the client. The URL must be https; the config checks that too.
func New(apiURL, senderID string, sec secrets.Getter) (*Client, error) {
	u, err := url.Parse(apiURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("sms: SMS_API_URL must be an https URL")
	}
	return newClient(apiURL, senderID, sec), nil
}

func newClient(apiURL, senderID string, sec secrets.Getter) *Client {
	return &Client{url: apiURL, sender: senderID, secrets: sec, http: &http.Client{
		Timeout: timeout,
		// An answer that redirects is a failure: the key must not follow it elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

type request struct {
	Sender  string `json:"sender"`
	To      string `json:"to"`
	Message string `json:"message"`
}

type answer struct {
	ID string `json:"id"`
}

// Send posts one SMS. Errors say what went wrong in fixed words; the provider's
// answer is never copied into them, because it may repeat the number or the text.
func (c *Client) Send(ctx context.Context, m notify.Message) (string, int, error) {
	text, err := notify.TextOf(m)
	if err != nil {
		return "", 0, err
	}
	key, err := c.secrets.Get(ctx, keyName)
	if err != nil {
		return "", 0, errors.New("sms: the API key is not available")
	}
	body, err := json.Marshal(request{Sender: c.sender, To: strings.TrimPrefix(m.To, "+"), Message: text})
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return "", 0, errors.New("sms: bad request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, errors.New("sms: the provider could not be reached")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", 0, fmt.Errorf("sms: the provider answered %d", resp.StatusCode)
	}
	var a answer
	_ = json.Unmarshal(raw, &a) // a provider that sends no id still accepted the text
	return a.ID, 1, nil         // every template fits one segment
}
