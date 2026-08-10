package syncer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/hlc"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

func openIndex(t *testing.T) *index.Store {
	t.Helper()
	st, err := index.Open(filepath.Join(t.TempDir(), index.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestSyncFolderPushThenPullApply(t *testing.T) {
	ctx := context.Background()
	idx := openIndex(t)

	if err := idx.EnqueueOutbox(ctx, index.OutboxItem{
		EventID:     "evt_local",
		FolderID:    "fld_1",
		Op:          model.MetaOpUpsert,
		Path:        "local.txt",
		Size:        1,
		ContentHash: "aa",
		HLCWall:     10,
		HLCCounter:  0,
	}); err != nil {
		t.Fatal(err)
	}

	var (
		mu        sync.Mutex
		pushedIDs []string
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/folders/fld_1/events", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Events []client.PushEvent `json:"events"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			for _, e := range body.Events {
				pushedIDs = append(pushedIDs, e.EventID)
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accepted": body.Events,
				"max_seq":  1,
			})
		case http.MethodGet:
			since := r.URL.Query().Get("since")
			if since == "0" {
				_ = json.NewEncoder(w).Encode(client.PullEventsResult{
					Events: []model.FolderEvent{{
						Seq:         5,
						EventID:     "evt_remote",
						FolderID:    "fld_1",
						DeviceID:    "dev_peer",
						Op:          model.MetaOpUpsert,
						Path:        "remote.txt",
						Size:        4,
						ContentHash: "bb",
						HLC:         model.HLC{Wall: 1000, Counter: 2},
					}},
					MaxSeq:  5,
					HasMore: false,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(client.PullEventsResult{
				Events:  []model.FolderEvent{},
				MaxSeq:  5,
				HasMore: false,
			})
		default:
			http.Error(w, "method", 405)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	clock := hlc.New()
	res, err := SyncFolder(ctx, Config{
		Client:   client.New(srv.URL, "tok"),
		Index:    idx,
		Clock:    clock,
		FolderID: "fld_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 1 {
		t.Fatalf("pushed=%d want 1", res.Pushed)
	}
	if res.Pulled != 1 || res.Applied != 1 {
		t.Fatalf("pulled=%d applied=%d want 1/1", res.Pulled, res.Applied)
	}
	if res.Cursor != 5 {
		t.Fatalf("cursor=%d want 5", res.Cursor)
	}

	n, err := idx.OutboxCount(ctx, "fld_1")
	if err != nil || n != 0 {
		t.Fatalf("outbox should be drained, n=%d err=%v", n, err)
	}

	entry, err := idx.Get(ctx, "fld_1", "remote.txt")
	if err != nil || entry.ContentHash != "bb" || entry.DeviceID != "dev_peer" {
		t.Fatalf("remote entry: %+v %v", entry, err)
	}

	cur, err := idx.Cursor(ctx, "fld_1")
	if err != nil || cur != 5 {
		t.Fatalf("stored cursor=%d err=%v", cur, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(pushedIDs) != 1 || pushedIDs[0] != "evt_local" {
		t.Fatalf("pushed ids: %v", pushedIDs)
	}
}

func TestApplyRemoteLWWRejectsOlder(t *testing.T) {
	ctx := context.Background()
	idx := openIndex(t)

	// Local newer entry already present.
	if err := idx.Upsert(ctx, index.Entry{
		FolderID: "fld_1", Path: "x.txt", Size: 1, ContentHash: "new",
		HLCWall: 500, HLCCounter: 0, DeviceID: "dev_local",
	}); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/folders/fld_1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{"accepted": []any{}, "max_seq": 0})
			return
		}
		_ = json.NewEncoder(w).Encode(client.PullEventsResult{
			Events: []model.FolderEvent{{
				Seq: 1, EventID: "evt_old", FolderID: "fld_1", DeviceID: "dev_peer",
				Op: model.MetaOpUpsert, Path: "x.txt", Size: 9, ContentHash: "old",
				HLC: model.HLC{Wall: 100, Counter: 0},
			}},
			MaxSeq: 1,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	res, err := SyncFolder(ctx, Config{
		Client:   client.New(srv.URL, "tok"),
		Index:    idx,
		Clock:    hlc.New(),
		FolderID: "fld_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pulled != 1 || res.Applied != 0 {
		t.Fatalf("want pulled=1 applied=0, got pulled=%d applied=%d", res.Pulled, res.Applied)
	}
	e, _ := idx.Get(ctx, "fld_1", "x.txt")
	if e.ContentHash != "new" {
		t.Fatalf("LWW should keep local newer hash, got %s", e.ContentHash)
	}
	// Cursor still advances past the observed older event.
	if res.Cursor != 1 {
		t.Fatalf("cursor=%d want 1", res.Cursor)
	}
}

func TestRunPollsUntilCancel(t *testing.T) {
	var pulls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/folders/fld_1/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			pulls.Add(1)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events":   []any{},
			"max_seq":  0,
			"has_more": false,
			"accepted": []any{},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	idx := openIndex(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Client: client.New(srv.URL, "tok"),
			Index:  idx,
			Clock:  hlc.New(),
		}, []string{"fld_1"}, 25*time.Millisecond)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for pulls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
	}
	cancel()
	err := <-done
	if err != nil && err != context.Canceled {
		t.Fatalf("run: %v", err)
	}
	if pulls.Load() < 2 {
		t.Fatalf("expected >=2 pulls, got %d", pulls.Load())
	}
}
