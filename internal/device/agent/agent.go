package agent

import (
	"context"
	"log"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/client"
)

// DefaultHeartbeatInterval keeps presence fresh under the server's 45s TTL.
const DefaultHeartbeatInterval = 20 * time.Second

// Config drives the long-running agent loop.
type Config struct {
	Client    *client.Client
	Interval  time.Duration
	Endpoint  string // optional dial hint for later P2P
	Logger    *log.Logger
}

// Run heartbeats until ctx is cancelled. Filesystem watch + metadata sync
// live in RunLoop (P4.6).
func Run(ctx context.Context, cfg Config) error {
	if cfg.Client == nil {
		return errNilClient
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	if err := beat(ctx, cfg, logger); err != nil {
		logger.Printf("heartbeat: %v", err)
	}

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := beat(ctx, cfg, logger); err != nil {
				logger.Printf("heartbeat: %v", err)
			}
		}
	}
}

func beat(ctx context.Context, cfg Config, logger *log.Logger) error {
	p, err := cfg.Client.Heartbeat(ctx, cfg.Endpoint)
	if err != nil {
		return err
	}
	logger.Printf("presence %s status=%s", p.DeviceID, p.Status)
	return nil
}

type nilClientError struct{}

func (nilClientError) Error() string { return "agent: nil client" }

var errNilClient nilClientError
