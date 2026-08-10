package index

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), FileName))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestOutboxEnqueueListAck(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)

	items := []OutboxItem{
		{EventID: "evt_b", FolderID: "fld_1", Op: model.MetaOpUpsert, Path: "b.txt", HLCWall: 100, HLCCounter: 0, ModTime: time.Now()},
		{EventID: "evt_a", FolderID: "fld_1", Op: model.MetaOpUpsert, Path: "a.txt", HLCWall: 50, HLCCounter: 0},
		{EventID: "evt_c", FolderID: "fld_2", Op: model.MetaOpDelete, Path: "c.txt", HLCWall: 10, HLCCounter: 0},
	}
	for _, it := range items {
		if err := st.EnqueueOutbox(ctx, it); err != nil {
			t.Fatalf("enqueue %s: %v", it.EventID, err)
		}
	}

	// Duplicate event_id is ignored.
	if err := st.EnqueueOutbox(ctx, items[0]); err != nil {
		t.Fatalf("dup enqueue: %v", err)
	}

	got, err := st.ListOutbox(ctx, "fld_1", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 items for fld_1, got %d", len(got))
	}
	// Ordered by HLC wall ascending: evt_a (50) then evt_b (100).
	if got[0].EventID != "evt_a" || got[1].EventID != "evt_b" {
		t.Fatalf("bad order: %s %s", got[0].EventID, got[1].EventID)
	}

	n, err := st.OutboxCount(ctx, "fld_1")
	if err != nil || n != 2 {
		t.Fatalf("count fld_1 = %d err=%v", n, err)
	}

	if err := st.AckOutbox(ctx, []string{"evt_a", "evt_b"}); err != nil {
		t.Fatalf("ack: %v", err)
	}
	n, _ = st.OutboxCount(ctx, "fld_1")
	if n != 0 {
		t.Fatalf("fld_1 should be drained, got %d", n)
	}
	// Other folder untouched.
	n, _ = st.OutboxCount(ctx, "fld_2")
	if n != 1 {
		t.Fatalf("fld_2 should still have 1, got %d", n)
	}
}
