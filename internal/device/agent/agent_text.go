package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/client"
)

func TestRunHeartbeatsUntilCancel(t *testing.T) {
	var n atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/presence/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_id": "dev_1",
			"status":    "online",
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Client:   client.New(srv.URL, "tok"),
			Interval: 30 * time.Millisecond,
		})
	}()

	deadline := time.Now().Add(2 * time.Second)
	for n.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	err := <-done
	if err != nil && err != context.Canceled {
		t.Fatalf("run: %v", err)
	}
	if n.Load() < 2 {
		t.Fatalf("expected >=2 heartbeats, got %d", n.Load())
	}
}
