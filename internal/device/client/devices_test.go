package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListAndRevokeDevice(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"devices": []map[string]any{
				{"device_id": "dev_a", "name": "Laptop", "is_this_device": true, "revoked": false},
				{"device_id": "dev_b", "name": "Phone", "is_this_device": false, "revoked": false},
			},
			"active_count":   2,
			"total_count":    2,
			"this_device_id": "dev_a",
		})
	})
	mux.HandleFunc("/v1/devices/dev_b", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method", 405)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_id": "dev_b",
			"name":      "Phone",
			"revoked":   true,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "tok")
	list, err := c.ListDevices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list.ThisDeviceID != "dev_a" || len(list.Devices) != 2 {
		t.Fatalf("list: %+v", list)
	}
	rev, err := c.RevokeDevice(context.Background(), "dev_b")
	if err != nil {
		t.Fatal(err)
	}
	if !rev.Revoked || rev.DeviceID != "dev_b" {
		t.Fatalf("revoke: %+v", rev)
	}
}
