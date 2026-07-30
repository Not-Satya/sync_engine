package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// DeviceSummary is one row from GET /v1/devices.
type DeviceSummary struct {
	DeviceID     string     `json:"device_id"`
	UserID       string     `json:"user_id"`
	Name         string     `json:"name"`
	Platform     string     `json:"platform"`
	CreatedAt    time.Time  `json:"created_at"`
	LastSeenAt   time.Time  `json:"last_seen_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	Revoked      bool       `json:"revoked"`
	IsThisDevice bool       `json:"is_this_device"`
}

type devicesResponse struct {
	Devices      []DeviceSummary `json:"devices"`
	ActiveCount  int             `json:"active_count"`
	TotalCount   int             `json:"total_count"`
	ThisDeviceID string          `json:"this_device_id"`
}

// ListDevices returns all devices on the caller's account.
func (c *Client) ListDevices(ctx context.Context) (devicesResponse, error) {
	var out devicesResponse
	if err := c.do(ctx, http.MethodGet, "/v1/devices", nil, &out); err != nil {
		return devicesResponse{}, err
	}
	return out, nil
}

// RevokeDevice soft-revokes a device on the same account (including self).
func (c *Client) RevokeDevice(ctx context.Context, deviceID string) (DeviceSummary, error) {
	if deviceID == "" {
		return DeviceSummary{}, fmt.Errorf("device id required")
	}
	var out DeviceSummary
	path := "/v1/devices/" + url.PathEscape(deviceID)
	if err := c.do(ctx, http.MethodDelete, path, nil, &out); err != nil {
		return DeviceSummary{}, err
	}
	return out, nil
}
