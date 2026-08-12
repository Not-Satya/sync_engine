package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListFolderPeers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/folders/fld_1/peers", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"peers": []map[string]any{
				{
					"device_id":      "dev_peer",
					"name":           "Laptop",
					"platform":       "windows",
					"endpoint":       "127.0.0.1:7900",
					"public_key_hex": "abcd",
					"status":         "online",
				},
				{
					"device_id": "dev_quiet",
					"name":      "Phone",
					"status":    "online",
					"endpoint":  "",
				},
			},
			"ttl_seconds": 45,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "tok")
	peers, err := c.ListFolderPeers(context.Background(), "fld_1")
	if err != nil || len(peers) != 2 {
		t.Fatalf("peers: %+v %v", peers, err)
	}
	dialable := DialablePeers(peers)
	if len(dialable) != 1 || dialable[0].Endpoint != "127.0.0.1:7900" {
		t.Fatalf("dialable: %+v", dialable)
	}
}
