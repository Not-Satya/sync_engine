package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMeAndHeartbeat(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok_test" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": "usr_1",
			"email":   "a@example.com",
			"device":  map[string]any{"device_id": "dev_1", "name": "Laptop"},
		})
	})
	mux.HandleFunc("/v1/presence/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_id": "dev_1",
			"status":    "online",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "tok_test")
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.UserID != "usr_1" || me.Email != "a@example.com" {
		t.Fatalf("unexpected me: %+v", me)
	}
	p, err := c.Heartbeat(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "online" {
		t.Fatalf("presence: %+v", p)
	}
}
