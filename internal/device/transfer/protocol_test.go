package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPullFileRoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte("hello-p2p-"), 8000) // > one chunk
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	store := MapBlobStore{hash: payload}

	serverID := testIdentity(t)
	clientID := testIdentity(t)

	ln, err := Listen(ListenConfig{
		Addr:     "127.0.0.1:0",
		Identity: serverID,
		OnSession: func(ctx context.Context, sess *Session, conn net.Conn) {
			defer conn.Close()
			sc, err := NewSecureConn(conn, sess)
			if err != nil {
				t.Errorf("secure: %v", err)
				return
			}
			if err := ServePull(ctx, sc, store); err != nil {
				t.Errorf("serve: %v", err)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ln.Serve(ctx) }()

	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := PullFrom(ctx, ln.Endpoint(), clientID, nil, hash, dest); err != nil {
		t.Fatalf("pull: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("bytes mismatch: len got=%d want=%d", len(got), len(payload))
	}
	cancel()
}

func TestPullFileNotFound(t *testing.T) {
	serverID := testIdentity(t)
	clientID := testIdentity(t)
	store := MapBlobStore{}

	ln, err := Listen(ListenConfig{
		Addr:     "127.0.0.1:0",
		Identity: serverID,
		OnSession: func(ctx context.Context, sess *Session, conn net.Conn) {
			defer conn.Close()
			sc, _ := NewSecureConn(conn, sess)
			_ = ServePull(ctx, sc, store)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ln.Serve(ctx) }()

	dest := filepath.Join(t.TempDir(), "missing.bin")
	err = PullFrom(ctx, ln.Endpoint(), clientID, nil, hex.EncodeToString(make([]byte, 32)), dest)
	if err != ErrBlobNotFound {
		t.Fatalf("want ErrBlobNotFound, got %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("dest should not exist after failed pull")
	}
	cancel()
}

func TestPullFromPathBlobStore(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	content := []byte("path-backed-blob")
	if err := os.WriteFile(src, content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])
	store := PathBlobStore{hash: src}

	serverID := testIdentity(t)
	clientID := testIdentity(t)
	ln, err := Listen(ListenConfig{
		Addr:     "127.0.0.1:0",
		Identity: serverID,
		OnSession: func(ctx context.Context, sess *Session, conn net.Conn) {
			defer conn.Close()
			sc, _ := NewSecureConn(conn, sess)
			_ = ServePull(ctx, sc, store)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = ln.Serve(ctx) }()

	dest := filepath.Join(dir, "copied.txt")
	if err := PullFrom(ctx, ln.Endpoint(), clientID, nil, hash, dest); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, content) {
		t.Fatalf("got %q", got)
	}
}
