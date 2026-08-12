package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// FolderPeer is an online peer for a subscribed folder (ADR 26). No file bytes.
type FolderPeer struct {
	DeviceID     string    `json:"device_id"`
	Name         string    `json:"name"`
	Platform     string    `json:"platform"`
	Endpoint     string    `json:"endpoint,omitempty"`
	PublicKeyHex string    `json:"public_key_hex"`
	Status       string    `json:"status"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type folderPeersResponse struct {
	Peers      []FolderPeer `json:"peers"`
	TTLSeconds int          `json:"ttl_seconds"`
}

// ListFolderPeers returns online devices subscribed to folderID (excludes this device).
func (c *Client) ListFolderPeers(ctx context.Context, folderID string) ([]FolderPeer, error) {
	if folderID == "" {
		return nil, fmt.Errorf("folder id required")
	}
	path := "/v1/folders/" + url.PathEscape(folderID) + "/peers"
	var out folderPeersResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	if out.Peers == nil {
		return []FolderPeer{}, nil
	}
	return out.Peers, nil
}

// DialablePeers filters peers that advertised a non-empty transfer endpoint.
func DialablePeers(peers []FolderPeer) []FolderPeer {
	var out []FolderPeer
	for _, p := range peers {
		if p.Endpoint != "" && p.Status == "online" {
			out = append(out, p)
		}
	}
	return out
}
