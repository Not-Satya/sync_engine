package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFoldersAndSubscriptions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/folders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"folder_id":  "fld_1",
				"owner_id":   "usr_1",
				"name":       "Movies",
				"created_at": "2026-01-01T00:00:00Z",
			})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"folders": []map[string]any{
					{"folder_id": "fld_1", "owner_id": "usr_1", "name": "Movies"},
				},
			})
		default:
			http.Error(w, "method", 405)
		}
	})
	mux.HandleFunc("/v1/folders/fld_1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"folder_id":     "fld_1",
				"device_id":     "dev_1",
				"subscribed_at": "2026-01-01T00:00:00Z",
			})
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method", 405)
		}
	})
	mux.HandleFunc("/v1/subscriptions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"subscriptions": []map[string]any{
				{"folder_id": "fld_1", "device_id": "dev_1"},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "tok")
	ctx := context.Background()

	f, err := c.CreateFolder(ctx, "Movies")
	if err != nil || f.FolderID != "fld_1" {
		t.Fatalf("create: %+v %v", f, err)
	}
	list, err := c.ListFolders(ctx)
	if err != nil || len(list) != 1 || list[0].Name != "Movies" {
		t.Fatalf("list: %+v %v", list, err)
	}
	sub, err := c.SubscribeFolder(ctx, "fld_1")
	if err != nil || sub.FolderID != "fld_1" {
		t.Fatalf("subscribe: %+v %v", sub, err)
	}
	subs, err := c.ListSubscriptions(ctx)
	if err != nil || len(subs) != 1 {
		t.Fatalf("subscriptions: %+v %v", subs, err)
	}
	if err := c.UnsubscribeFolder(ctx, "fld_1"); err != nil {
		t.Fatal(err)
	}
}
