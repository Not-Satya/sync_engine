package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the coordination server. It never uploads file bytes.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Me is the authenticated identity returned by GET /v1/me.
type Me struct {
	UserID string         `json:"user_id"`
	Email  string         `json:"email"`
	Device map[string]any `json:"device"`
}

// Presence is returned by heartbeat.
type Presence struct {
	DeviceID  string    `json:"device_id"`
	Status    string    `json:"status"`
	Endpoint  string    `json:"endpoint,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("coord api %d: %s", e.Status, e.Body)
}

func (c *Client) Me(ctx context.Context) (Me, error) {
	var out Me
	if err := c.do(ctx, http.MethodGet, "/v1/me", nil, &out); err != nil {
		return Me{}, err
	}
	return out, nil
}

func (c *Client) Heartbeat(ctx context.Context, endpoint string) (Presence, error) {
	body := map[string]string{}
	if endpoint != "" {
		body["endpoint"] = endpoint
	}
	var out Presence
	if err := c.do(ctx, http.MethodPost, "/v1/presence/heartbeat", body, &out); err != nil {
		return Presence{}, err
	}
	return out, nil
}

func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return &apiError{Status: res.StatusCode, Body: string(b)}
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &apiError{Status: res.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}
