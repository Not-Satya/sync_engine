package agent

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/hlc"
	"github.com/Not-Satya/sync_engine/internal/device/index"
	"github.com/Not-Satya/sync_engine/internal/device/scanner"
	"github.com/Not-Satya/sync_engine/internal/device/syncer"
	"github.com/Not-Satya/sync_engine/internal/device/transfer"
	"github.com/Not-Satya/sync_engine/internal/device/watcher"
)

// DefaultReconcileInterval is the full-folder rescan safety net (ADR 18).
const DefaultReconcileInterval = 5 * time.Minute

// DefaultTransferListen is the default TCP bind for P2P byte transfer (ADR 23).
const DefaultTransferListen = transfer.DefaultListenAddr

// LoopConfig drives heartbeat + watch + scan + metadata push/pull + P2P transfer.
type LoopConfig struct {
	Client    *client.Client
	Index     *index.Store
	Bindings  *bindings.Store
	DeviceID  string
	Identity  transfer.Identity // required for transfer listen/fetch; zero disables P2P
	ListenAddr string           // TCP listen; empty uses DefaultTransferListen when Identity set
	Heartbeat time.Duration
	SyncPoll  time.Duration
	Reconcile time.Duration
	Debounce  time.Duration
	Endpoint  string // optional override for presence advertisement
	Logger    *log.Logger
}

// RunLoop runs presence heartbeats, fsnotify watchers, hash/scan, coordinator
// metadata sync, and (when Identity is set) P2P transfer listen + fetch.
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

	xferEnabled := cfg.Identity.Validate() == nil
	endpoint := cfg.Endpoint
	var planner *transfer.Planner
	if xferEnabled {
		planner = &transfer.Planner{
			Peers: cfg.Client,
			Index: cfg.Index,
			ID:    cfg.Identity,
		}
	}

	rt := &runtime{
		cfg:      cfg,
		clock:    clock,
		scan:     scan,
		planner:  planner,
		logger:   logger,
		endpoint: endpoint,
	}

	var wg sync.WaitGroup

	if xferEnabled {
		listenAddr := cfg.ListenAddr
		if listenAddr == "" {
			listenAddr = DefaultTransferListen
		}
		blob := transfer.IndexBlobStore{Index: cfg.Index, Bindings: cfg.Bindings}
		ln, err := transfer.Listen(transfer.ListenConfig{
			Addr:     listenAddr,
			Identity: cfg.Identity,
			Logger:   logger,
			OnSession: func(sessCtx context.Context, sess *transfer.Session, conn net.Conn) {
				defer conn.Close()
				sc, err := transfer.NewSecureConn(conn, sess)
				if err != nil {
					logger.Printf("transfer secure: %v", err)
					return
				}
				for {
					if err := transfer.ServePull(sessCtx, sc, blob); err != nil {
						return
					}
				}
			},
		})
		if err != nil {
			return fmt.Errorf("transfer listen: %w", err)
		}
		if endpoint == "" {
			endpoint = ln.Endpoint()
			rt.endpoint = endpoint
		}
		logger.Printf("transfer listening on %s (advertising %s)", ln.Endpoint(), endpoint)

		wg.Add(1)
		go func() {
			defer wg.Done()
			defer ln.Close()
			if err := ln.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Printf("transfer listener stopped: %v", err)
			}
		}()
	} else {
		logger.Printf("transfer disabled (no device identity / key material)")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := Run(ctx, Config{
			Client:   cfg.Client,
			Interval: heartbeat,
			Endpoint: endpoint,
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
	cfg      LoopConfig
	clock    *hlc.Clock
	scan     *scanner.Scanner
	planner  *transfer.Planner
	logger   *log.Logger
	endpoint string

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

func (rt *runtime) bindingFor(folderID string) (bindings.Binding, bool) {
	b, err := rt.cfg.Bindings.Get(folderID)
	if err != nil {
		return bindings.Binding{}, false
	}
	return b, true
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
	rt.fetchOne(ctx, folderID)
}

func (rt *runtime) fetchOne(ctx context.Context, folderID string) {
	if rt.planner == nil {
		return
	}
	b, ok := rt.bindingFor(folderID)
	if !ok || !b.Subscribed {
		return
	}
	if h, _ := bindings.CheckPath(b.LocalPath); h != bindings.PathOK {
		return
	}
	results, err := rt.planner.FetchFolder(ctx, folderID, b.LocalPath)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		rt.logger.Printf("fetch %s: %v", folderID, err)
		return
	}
	fetched, failed := 0, 0
	for _, r := range results {
		if r.Fetched {
			fetched++
			rt.logger.Printf("fetch %s: got %s from %s", folderID, r.Candidate.Entry.Path, r.Peer.DeviceID)
			continue
		}
		failed++
		if r.Err != nil {
			rt.logger.Printf("fetch %s: %s: %v", folderID, r.Candidate.Entry.Path, r.Err)
		}
	}
	if fetched > 0 || failed > 0 {
		rt.logger.Printf("fetch %s: fetched=%d pending=%d", folderID, fetched, failed)
	}
}

// IdentityFromKeyMaterial builds a transfer.Identity from raw keystore bytes.
func IdentityFromKeyMaterial(deviceID string, pub, priv []byte) (transfer.Identity, error) {
	id := transfer.Identity{
		DeviceID:   deviceID,
		PublicKey:  ed25519.PublicKey(append([]byte(nil), pub...)),
		PrivateKey: ed25519.PrivateKey(append([]byte(nil), priv...)),
	}
	if err := id.Validate(); err != nil {
		return transfer.Identity{}, err
	}
	return id, nil
}
