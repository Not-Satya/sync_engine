package watcher

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounce collapses filesystem event bursts before emitting a batch (ADR 18).
const DefaultDebounce = 400 * time.Millisecond

// Batch is a debounced set of changed relative paths within the watched root.
// Paths are forward-slash, relative to root. The watcher does not classify
// create/modify/delete — the hash/scan stage (P4.4) resolves that from disk.
type Batch struct {
	Paths []string
}

// Config configures a folder watcher.
type Config struct {
	Root     string        // absolute directory to watch
	Debounce time.Duration // quiescence window; defaults to DefaultDebounce
	Logger   *log.Logger
}

// Watcher observes one bound folder root recursively with debounced batching.
type Watcher struct {
	root     string
	debounce time.Duration
	logger   *log.Logger
	fsw      *fsnotify.Watcher
}

// New creates a watcher rooted at cfg.Root (does not start it).
func New(cfg Config) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	debounce := cfg.Debounce
	if debounce <= 0 {
		debounce = DefaultDebounce
	}
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		_ = fsw.Close()
		return nil, err
	}
	return &Watcher{
		root:     filepath.Clean(root),
		debounce: debounce,
		logger:   logger,
		fsw:      fsw,
	}, nil
}

// Run watches until ctx is cancelled, emitting debounced batches on out.
// fsnotify is not recursive, so subdirectories are added on start and as they appear.
func (w *Watcher) Run(ctx context.Context, out chan<- Batch) error {
	defer w.fsw.Close()

	if err := w.addTree(w.root); err != nil {
		return err
	}

	pending := make(map[string]struct{})
	var timer *time.Timer
	var timerC <-chan time.Time

	arm := func() {
		if timer == nil {
			timer = time.NewTimer(w.debounce)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(w.debounce)
		timerC = timer.C
	}

	flush := func() {
		if len(pending) == 0 {
			return
		}
		paths := make([]string, 0, len(pending))
		for p := range pending {
			paths = append(paths, p)
		}
		pending = make(map[string]struct{})
		select {
		case out <- Batch{Paths: paths}:
		case <-ctx.Done():
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case ev, ok := <-w.fsw.Events:
			if !ok {
				return nil
			}
			w.handleEvent(ev, pending)
			arm()

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return nil
			}
			w.logger.Printf("watch error: %v", err)

		case <-timerC:
			flush()
			timerC = nil
		}
	}
}

// Close releases the underlying watcher (safe if Run already returned).
func (w *Watcher) Close() error {
	return w.fsw.Close()
}

func (w *Watcher) handleEvent(ev fsnotify.Event, pending map[string]struct{}) {
	// New directory: start watching it so nested changes are seen.
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			if err := w.addTree(ev.Name); err != nil {
				w.logger.Printf("watch new dir %s: %v", ev.Name, err)
			}
		}
	}

	rel, ok := w.relPath(ev.Name)
	if !ok {
		return
	}
	pending[rel] = struct{}{}
}

// addTree adds dir and all existing subdirectories to the watch set.
func (w *Watcher) addTree(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// A path may vanish mid-walk; skip rather than abort.
			return nil
		}
		if d.IsDir() {
			if addErr := w.fsw.Add(path); addErr != nil {
				w.logger.Printf("watch add %s: %v", path, addErr)
			}
		}
		return nil
	})
}

// relPath converts an absolute event path to a forward-slash path relative to root.
func (w *Watcher) relPath(abs string) (string, bool) {
	rel, err := filepath.Rel(w.root, filepath.Clean(abs))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || rel == ".." {
		return "", false
	}
	return rel, true
}
