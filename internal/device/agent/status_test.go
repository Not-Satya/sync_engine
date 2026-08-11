package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

func TestCollectFolderReports(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "movies")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	bind := bindings.Open(filepath.Join(root, "bindings.json"))
	if err := bind.Put(bindings.Binding{
		FolderID: "fld_1", LocalPath: dir, Name: "Movies", Subscribed: true,
	}); err != nil {
		t.Fatal(err)
	}

	idx, err := index.Open(filepath.Join(root, "meta", index.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = idx.Close() })

	if err := idx.Upsert(ctx, index.Entry{
		FolderID: "fld_1", Path: "a.txt", Size: 1, ContentHash: "aa",
		HLCWall: 1, DeviceID: "dev",
	}); err != nil {
		t.Fatal(err)
	}
	if err := idx.SetCursor(ctx, "fld_1", 9); err != nil {
		t.Fatal(err)
	}

	reps, err := CollectFolderReports(ctx, bind, idx)
	if err != nil || len(reps) != 1 {
		t.Fatalf("reports: %+v %v", reps, err)
	}
	if reps[0].Alive != 1 || reps[0].Cursor != 9 || reps[0].Outbox != 0 {
		t.Fatalf("stats: %+v", reps[0])
	}
	s := reps[0].String()
	if !strings.Contains(s, "files=1") || !strings.Contains(s, "cursor=9") {
		t.Fatalf("string: %s", s)
	}
}
