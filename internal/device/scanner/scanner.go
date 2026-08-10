// Package scanner turns filesystem changes into local index updates plus
// coordinator-bound metadata events (the outbox). It hashes files with SHA-256
// (ADR 17), stamps each mutation with an HLC (ADR 16), and never moves bytes.
package scanner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/device/hlc"
	"github.com/Not-Satya/sync_engine/internal/device/index"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

// FileMeta is the result of hashing one on-disk file.
type FileMeta struct {
	Size        int64
	ContentHash string
	ModTime     time.Time
}

// Change records a metadata mutation the scanner detected and applied locally.
type Change struct {
	Path        string
	Op          model.MetaOp
	Size        int64
	ContentHash string
	ModTime     time.Time
}

// Scanner reconciles a bound folder's on-disk state against the local index.
type Scanner struct {
	idx      *index.Store
	clock    *hlc.Clock
	deviceID string
}

// New builds a scanner. deviceID is the last-writer tag used for LWW tie-breaks.
func New(idx *index.Store, clock *hlc.Clock, deviceID string) *Scanner {
	return &Scanner{idx: idx, clock: clock, deviceID: deviceID}
}

// HashFile stats and SHA-256 hashes a regular file.
func HashFile(path string) (FileMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileMeta{}, err
	}
	if info.IsDir() {
		return FileMeta{}, fmt.Errorf("hash: %s is a directory", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return FileMeta{}, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return FileMeta{}, err
	}
	return FileMeta{
		Size:        info.Size(),
		ContentHash: hex.EncodeToString(h.Sum(nil)),
		ModTime:     info.ModTime().UTC(),
	}, nil
}

// ScanPaths reconciles a specific set of relative paths (e.g. from a watcher
// batch) within folderID rooted at root. Directories and unreadable entries are
// skipped. Returns the changes that were applied to the index and enqueued.
func (s *Scanner) ScanPaths(ctx context.Context, folderID, root string, relPaths []string) ([]Change, error) {
	var changes []Change
	for _, rel := range relPaths {
		norm, err := index.NormalizePath(rel)
		if err != nil {
			continue // ignore paths we can't represent (root itself, traversal, etc.)
		}
		ch, ok, err := s.reconcilePath(ctx, folderID, root, norm)
		if err != nil {
			return changes, err
		}
		if ok {
			changes = append(changes, ch)
		}
	}
	return changes, nil
}

// ScanFolder does a full reconcile: every file on disk is hashed and compared,
// and any live index entry whose file has vanished is tombstoned. This is the
// periodic safety net from ADR 18.
func (s *Scanner) ScanFolder(ctx context.Context, folderID, root string) ([]Change, error) {
	onDisk := make(map[string]struct{})
	var changes []Change

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip vanished/permission paths
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		norm, err := index.NormalizePath(rel)
		if err != nil {
			return nil
		}
		onDisk[norm] = struct{}{}
		ch, ok, err := s.reconcilePath(ctx, folderID, root, norm)
		if err != nil {
			return err
		}
		if ok {
			changes = append(changes, ch)
		}
		return nil
	})
	if err != nil {
		return changes, err
	}

	// Tombstone live entries whose files are gone.
	entries, err := s.idx.List(ctx, folderID)
	if err != nil {
		return changes, err
	}
	for _, e := range entries {
		if _, still := onDisk[e.Path]; still {
			continue
		}
		ch, ok, err := s.applyDelete(ctx, folderID, e.Path)
		if err != nil {
			return changes, err
		}
		if ok {
			changes = append(changes, ch)
		}
	}
	return changes, nil
}

// reconcilePath resolves one normalized relative path: hash+upsert if present
// and changed, tombstone if it disappeared. ok is false when nothing changed.
func (s *Scanner) reconcilePath(ctx context.Context, folderID, root, norm string) (Change, bool, error) {
	abs := filepath.Join(root, filepath.FromSlash(norm))
	info, statErr := os.Stat(abs)
	switch {
	case statErr == nil && info.IsDir():
		return Change{}, false, nil // directories are not tracked as entries
	case statErr == nil:
		return s.applyUpsert(ctx, folderID, norm, abs)
	case errors.Is(statErr, os.ErrNotExist):
		return s.applyDelete(ctx, folderID, norm)
	default:
		return Change{}, false, statErr
	}
}

func (s *Scanner) applyUpsert(ctx context.Context, folderID, path, abs string) (Change, bool, error) {
	meta, err := HashFile(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s.applyDelete(ctx, folderID, path)
		}
		return Change{}, false, err
	}

	existing, err := s.idx.Get(ctx, folderID, path)
	if err != nil && !errors.Is(err, index.ErrNotFound) {
		return Change{}, false, err
	}
	unchanged := err == nil && !existing.Deleted &&
		existing.ContentHash == meta.ContentHash && existing.Size == meta.Size
	if unchanged {
		return Change{}, false, nil
	}

	wall, counter := s.clock.Now()
	entry := index.Entry{
		FolderID:    folderID,
		Path:        path,
		Size:        meta.Size,
		ContentHash: meta.ContentHash,
		ModTime:     meta.ModTime,
		HLCWall:     wall,
		HLCCounter:  counter,
		Deleted:     false,
		DeviceID:    s.deviceID,
	}
	if err := s.idx.Upsert(ctx, entry); err != nil {
		return Change{}, false, err
	}
	if err := s.enqueue(ctx, model.MetaOpUpsert, entry); err != nil {
		return Change{}, false, err
	}
	return Change{
		Path:        path,
		Op:          model.MetaOpUpsert,
		Size:        meta.Size,
		ContentHash: meta.ContentHash,
		ModTime:     meta.ModTime,
	}, true, nil
}

func (s *Scanner) applyDelete(ctx context.Context, folderID, path string) (Change, bool, error) {
	existing, err := s.idx.Get(ctx, folderID, path)
	if errors.Is(err, index.ErrNotFound) {
		return Change{}, false, nil // never tracked; nothing to delete
	}
	if err != nil {
		return Change{}, false, err
	}
	if existing.Deleted {
		return Change{}, false, nil // already a tombstone
	}

	wall, counter := s.clock.Now()
	entry := index.Entry{
		FolderID:   folderID,
		Path:       path,
		HLCWall:    wall,
		HLCCounter: counter,
		Deleted:    true,
		DeviceID:   s.deviceID,
	}
	if err := s.idx.Upsert(ctx, entry); err != nil {
		return Change{}, false, err
	}
	if err := s.enqueue(ctx, model.MetaOpDelete, entry); err != nil {
		return Change{}, false, err
	}
	return Change{Path: path, Op: model.MetaOpDelete}, true, nil
}

func (s *Scanner) enqueue(ctx context.Context, op model.MetaOp, e index.Entry) error {
	eventID, err := ids.NewEventID()
	if err != nil {
		return err
	}
	return s.idx.EnqueueOutbox(ctx, index.OutboxItem{
		EventID:     eventID,
		FolderID:    e.FolderID,
		Op:          op,
		Path:        e.Path,
		Size:        e.Size,
		ContentHash: e.ContentHash,
		ModTime:     e.ModTime,
		HLCWall:     e.HLCWall,
		HLCCounter:  e.HLCCounter,
	})
}
