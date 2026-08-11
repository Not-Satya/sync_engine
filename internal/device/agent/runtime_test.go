package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

func TestRunLoopScansAndPushes(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "sync")
	if err := os.Mkdir(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "hello.txt"), []byte("hi"), 0o600); err != nil {
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

	var pushes atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/presence/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"device_id": "dev_1", "status": "online"})
	})
	mux.HandleFunc("/v1/folders/fld_1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			pushes.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"accepted": []any{}, "max_seq": 1})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": []any{}, "max_seq": 0, "has_more": false,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunLoop(ctx, LoopConfig{
			Client:    client.New(srv.URL, "tok"),
			Index:     idx,
			Bindings:  bind,
			DeviceID:  "dev_1",
			Heartbeat: time.Hour,
			SyncPoll:  40 * time.Millisecond,
			Reconcile: time.Hour,
			Debounce:  50 * time.Millisecond,
		})
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, _ := idx.OutboxCount(context.Background(), "fld_1")
		if pushes.Load() >= 1 && n == 0 {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	cancel()
	err = <-done
	if err != nil && err != context.Canceled {
		t.Fatalf("run: %v", err)
	}
	if pushes.Load() < 1 {
		t.Fatal("expected at least one outbox push after initial scan")
	}

	e, err := idx.Get(context.Background(), "fld_1", "hello.txt")
	if err != nil || e.Size != 2 {
		t.Fatalf("index entry: %+v %v", e, err)
	}
	n, _ := idx.OutboxCount(context.Background(), "fld_1")
	if n != 0 {
		t.Fatalf("outbox should be acked after push, got %d", n)
	}
}
