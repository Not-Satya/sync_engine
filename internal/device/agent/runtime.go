package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/hlc"
	"github.com/Not-Satya/sync_engine/internal/device/index"
	"github.com/Not-Satya/sync_engine/internal/device/scanner"
	"github.com/Not-Satya/sync_engine/internal/device/syncer"
	"github.com/Not-Satya/sync_engine/internal/device/watcher"
)

// DefaultReconcileInterval is the full-folder rescan safety net (ADR 18).
const DefaultReconcileInterval = 5 * time.Minute

// LoopConfig drives heartbeat + watch + scan + metadata push/pull.
type LoopConfig struct {
	Client    *client.Client
	Index     *index.Store
	Bindings  *bindings.Store
	DeviceID  string
	Heartbeat time.Duration
	SyncPoll  time.Duration
	Reconcile time.Duration
	Debounce  time.Duration
	Endpoint  string
	Logger    *log.Logger
}

// RunLoop runs presence heartbeats, fsnotify watchers, hash/scan, and
// coordinator metadata sync until ctx is cancelled. File bytes are never sent.
// New bindings added while running are picked up on the next reconcile tick.
func RunLoop(ctx context.Context, cfg LoopConfig) error {
	if cfg.Client == nil {
		return errNilClient
	}
	if cfg.Index == nil {
		return fmt.Errorf("agent: nil index")
	}
	if cfg.Bindings == nil {
		return fmt.Errorf("agent: nil bindings")
	}
	if cfg.DeviceID == "" {
		return fmt.Errorf("agent: device_id required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	clock := hlc.New()
	scan := scanner.New(cfg.Index, clock, cfg.DeviceID)

	heartbeat := cfg.Heartbeat
	if heartbeat <= 0 {
		heartbeat = DefaultHeartbeatInterval
	}
	syncPoll := cfg.SyncPoll
	if syncPoll <= 0 {
		syncPoll = syncer.DefaultPollInterval
	}
	reconcile := cfg.Reconcile
	if reconcile <= 0 {
		reconcile = DefaultReconcileInterval
	}

	rt := &runtime{
		cfg:    cfg,
		clock:  clock,
		scan:   scan,
		logger: logger,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := Run(ctx, Config{
			Client:   cfg.Client,
			Interval: heartbeat,
			Endpoint: cfg.Endpoint,
			Logger:   logger,
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Printf("heartbeat stopped: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		rt.watchAndReconcile(ctx, reconcile)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		rt.pollSync(ctx, syncPoll)
	}()

	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

type runtime struct {
	cfg    LoopConfig
	clock  *hlc.Clock
	scan   *scanner.Scanner
	logger *log.Logger

	mu      sync.Mutex
	watched map[string]struct{}
}

func (rt *runtime) watchAndReconcile(ctx context.Context, interval time.Duration) {
	rt.watched = make(map[string]struct{})
	rt.startMissingWatchers(ctx)

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rt.reconcileAll(ctx)
			rt.startMissingWatchers(ctx)
		}
	}
}

func (rt *runtime) startMissingWatchers(ctx context.Context) {
	for _, b := range rt.watchable() {
		rt.mu.Lock()
		_, already := rt.watched[b.FolderID]
		if !already {
			rt.watched[b.FolderID] = struct{}{}
		}
		rt.mu.Unlock()
		if already {
			continue
		}
		go rt.watchFolder(ctx, b)
	}
}

func (rt *runtime) watchable() []bindings.Binding {
	list, err := rt.cfg.Bindings.List()
	if err != nil {
		rt.logger.Printf("list bindings: %v", err)
		return nil
	}
	var out []bindings.Binding
	for _, b := range list {
		if !b.Subscribed {
			continue
		}
		if h, detail := bindings.CheckPath(b.LocalPath); h != bindings.PathOK {
			rt.logger.Printf("skip watch %s: %s (%s)", b.FolderID, h, detail)
			continue
		}
		out = append(out, b)
	}
	return out
}

func (rt *runtime) subscribedIDs() []string {
	list, err := rt.cfg.Bindings.List()
	if err != nil {
		return nil
	}
	var ids []string
	for _, b := range list {
		if b.Subscribed {
			ids = append(ids, b.FolderID)
		}
	}
	return ids
}

func (rt *runtime) watchFolder(ctx context.Context, b bindings.Binding) {
	w, err := watcher.New(watcher.Config{
		Root:     b.LocalPath,
		Debounce: rt.cfg.Debounce,
		Logger:   rt.logger,
	})
	if err != nil {
		rt.logger.Printf("watch %s: %v", b.FolderID, err)
		rt.mu.Lock()
		delete(rt.watched, b.FolderID)
		rt.mu.Unlock()
		return
	}
	out := make(chan watcher.Batch, 16)
	go func() {
		if err := w.Run(ctx, out); err != nil && !errors.Is(err, context.Canceled) {
			rt.logger.Printf("watch %s stopped: %v", b.FolderID, err)
		}
	}()

	rt.logger.Printf("watching %s path=%s", b.FolderID, b.LocalPath)
	rt.scanFolder(ctx, b)
	// Always push/pull once after the initial tree scan (even if nothing changed)
	// so a newly linked device catches up on remote metadata.
	rt.syncOne(ctx, b.FolderID)

	for {
		select {
		case <-ctx.Done():
			return
		case batch := <-out:
			rt.scanPaths(ctx, b, batch.Paths)
		}
	}
}

func (rt *runtime) reconcileAll(ctx context.Context) {
	for _, b := range rt.watchable() {
		rt.scanFolder(ctx, b)
	}
}

func (rt *runtime) scanFolder(ctx context.Context, b bindings.Binding) {
	changes, err := rt.scan.ScanFolder(ctx, b.FolderID, b.LocalPath)
	if err != nil {
		rt.logger.Printf("scan %s: %v", b.FolderID, err)
		return
	}
	if len(changes) == 0 {
		return
	}
	rt.logger.Printf("scan %s: %d change(s)", b.FolderID, len(changes))
	rt.syncOne(ctx, b.FolderID)
}

func (rt *runtime) scanPaths(ctx context.Context, b bindings.Binding, paths []string) {
	changes, err := rt.scan.ScanPaths(ctx, b.FolderID, b.LocalPath, paths)
	if err != nil {
		rt.logger.Printf("scan paths %s: %v", b.FolderID, err)
		return
	}
	if len(changes) == 0 {
		return
	}
	rt.logger.Printf("scan %s: %d change(s)", b.FolderID, len(changes))
	rt.syncOne(ctx, b.FolderID)
}

func (rt *runtime) pollSync(ctx context.Context, interval time.Duration) {
	tick := func() {
		for _, id := range rt.subscribedIDs() {
			rt.syncOne(ctx, id)
		}
	}
	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			tick()
		}
	}
}

func (rt *runtime) syncOne(ctx context.Context, folderID string) {
	res, err := syncer.SyncFolder(ctx, syncer.Config{
		Client:   rt.cfg.Client,
		Index:    rt.cfg.Index,
		Clock:    rt.clock,
		FolderID: folderID,
		Logger:   rt.logger,
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		rt.logger.Printf("sync %s: %v", folderID, err)
		return
	}
	if res.Pushed > 0 || res.Pulled > 0 {
		rt.logger.Printf("sync %s: pushed=%d pulled=%d applied=%d cursor=%d",
			folderID, res.Pushed, res.Pulled, res.Applied, res.Cursor)
	}
}
