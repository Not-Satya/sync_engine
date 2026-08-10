package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

func TestPushAndPullFolderEvents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/folders/fld_1/events", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var body struct {
				Events []PushEvent `json:"events"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				http.Error(w, "bad json", 400)
				return
			}
			if len(body.Events) != 1 || body.Events[0].EventID != "evt_1" {
				http.Error(w, "unexpected body", 400)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accepted": []map[string]any{
					{"seq": 1, "event_id": "evt_1", "folder_id": "fld_1", "op": "upsert", "path": "a.txt"},
				},
				"max_seq": 1,
			})
		case http.MethodGet:
			if r.URL.Query().Get("since") != "0" {
				http.Error(w, "bad since", 400)
				return
			}
			mt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
			_ = json.NewEncoder(w).Encode(PullEventsResult{
				Events: []model.FolderEvent{{
					Seq:         1,
					EventID:     "evt_1",
					FolderID:    "fld_1",
					DeviceID:    "dev_peer",
					Op:          model.MetaOpUpsert,
					Path:        "a.txt",
					Size:        3,
					ContentHash: "abc",
					ModTime:     mt,
					HLC:         model.HLC{Wall: 100, Counter: 0},
				}},
				MaxSeq:  1,
				HasMore: false,
			})
		default:
			http.Error(w, "method", 405)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "tok")
	ctx := context.Background()

	mt := time.Now().UTC()
	push, err := c.PushFolderEvents(ctx, "fld_1", []PushEvent{{
		EventID: "evt_1",
		Op:      "upsert",
		Path:    "a.txt",
		Size:    3,
		HLC:     model.HLC{Wall: 50, Counter: 0},
		ModTime: &mt,
	}})
	if err != nil || len(push.Accepted) != 1 || push.MaxSeq != 1 {
		t.Fatalf("push: %+v %v", push, err)
	}

	pull, err := c.PullFolderEvents(ctx, "fld_1", 0, 50)
	if err != nil || len(pull.Events) != 1 || pull.Events[0].Path != "a.txt" {
		t.Fatalf("pull: %+v %v", pull, err)
	}
}
