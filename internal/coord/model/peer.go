package model

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// FolderPeer is an online device subscribed to a folder, for P2P introduction
// (ADR 21 / 26). No content hashes — the coordinator is not a provider index.
type FolderPeer struct {
	DeviceID     string         `json:"device_id"`
	Name         string         `json:"name"`
	Platform     string         `json:"platform"`
	Endpoint     string         `json:"endpoint,omitempty"`
	PublicKey    []byte         `json:"-"`
	PublicKeyHex string         `json:"public_key_hex"`
	Status       PresenceStatus `json:"status"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// NormalizeTransferEndpoint trims and validates a dial hint as host:port.
// Empty string is allowed (heartbeat without a listener yet).
func NormalizeTransferEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return "", fmt.Errorf("endpoint must not contain whitespace")
	}
	if strings.Contains(raw, "://") {
		return "", fmt.Errorf("endpoint must be host:port, not a URL")
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return "", fmt.Errorf("endpoint must be host:port: %w", err)
	}
	if host == "" {
		return "", fmt.Errorf("endpoint host required")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("endpoint port must be 1-65535")
	}
	return net.JoinHostPort(host, strconv.Itoa(n)), nil
}
