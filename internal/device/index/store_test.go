package index

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestNormalizePath(t *testing.T) {
	got, err := NormalizePath(`sub\file.txt`)
	if err != nil || got != "sub/file.txt" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizePath("../x"); err == nil {
		t.Fatal("expected error for ..")
	}
}

func TestUpsertListCountCursor(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "file_index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	e := Entry{
		FolderID: "fld_1", Path: "a.txt", Size: 2, ContentHash: "ab",
		ModTime: now, HLCWall: 10, HLCCounter: 0, DeviceID: "dev_a",
	}
	if err := store.Upsert(ctx, e); err != nil {
		t.Fatal(err)
	}
	list, err := store.List(ctx, "fld_1")
	if err != nil || len(list) != 1 || list[0].ContentHash != "ab" {
		t.Fatalf("list: %+v %v", list, err)
	}
	alive, tombs, err := store.Count(ctx, "fld_1")
	if err != nil || alive != 1 || tombs != 0 {
		t.Fatalf("count: %d %d %v", alive, tombs, err)
	}
	if err := store.SetCursor(ctx, "fld_1", 42); err != nil {
		t.Fatal(err)
	}
	cur, err := store.Cursor(ctx, "fld_1")
	if err != nil || cur != 42 {
		t.Fatalf("cursor: %d %v", cur, err)
	}
}

func TestApplyRemoteLWW(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "file_index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	_ = store.Upsert(ctx, Entry{
		FolderID: "fld", Path: "f.txt", ContentHash: "old",
		HLCWall: 100, HLCCounter: 0, DeviceID: "dev_a",
	})

	changed, err := store.ApplyRemote(ctx, Entry{
		FolderID: "fld", Path: "f.txt", ContentHash: "stale",
		HLCWall: 50, HLCCounter: 0, DeviceID: "dev_b",
	})
	if err != nil || changed {
		t.Fatalf("stale should not apply: changed=%v err=%v", changed, err)
	}

	changed, err = store.ApplyRemote(ctx, Entry{
		FolderID: "fld", Path: "f.txt", ContentHash: "new",
		HLCWall: 200, HLCCounter: 0, DeviceID: "dev_b",
	})
	if err != nil || !changed {
		t.Fatalf("newer should apply: changed=%v err=%v", changed, err)
	}
	got, _ := store.Get(ctx, "fld", "f.txt")
	if got.ContentHash != "new" {
		t.Fatalf("got %+v", got)
	}

	// Tombstone with higher HLC
	changed, err = store.ApplyRemote(ctx, Entry{
		FolderID: "fld", Path: "f.txt", Deleted: true,
		HLCWall: 300, HLCCounter: 0, DeviceID: "dev_a",
	})
	if err != nil || !changed {
		t.Fatalf("delete: %v %v", changed, err)
	}
	list, _ := store.List(ctx, "fld")
	if len(list) != 0 {
		t.Fatalf("tombstone should hide from list: %+v", list)
	}
	alive, tombs, _ := store.Count(ctx, "fld")
	if alive != 0 || tombs != 1 {
		t.Fatalf("count after tombstone: %d %d", alive, tombs)
	}
}
