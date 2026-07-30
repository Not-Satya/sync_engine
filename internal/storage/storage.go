// StorageBackend defines the operations requred by the sync engine.
//
// Implementation may be backed by LocalDisk, S3, Cloud Storage,
// or any other object store. The sync engine interacts only through
// this interface and remains agnoistic to the underlying storage.

package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotFound = errors.New("object not found")
var ErrConflict = errors.New("etag mismatch")
var ErrUploadNotfound = errors.New("upload session not found")

type PutOptions struct {
	IfMatch string
	ModTime time.Time
}

type ObjectMeta struct {
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ContentHash string    `json:"content_hash"`
	ModTime     time.Time `json:"mod_time"`
	ETag        string    `json:"etag"`
	Deleted     bool      `json:"deleted,omitempty"`
	Revision    int64     `json:"revision"`
	IsDir       bool      `json:"is_dir"`
}

type UploadSession struct {
	Key         string    `json:"key"`
	ID          string    `json:"id"`
	Size        int64     `json:"size"`
	Received    int64     `json:"received"`
	ContentHash string    `json:"content_hash,omitempty"`
	IfMatch     string    `json:"if_match,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type StorageBackend interface {
	List(ctx context.Context, prefix string) ([]ObjectMeta, error)
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectMeta, error)
	GetRange(ctx context.Context, key string, start, end int64) (io.ReadCloser, ObjectMeta, error)
	Put(ctx context.Context, key string, r io.Reader, opts PutOptions) (ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (ObjectMeta, error)

	CreateUpload(ctx context.Context, key, ifMatch string, totalSize int64) (UploadSession, error)
	Uploadchunk(ctx context.Context, uploadID string, offset int64, r io.Reader) (UploadSession, error)
	CompleteUpload(ctx context.Context, uploadID string) (ObjectMeta, error)
}
