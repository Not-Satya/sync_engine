// Package syncer pushes local outbox metadata to the coordinator and pulls
// remote folder events into the device index (ADR 19). File bytes are never
// transferred here — only names, hashes, sizes, and HLC stamps.
package syncer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/hlc"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

const (
	// DefaultPollInterval is how often Run re-syncs each folder while online.
	DefaultPollInterval = 5 * time.Second
	// DefaultPushBatch is the max outbox events sent per POST.
	DefaultPushBatch = 200
	// DefaultPullLimit is the max events requested per GET page.
	DefaultPullLimit = 200
)

// Config drives a metadata sync session for one or more folders.
type Config struct {
	Client   *client.Client
	Index    *index.Store
	Clock    *hlc.Clock
	FolderID string // required for SyncFolder; ignored by Run which takes a list
	Logger   *log.Logger
}

// Result summarizes one SyncFolder pass.
type Result struct {
	Pushed   int
	Pulled   int
	Applied  int
	Cursor   int64
}

// SyncFolder drains the outbox for folderID, then pulls and applies remote
// events until the coordinator reports no more pages (ADR 19).
func SyncFolder(ctx context.Context, cfg Config) (Result, error) {
	if cfg.Client == nil {
		return Result{}, fmt.Errorf("syncer: nil client")
	}
	if cfg.Index == nil {
		return Result{}, fmt.Errorf("syncer: nil index")
	}
	if cfg.Clock == nil {
		return Result{}, fmt.Errorf("syncer: nil clock")
	}
	if cfg.FolderID == "" {
		return Result{}, fmt.Errorf("syncer: folder_id required")
	}

	var res Result
	pushed, err := pushOutbox(ctx, cfg)
	if err != nil {
		return res, err
	}
	res.Pushed = pushed

	pulled, applied, cursor, err := pullApply(ctx, cfg)
	if err != nil {
		return res, err
	}
	res.Pulled = pulled
	res.Applied = applied
	res.Cursor = cursor
	return res, nil
}

// Run periodically SyncFolders until ctx is cancelled. folderIDs are synced
// in order each tick. Failures on one folder are logged; others still run.
func Run(ctx context.Context, cfg Config, folderIDs []string, interval time.Duration) error {
	if cfg.Client == nil || cfg.Index == nil || cfg.Clock == nil {
		return fmt.Errorf("syncer: client, index, and clock required")
	}
	if interval <= 0 {
		interval = DefaultPollInterval
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	tick := func() {
		for _, id := range folderIDs {
			pass := cfg
			pass.FolderID = id
			res, err := SyncFolder(ctx, pass)
			if err != nil {
				logger.Printf("sync %s: %v", id, err)
				continue
			}
			if res.Pushed > 0 || res.Pulled > 0 {
				logger.Printf("sync %s: pushed=%d pulled=%d applied=%d cursor=%d",
					id, res.Pushed, res.Pulled, res.Applied, res.Cursor)
			}
		}
	}

	tick()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			tick()
		}
	}
}

func pushOutbox(ctx context.Context, cfg Config) (int, error) {
	total := 0
	for {
		items, err := cfg.Index.ListOutbox(ctx, cfg.FolderID, DefaultPushBatch)
		if err != nil {
			return total, err
		}
		if len(items) == 0 {
			return total, nil
		}

		events := make([]client.PushEvent, 0, len(items))
		ids := make([]string, 0, len(items))
		for _, it := range items {
			ev := client.PushEvent{
				EventID:     it.EventID,
				Op:          string(it.Op),
				Path:        it.Path,
				OldPath:     it.OldPath,
				Size:        it.Size,
				ContentHash: it.ContentHash,
				HLC:         model.HLC{Wall: it.HLCWall, Counter: it.HLCCounter},
			}
			if !it.ModTime.IsZero() {
				mt := it.ModTime.UTC()
				ev.ModTime = &mt
			}
			events = append(events, ev)
			ids = append(ids, it.EventID)
		}

		if _, err := cfg.Client.PushFolderEvents(ctx, cfg.FolderID, events); err != nil {
			return total, err
		}
		// Ack the whole batch on HTTP success. Duplicates already on the
		// server are omitted from "accepted" but must leave the outbox.
		if err := cfg.Index.AckOutbox(ctx, ids); err != nil {
			return total, err
		}
		total += len(ids)
		if len(items) < DefaultPushBatch {
			return total, nil
		}
	}
}

func pullApply(ctx context.Context, cfg Config) (pulled, applied int, cursor int64, err error) {
	cursor, err = cfg.Index.Cursor(ctx, cfg.FolderID)
	if err != nil {
		return 0, 0, 0, err
	}

	for {
		page, err := cfg.Client.PullFolderEvents(ctx, cfg.FolderID, cursor, DefaultPullLimit)
		if err != nil {
			return pulled, applied, cursor, err
		}
		if len(page.Events) == 0 {
			return pulled, applied, cursor, nil
		}

		var maxSeq int64 = cursor
		for _, ev := range page.Events {
			cfg.Clock.Observe(ev.HLC.Wall, ev.HLC.Counter)
			n, err := applyEvent(ctx, cfg.Index, ev)
			if err != nil {
				return pulled, applied, cursor, fmt.Errorf("apply seq=%d: %w", ev.Seq, err)
			}
			applied += n
			if ev.Seq > maxSeq {
				maxSeq = ev.Seq
			}
		}
		pulled += len(page.Events)
		if err := cfg.Index.SetCursor(ctx, cfg.FolderID, maxSeq); err != nil {
			return pulled, applied, cursor, err
		}
		cursor = maxSeq

		if !page.HasMore {
			return pulled, applied, cursor, nil
		}
	}
}

func applyEvent(ctx context.Context, idx *index.Store, ev model.FolderEvent) (int, error) {
	applied := 0
	switch ev.Op {
	case model.MetaOpUpsert:
		ok, err := idx.ApplyRemote(ctx, entryFromEvent(ev, false))
		if err != nil {
			return 0, err
		}
		if ok {
			applied++
		}
	case model.MetaOpDelete:
		ok, err := idx.ApplyRemote(ctx, entryFromEvent(ev, true))
		if err != nil {
			return 0, err
		}
		if ok {
			applied++
		}
	case model.MetaOpRename:
		if ev.OldPath != "" {
			old := entryFromEvent(ev, true)
			old.Path = ev.OldPath
			old.Size = 0
			old.ContentHash = ""
			ok, err := idx.ApplyRemote(ctx, old)
			if err != nil {
				return 0, err
			}
			if ok {
				applied++
			}
		}
		ok, err := idx.ApplyRemote(ctx, entryFromEvent(ev, false))
		if err != nil {
			return applied, err
		}
		if ok {
			applied++
		}
	default:
		return 0, fmt.Errorf("unknown op %q", ev.Op)
	}
	return applied, nil
}

func entryFromEvent(ev model.FolderEvent, deleted bool) index.Entry {
	return index.Entry{
		FolderID:    ev.FolderID,
		Path:        ev.Path,
		Size:        ev.Size,
		ContentHash: ev.ContentHash,
		ModTime:     ev.ModTime,
		HLCWall:     ev.HLC.Wall,
		HLCCounter:  ev.HLC.Counter,
		Deleted:     deleted,
		DeviceID:    ev.DeviceID,
	}
}
