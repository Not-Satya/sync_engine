package transfer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"net"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/ids"
)

func testIdentity(t *testing.T) Identity {
	t.Helper()
	k, err := ids.NewDeviceKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	return Identity{
		DeviceID:   k.DeviceID,
		PublicKey:  k.PublicKey,
		PrivateKey: k.PrivateKey,
	}
}

func TestHandshakeMutualLocalhost(t *testing.T) {
	serverID := testIdentity(t)
	clientID := testIdentity(t)

	inbound := make(chan *Session, 1)
	ln, err := Listen(ListenConfig{
		Addr:     "127.0.0.1:0",
		Identity: serverID,
		Allow: func(deviceID string, pub ed25519.PublicKey) bool {
			return deviceID == clientID.DeviceID
		},
		OnSession: func(ctx context.Context, sess *Session, conn net.Conn) {
			defer conn.Close()
			inbound <- sess
			<-ctx.Done()
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- ln.Serve(ctx) }()

	sess, conn, err := Dial(ctx, ln.Endpoint(), clientID, func(deviceID string, pub ed25519.PublicKey) bool {
		return deviceID == serverID.DeviceID
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if sess.PeerDeviceID != serverID.DeviceID {
		t.Fatalf("client peer=%s want %s", sess.PeerDeviceID, serverID.DeviceID)
	}
	if !sess.Dialer {
		t.Fatal("client should be dialer")
	}
	if len(sess.SessionKey) != 32 {
		t.Fatalf("session key len=%d", len(sess.SessionKey))
	}

	select {
	case peer := <-inbound:
		if peer.PeerDeviceID != clientID.DeviceID {
			t.Fatalf("server peer=%s want %s", peer.PeerDeviceID, clientID.DeviceID)
		}
		if peer.Dialer {
			t.Fatal("server should not be dialer")
		}
		if !bytes.Equal(peer.SessionKey, sess.SessionKey) {
			t.Fatal("session keys differ")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for inbound session")
	}

	cancel()
	<-errCh
}

func TestHandshakeRejectsWrongAllowlist(t *testing.T) {
	serverID := testIdentity(t)
	clientID := testIdentity(t)

	ln, err := Listen(ListenConfig{
		Addr:     "127.0.0.1:0",
		Identity: serverID,
		Allow:    func(string, ed25519.PublicKey) bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ln.Serve(ctx) }()

	_, conn, err := Dial(context.Background(), ln.Endpoint(), clientID, nil)
	if err == nil {
		if conn != nil {
			_ = conn.Close()
		}
		t.Fatal("expected allowlist rejection")
	}
	cancel()
}

func TestHandshakeRejectsDeviceIDMismatch(t *testing.T) {
	serverID := testIdentity(t)
	bad := testIdentity(t)
	bad.DeviceID = "dev_notmatching000000000000000000"

	ln, err := Listen(ListenConfig{
		Addr:     "127.0.0.1:0",
		Identity: serverID,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = ln.Serve(ctx) }()

	_, conn, err := Dial(context.Background(), ln.Endpoint(), bad, nil)
	if err == nil {
		_ = conn.Close()
		t.Fatal("expected device_id mismatch rejection")
	}
	cancel()
}
