package model

import "time"

// PresenceStatus is the coarse online state visible to the coordinator.
type PresenceStatus string

const (
	PresenceOnline  PresenceStatus = "online"
	PresenceOffline PresenceStatus = "offline"
)

// User is an account. The server stores no file bytes for a user.
type User struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Device is a linked peer. DeviceID is derived from the device public key.
type Device struct {
	DeviceID  string    `json:"device_id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Platform  string    `json:"platform"`
	PublicKey []byte    `json:"public_key"` // raw Ed25519 public key
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen_at"`
}

// AuthToken is a per-device opaque credential. Only the hash is persisted.
type AuthToken struct {
	TokenHash string    `json:"-"`
	DeviceID  string    `json:"device_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"` // zero means no expiry in v1
}

// Folder is the sync unit. Membership is via Subscription, not implied storage.
type Folder struct {
	FolderID  string    `json:"folder_id"`
	OwnerID   string    `json:"owner_id"` // user_id of creator
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Subscription means a device will receive metadata updates for a folder.
// It does not require the device to hold every file's bytes (selective sync later).
type Subscription struct {
	FolderID     string    `json:"folder_id"`
	DeviceID     string    `json:"device_id"`
	SubscribedAt time.Time `json:"subscribed_at"`
}

// Presence is coordinator-visible reachability for peer introduction.
// Endpoint is optional in Phase 1; later phases fill in dial hints.
type Presence struct {
	DeviceID  string         `json:"device_id"`
	Status    PresenceStatus `json:"status"`
	Endpoint  string         `json:"endpoint,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}
