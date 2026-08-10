package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/device/hlc"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

func newScanner(t *testing.T) (*Scanner, *index.Store, string) {
	t.Helper()
	// Index DB lives outside the scanned root (as it does in production).
	idx, err := index.Open(filepath.Join(t.TempDir(), index.FileName))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	root := t.TempDir()
	return New(idx, hlc.New(), "dev_test"), idx, root
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanPathsUpsertModifyDelete(t *testing.T) {
	ctx := context.Background()
	sc, idx, root := newScanner(t)

	// Create -> upsert.
	write(t, root, "a.txt", "hello")
	changes, err := sc.ScanPaths(ctx, "fld_1", root, []string{"a.txt"})
	if err != nil {
		t.Fatalf("scan create: %v", err)
	}
	if len(changes) != 1 || changes[0].Op != model.MetaOpUpsert {
		t.Fatalf("expected 1 upsert, got %+v", changes)
	}
	firstHash := changes[0].ContentHash

	// Re-scan identical content -> no change.
	changes, err = sc.ScanPaths(ctx, "fld_1", root, []string{"a.txt"})
	if err != nil {
		t.Fatalf("scan noop: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected no change on identical content, got %+v", changes)
	}

	// Modify -> upsert with new hash.
	write(t, root, "a.txt", "hello world")
	changes, err = sc.ScanPaths(ctx, "fld_1", root, []string{"a.txt"})
	if err != nil {
		t.Fatalf("scan modify: %v", err)
	}
	if len(changes) != 1 || changes[0].ContentHash == firstHash {
		t.Fatalf("expected modified hash, got %+v", changes)
	}

	// Delete file on disk -> tombstone.
	if err := os.Remove(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	changes, err = sc.ScanPaths(ctx, "fld_1", root, []string{"a.txt"})
	if err != nil {
		t.Fatalf("scan delete: %v", err)
	}
	if len(changes) != 1 || changes[0].Op != model.MetaOpDelete {
		t.Fatalf("expected delete, got %+v", changes)
	}

	entry, err := idx.Get(ctx, "fld_1", "a.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !entry.Deleted {
		t.Fatal("entry should be tombstoned")
	}

	// Outbox should hold: upsert, upsert(modify), delete = 3 events.
	n, err := idx.OutboxCount(ctx, "fld_1")
	if err != nil {
		t.Fatalf("outbox count: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 queued events, got %d", n)
	}
}

func TestScanFolderReconcilesTree(t *testing.T) {
	ctx := context.Background()
	sc, idx, root := newScanner(t)

	write(t, root, "top.txt", "1")
	write(t, root, "sub/nested.txt", "2")

	changes, err := sc.ScanFolder(ctx, "fld_1", root)
	if err != nil {
		t.Fatalf("scan folder: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 upserts, got %d: %+v", len(changes), changes)
	}

	alive, tombstones, err := idx.Count(ctx, "fld_1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if alive != 2 || tombstones != 0 {
		t.Fatalf("alive=%d tombstones=%d", alive, tombstones)
	}

	// Remove one file, full scan should tombstone it.
	if err := os.Remove(filepath.Join(root, "sub", "nested.txt")); err != nil {
		t.Fatal(err)
	}
	changes, err = sc.ScanFolder(ctx, "fld_1", root)
	if err != nil {
		t.Fatalf("rescan: %v", err)
	}
	if len(changes) != 1 || changes[0].Op != model.MetaOpDelete || changes[0].Path != "sub/nested.txt" {
		t.Fatalf("expected 1 delete of sub/nested.txt, got %+v", changes)
	}

	alive, tombstones, _ = idx.Count(ctx, "fld_1")
	if alive != 1 || tombstones != 1 {
		t.Fatalf("after delete alive=%d tombstones=%d", alive, tombstones)
	}
}

func TestHashFileStable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	m1, err := HashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256("abc")
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if m1.ContentHash != want {
		t.Fatalf("hash = %s want %s", m1.ContentHash, want)
	}
	if m1.Size != 3 {
		t.Fatalf("size = %d want 3", m1.Size)
	}
}
