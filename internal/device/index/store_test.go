package index

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
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

func TestOmitLocalSetAndCount(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "file_index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	if err := store.Upsert(ctx, Entry{
		FolderID: "fld", Path: "big.mov", Size: 9, ContentHash: "hh",
		HLCWall: 1, DeviceID: "dev",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOmitLocal(ctx, "fld", "big.mov", true); err != nil {
		t.Fatal(err)
	}
	e, err := store.Get(ctx, "fld", "big.mov")
	if err != nil || !e.OmitLocal || e.Deleted || e.ContentHash != "hh" {
		t.Fatalf("omit entry: %+v %v", e, err)
	}
	n, err := store.CountOmitted(ctx, "fld")
	if err != nil || n != 1 {
		t.Fatalf("omitted=%d err=%v", n, err)
	}
	list, _ := store.List(ctx, "fld")
	if len(list) != 1 || !list[0].OmitLocal {
		t.Fatalf("list should still show live omitted: %+v", list)
	}

	// Tombstone cannot be omitted.
	_ = store.Upsert(ctx, Entry{
		FolderID: "fld", Path: "gone.txt", Deleted: true,
		HLCWall: 2, DeviceID: "dev",
	})
	if err := store.SetOmitLocal(ctx, "fld", "gone.txt", true); !errors.Is(err, ErrNotAlive) {
		t.Fatalf("want ErrNotAlive, got %v", err)
	}
}

func TestApplyRemotePreservesOmitUntilHashChanges(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "file_index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	_ = store.Upsert(ctx, Entry{
		FolderID: "fld", Path: "f.txt", ContentHash: "same", OmitLocal: true,
		HLCWall: 100, DeviceID: "dev_a",
	})

	// Same hash, higher HLC: keep omit.
	ok, err := store.ApplyRemote(ctx, Entry{
		FolderID: "fld", Path: "f.txt", ContentHash: "same", Size: 1,
		HLCWall: 200, DeviceID: "dev_b",
	})
	if err != nil || !ok {
		t.Fatalf("apply same hash: %v %v", ok, err)
	}
	e, _ := store.Get(ctx, "fld", "f.txt")
	if !e.OmitLocal {
		t.Fatal("omit should be preserved for same content_hash")
	}

	// Different hash: clear omit so device can pull the new version.
	ok, err = store.ApplyRemote(ctx, Entry{
		FolderID: "fld", Path: "f.txt", ContentHash: "new", Size: 2,
		HLCWall: 300, DeviceID: "dev_b",
	})
	if err != nil || !ok {
		t.Fatalf("apply new hash: %v %v", ok, err)
	}
	e, _ = store.Get(ctx, "fld", "f.txt")
	if e.OmitLocal || e.ContentHash != "new" {
		t.Fatalf("omit should clear on hash change: %+v", e)
	}

	_ = store.SetOmitLocal(ctx, "fld", "f.txt", true)
	ok, err = store.ApplyRemote(ctx, Entry{
		FolderID: "fld", Path: "f.txt", Deleted: true,
		HLCWall: 400, DeviceID: "dev_a",
	})
	if err != nil || !ok {
		t.Fatalf("delete: %v %v", ok, err)
	}
	e, _ = store.Get(ctx, "fld", "f.txt")
	if !e.Deleted || e.OmitLocal {
		t.Fatalf("tombstone should clear omit: %+v", e)
	}
}

func TestMigrateOmitLocalOnLegacyDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE file_entries (
			folder_id TEXT NOT NULL, path TEXT NOT NULL,
			size INTEGER NOT NULL DEFAULT 0, content_hash TEXT NOT NULL DEFAULT '',
			mtime TEXT, hlc_wall INTEGER NOT NULL, hlc_counter INTEGER NOT NULL,
			deleted INTEGER NOT NULL DEFAULT 0, device_id TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL, PRIMARY KEY (folder_id, path)
		)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO file_entries (
		folder_id, path, size, content_hash, hlc_wall, hlc_counter, deleted, device_id, updated_at
	) VALUES ('fld', 'a.txt', 1, 'x', 1, 0, 0, 'd', ?)`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	e, err := store.Get(context.Background(), "fld", "a.txt")
	if err != nil || e.OmitLocal {
		t.Fatalf("migrated get: %+v %v", e, err)
	}
	if err := store.SetOmitLocal(context.Background(), "fld", "a.txt", true); err != nil {
		t.Fatal(err)
	}
}
