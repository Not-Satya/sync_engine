package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func collect(t *testing.T, ctx context.Context, out <-chan Batch, wantPath string) bool {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case b := <-out:
			for _, p := range b.Paths {
				if p == wantPath {
					return true
				}
			}
		case <-deadline:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

func TestWatcherDetectsCreateAndNestedDir(t *testing.T) {
	root := t.TempDir()
	w, err := New(Config{Root: root, Debounce: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan Batch, 8)
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx, out) }()

	// Give Run a moment to add the initial tree.
	time.Sleep(150 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !collect(t, ctx, out, "a.txt") {
		t.Fatal("did not observe a.txt create")
	}

	// New subdir + nested file should be watched after dir creation.
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("yo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !collect(t, ctx, out, "sub/b.txt") {
		t.Fatal("did not observe nested sub/b.txt create")
	}

	cancel()
	<-done
}

func TestRelPathRejectsOutside(t *testing.T) {
	root := t.TempDir()
	w, err := New(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	if _, ok := w.relPath(root); ok {
		t.Fatal("root itself should be rejected")
	}
	if p, ok := w.relPath(filepath.Join(root, "x", "y.txt")); !ok || p != "x/y.txt" {
		t.Fatalf("rel: %q %v", p, ok)
	}
}
