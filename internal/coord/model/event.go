package model

import "time"

// MetaOp is a metadata mutation in the folder event log (ADR 15). No file bytes.
type MetaOp string

const (
	MetaOpUpsert MetaOp = "upsert"
	MetaOpDelete MetaOp = "delete"
	MetaOpRename MetaOp = "rename"
)

// HLC is a hybrid logical clock stamp (ADR 16).
type HLC struct {
	Wall    int64 `json:"wall"`    // unix nanoseconds
	Counter int64 `json:"counter"` // logical tie-break within the same wall
}

// FolderEvent is one append-only metadata event. The coordinator never stores file bytes.
type FolderEvent struct {
	Seq         int64     `json:"seq"` // server-assigned, monotonic per DB
	EventID     string    `json:"event_id"`
	FolderID    string    `json:"folder_id"`
	DeviceID    string    `json:"device_id"`
	Op          MetaOp    `json:"op"`
	Path        string    `json:"path"`               // relative path within folder
	OldPath     string    `json:"old_path,omitempty"` // rename only
	Size        int64     `json:"size,omitempty"`
	ContentHash string    `json:"content_hash,omitempty"`
	ModTime     time.Time `json:"mtime,omitempty"`
	HLC         HLC       `json:"hlc"`
	CreatedAt   time.Time `json:"created_at"`
}
