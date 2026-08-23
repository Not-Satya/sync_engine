package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

func TestIndexBlobStoreOpen(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	folder := filepath.Join(root, "sync")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte("blob-bytes-for-p5.5")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(folder, "doc.txt"), payload, 0o600); err != nil {
		t.Fatal(err)
	}

	bind := bindings.Open(filepath.Join(root, "bindings.json"))
	if err := bind.Put(bindings.Binding{
		FolderID: "fld_1", LocalPath: folder, Name: "Sync", Subscribed: true,
	}); err != nil {
		t.Fatal(err)
	}
	idx, err := index.Open(filepath.Join(root, index.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	if err := idx.Upsert(ctx, index.Entry{
		FolderID: "fld_1", Path: "doc.txt", Size: int64(len(payload)),
		ContentHash: hash, HLCWall: 1, DeviceID: "dev_a",
	}); err != nil {
		t.Fatal(err)
	}

	store := IndexBlobStore{Index: idx, Bindings: bind}
	size, rc, err := store.Open(ctx, hash)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if size != int64(len(payload)) {
		t.Fatalf("size=%d", size)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q", got)
	}

	_, _, err = store.Open(ctx, "deadbeef")
	if err != ErrBlobNotFound {
		t.Fatalf("want ErrBlobNotFound, got %v", err)
	}
}
