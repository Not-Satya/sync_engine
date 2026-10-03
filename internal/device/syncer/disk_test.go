package syncer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Not-Satya/sync_engine/internal/device/index"
)

func TestUnlinkUnderRootRemovesFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sub", "a.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := UnlinkUnderRoot(root, "sub/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be gone: %v", err)
	}
}

func TestUnlinkUnderRootMissingIsOK(t *testing.T) {
	root := t.TempDir()
	if err := UnlinkUnderRoot(root, "nope.txt"); err != nil {
		t.Fatal(err)
	}
}

func TestUnlinkUnderRootSkipsDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "keep")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := UnlinkUnderRoot(root, "keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir should remain: %v", err)
	}
}

func TestUnlinkUnderRootRejectsEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	err := UnlinkUnderRoot(root, "../outside.txt")
	if err == nil {
		t.Fatal("expected escape error")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file must not be removed: %v", err)
	}
}

func TestRemoveLocalCopyOmitsWithoutOutbox(t *testing.T) {
	ctx := context.Background()
	idx := openIndex(t)
	root := t.TempDir()
	path := filepath.Join(root, "movie.mp4")
	if err := os.WriteFile(path, []byte("bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := idx.Upsert(ctx, index.Entry{
		FolderID: "fld_1", Path: "movie.mp4", Size: 5, ContentHash: "hh",
		HLCWall: 1, DeviceID: "dev",
	}); err != nil {
		t.Fatal(err)
	}

	if err := RemoveLocalCopy(ctx, idx, "fld_1", "movie.mp4", root); err != nil {
		t.Fatal(err)
	}
	e, err := idx.Get(ctx, "fld_1", "movie.mp4")
	if err != nil || e.Deleted || !e.OmitLocal || e.ContentHash != "hh" {
		t.Fatalf("want live omitted entry: %+v %v", e, err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("file should be unlinked: %v", err)
	}
	n, err := idx.OutboxCount(ctx, "fld_1")
	if err != nil || n != 0 {
		t.Fatalf("must not enqueue delete, outbox=%d err=%v", n, err)
	}
	omitted, err := idx.CountOmitted(ctx, "fld_1")
	if err != nil || omitted != 1 {
		t.Fatalf("omitted=%d err=%v", omitted, err)
	}

	if err := ClearOmitLocal(ctx, idx, "fld_1", "movie.mp4"); err != nil {
		t.Fatal(err)
	}
	e, _ = idx.Get(ctx, "fld_1", "movie.mp4")
	if e.OmitLocal {
		t.Fatal("clear should drop omit_local")
	}
}
