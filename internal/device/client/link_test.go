package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterAndPairingCode(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/accounts", func(w http.ResponseWriter, r *http.Request) {
		priv := make([]byte, 64)
		pub := make([]byte, 32)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user_id": "usr_1",
			"token":   "tok_abc",
			"device_private_key_hex": hex.EncodeToString(priv),
			"device": map[string]any{
				"device_id":       "dev_1",
				"name":            "Laptop",
				"platform":        "windows",
				"public_key_hex":  hex.EncodeToString(pub),
			},
		})
	})
	mux.HandleFunc("/v1/pairing-codes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			http.Error(w, `{"error":"missing"}`, 401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":         "ABCD1234",
			"ttl_seconds":  600,
			"expires_at":   "2030-01-01T00:00:00Z",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "")
	link, err := c.Register(context.Background(), "a@b.com", "password123", DeviceInfo{Name: "Laptop"})
	if err != nil {
		t.Fatal(err)
	}
	if link.DeviceID != "dev_1" || link.Token != "tok_abc" || len(link.PrivateKey) != 64 {
		t.Fatalf("bad link: %+v", link)
	}

	c.Token = link.Token
	code, _, err := c.CreatePairingCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if code != "ABCD1234" {
		t.Fatalf("code=%q", code)
	}
}
