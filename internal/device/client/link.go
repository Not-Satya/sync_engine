package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"runtime"
	"time"
)

// DeviceInfo is sent when linking a device.
type DeviceInfo struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

// LinkResult is returned by register / login / pairing redeem.
type LinkResult struct {
	UserID     string
	DeviceID   string
	Token      string
	PrivateKey []byte // Ed25519 private key
	PublicKey  []byte
	Name       string
	Platform   string
}

type authAPIResponse struct {
	UserID              string `json:"user_id"`
	Token               string `json:"token"`
	DevicePrivateKeyHex string `json:"device_private_key_hex"`
	Device              struct {
		DeviceID     string `json:"device_id"`
		Name         string `json:"name"`
		Platform     string `json:"platform"`
		PublicKeyHex string `json:"public_key_hex"`
	} `json:"device"`
}

type pairingCodeResult struct {
	Code       string    `json:"code"`
	ExpiresAt  time.Time `json:"expires_at"`
	TTLSeconds int       `json:"ttl_seconds"`
}

// DefaultPlatform returns a coarse OS label for device registration.
func DefaultPlatform() string {
	return runtime.GOOS
}

// Register creates an account and first device.
func (c *Client) Register(ctx context.Context, email, password string, device DeviceInfo) (LinkResult, error) {
	if device.Platform == "" {
		device.Platform = DefaultPlatform()
	}
	var raw authAPIResponse
	err := c.do(ctx, http.MethodPost, "/v1/accounts", map[string]any{
		"email":    email,
		"password": password,
		"device":   device,
	}, &raw)
	if err != nil {
		return LinkResult{}, err
	}
	return parseLinkResult(raw)
}

// Login links a new device to an existing account with password.
func (c *Client) Login(ctx context.Context, email, password string, device DeviceInfo) (LinkResult, error) {
	if device.Platform == "" {
		device.Platform = DefaultPlatform()
	}
	var raw authAPIResponse
	err := c.do(ctx, http.MethodPost, "/v1/accounts/login", map[string]any{
		"email":    email,
		"password": password,
		"device":   device,
	}, &raw)
	if err != nil {
		return LinkResult{}, err
	}
	return parseLinkResult(raw)
}

// CreatePairingCode asks the coordinator for a short-lived code (auth required).
func (c *Client) CreatePairingCode(ctx context.Context) (code string, expiresAt time.Time, err error) {
	var raw pairingCodeResult
	if err := c.do(ctx, http.MethodPost, "/v1/pairing-codes", map[string]any{}, &raw); err != nil {
		return "", time.Time{}, err
	}
	return raw.Code, raw.ExpiresAt, nil
}

// RedeemPairingCode links a new device using a pairing code (no auth).
func (c *Client) RedeemPairingCode(ctx context.Context, code string, device DeviceInfo) (LinkResult, error) {
	if device.Platform == "" {
		device.Platform = DefaultPlatform()
	}
	var raw authAPIResponse
	err := c.do(ctx, http.MethodPost, "/v1/pairing-codes/redeem", map[string]any{
		"code":   code,
		"device": device,
	}, &raw)
	if err != nil {
		return LinkResult{}, err
	}
	return parseLinkResult(raw)
}

func parseLinkResult(raw authAPIResponse) (LinkResult, error) {
	if raw.Token == "" || raw.Device.DeviceID == "" || raw.DevicePrivateKeyHex == "" {
		return LinkResult{}, fmt.Errorf("incomplete link response")
	}
	priv, err := hex.DecodeString(raw.DevicePrivateKeyHex)
	if err != nil {
		return LinkResult{}, fmt.Errorf("private key: %w", err)
	}
	pub, err := hex.DecodeString(raw.Device.PublicKeyHex)
	if err != nil {
		return LinkResult{}, fmt.Errorf("public key: %w", err)
	}
	return LinkResult{
		UserID:     raw.UserID,
		DeviceID:   raw.Device.DeviceID,
		Token:      raw.Token,
		PrivateKey: priv,
		PublicKey:  pub,
		Name:       raw.Device.Name,
		Platform:   raw.Device.Platform,
	}, nil
}
