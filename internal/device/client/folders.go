package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Folder is a sync unit on the coordinator (no local path).
type Folder struct {
	FolderID  string    `json:"folder_id"`
	OwnerID   string    `json:"owner_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Subscription is this device's membership in a folder on the coordinator.
type Subscription struct {
	FolderID     string    `json:"folder_id"`
	DeviceID     string    `json:"device_id"`
	SubscribedAt time.Time `json:"subscribed_at"`
}

type foldersResponse struct {
	Folders []Folder `json:"folders"`
}

type subscriptionsResponse struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// CreateFolder creates a folder on the account; the creating device is auto-subscribed server-side.
func (c *Client) CreateFolder(ctx context.Context, name string) (Folder, error) {
	if name == "" {
		return Folder{}, fmt.Errorf("folder name required")
	}
	var out Folder
	if err := c.do(ctx, http.MethodPost, "/v1/folders", map[string]string{"name": name}, &out); err != nil {
		return Folder{}, err
	}
	return out, nil
}

// ListFolders returns folders owned by the caller's account.
func (c *Client) ListFolders(ctx context.Context) ([]Folder, error) {
	var out foldersResponse
	if err := c.do(ctx, http.MethodGet, "/v1/folders", nil, &out); err != nil {
		return nil, err
	}
	if out.Folders == nil {
		return []Folder{}, nil
	}
	return out.Folders, nil
}

// SubscribeFolder subscribes the calling device to folderID.
func (c *Client) SubscribeFolder(ctx context.Context, folderID string) (Subscription, error) {
	if folderID == "" {
		return Subscription{}, fmt.Errorf("folder id required")
	}
	var out Subscription
	path := "/v1/folders/" + url.PathEscape(folderID) + "/subscriptions"
	if err := c.do(ctx, http.MethodPost, path, map[string]any{}, &out); err != nil {
		return Subscription{}, err
	}
	return out, nil
}

// UnsubscribeFolder removes this device's subscription to folderID.
func (c *Client) UnsubscribeFolder(ctx context.Context, folderID string) error {
	if folderID == "" {
		return fmt.Errorf("folder id required")
	}
	path := "/v1/folders/" + url.PathEscape(folderID) + "/subscriptions"
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// ListSubscriptions returns this device's folder subscriptions.
func (c *Client) ListSubscriptions(ctx context.Context) ([]Subscription, error) {
	var out subscriptionsResponse
	if err := c.do(ctx, http.MethodGet, "/v1/subscriptions", nil, &out); err != nil {
		return nil, err
	}
	if out.Subscriptions == nil {
		return []Subscription{}, nil
	}
	return out.Subscriptions, nil
}
