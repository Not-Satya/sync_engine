package transfer

import (
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/curve25519"

	"github.com/Not-Satya/sync_engine/internal/ids"
)

const (
	maxFrameBytes   = 64 << 10
	handshakeTimeout = 15 * time.Second
)

type helloFrame struct {
	Type     string `json:"type"` // "hello"
	DeviceID string `json:"device_id"`
	PubKey   []byte `json:"pub_key"`
	Nonce    []byte `json:"nonce"`
	EphPub   []byte `json:"eph_pub"` // X25519 ephemeral public key
}

type authFrame struct {
	Type string `json:"type"` // "auth"
	Sig  []byte `json:"sig"`
}

// Handshake runs mutual Ed25519 authentication and ECDH session-key derivation
// on an already-connected TCP socket. dialer is true if we initiated the dial.
func Handshake(conn net.Conn, id Identity, dialer bool, allow PeerAllowFunc) (*Session, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	var ephPriv, ephPub [32]byte
	if _, err := rand.Read(ephPriv[:]); err != nil {
		return nil, err
	}
	curve25519.ScalarBaseMult(&ephPub, &ephPriv)

	localHello := helloFrame{
		Type:     "hello",
		DeviceID: id.DeviceID,
		PubKey:   append([]byte(nil), id.PublicKey...),
		Nonce:    nonce,
		EphPub:   ephPub[:],
	}

	var remoteHello helloFrame
	if dialer {
		if err := writeJSONFrame(conn, localHello); err != nil {
			return nil, fmt.Errorf("send hello: %w", err)
		}
		if err := readJSONFrame(conn, &remoteHello); err != nil {
			return nil, fmt.Errorf("read hello: %w", err)
		}
		if remoteHello.Type != "hello" {
			return nil, fmt.Errorf("expected hello frame, got %q", remoteHello.Type)
		}
		if err := validateHello(remoteHello); err != nil {
			return nil, err
		}
		peerPub := ed25519.PublicKey(remoteHello.PubKey)
		if allow != nil && !allow(remoteHello.DeviceID, peerPub) {
			return nil, fmt.Errorf("peer %s not allowed", remoteHello.DeviceID)
		}
	} else {
		if err := readJSONFrame(conn, &remoteHello); err != nil {
			return nil, fmt.Errorf("read hello: %w", err)
		}
		if remoteHello.Type != "hello" {
			return nil, fmt.Errorf("expected hello frame, got %q", remoteHello.Type)
		}
		if err := validateHello(remoteHello); err != nil {
			return nil, err
		}
		peerPub := ed25519.PublicKey(remoteHello.PubKey)
		if allow != nil && !allow(remoteHello.DeviceID, peerPub) {
			return nil, fmt.Errorf("peer %s not allowed", remoteHello.DeviceID)
		}
		if err := writeJSONFrame(conn, localHello); err != nil {
			return nil, fmt.Errorf("send hello: %w", err)
		}
	}

	// Transcript binds both hellos in dialer-first order.
	var transcript []byte
	if dialer {
		transcript = helloTranscript(localHello, remoteHello)
	} else {
		transcript = helloTranscript(remoteHello, localHello)
	}

	sig := ed25519.Sign(id.PrivateKey, transcript)
	localAuth := authFrame{Type: "auth", Sig: sig}
	var remoteAuth authFrame
	if dialer {
		if err := writeJSONFrame(conn, localAuth); err != nil {
			return nil, fmt.Errorf("send auth: %w", err)
		}
		if err := readJSONFrame(conn, &remoteAuth); err != nil {
			return nil, fmt.Errorf("read auth: %w", err)
		}
	} else {
		if err := readJSONFrame(conn, &remoteAuth); err != nil {
			return nil, fmt.Errorf("read auth: %w", err)
		}
		if err := writeJSONFrame(conn, localAuth); err != nil {
			return nil, fmt.Errorf("send auth: %w", err)
		}
	}
	if remoteAuth.Type != "auth" {
		return nil, fmt.Errorf("expected auth frame, got %q", remoteAuth.Type)
	}
	peerPub := ed25519.PublicKey(remoteHello.PubKey)
	if !ed25519.Verify(peerPub, transcript, remoteAuth.Sig) {
		return nil, fmt.Errorf("peer signature invalid")
	}

	var peerEph [32]byte
	copy(peerEph[:], remoteHello.EphPub)
	shared, err := curve25519.X25519(ephPriv[:], peerEph[:])
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}
	sessionKey, err := hkdf.Key(sha256.New, shared, transcript, "sync-xfer-session", 32)
	if err != nil {
		return nil, fmt.Errorf("hkdf: %w", err)
	}

	return &Session{
		PeerDeviceID:  remoteHello.DeviceID,
		PeerPublicKey: peerPub,
		SessionKey:    sessionKey,
		Dialer:        dialer,
	}, nil
}

func validateHello(h helloFrame) error {
	if len(h.PubKey) != ed25519.PublicKeySize {
		return fmt.Errorf("peer public key length")
	}
	if len(h.Nonce) != 32 {
		return fmt.Errorf("peer nonce length")
	}
	if len(h.EphPub) != 32 {
		return fmt.Errorf("peer eph_pub length")
	}
	if h.DeviceID == "" {
		return fmt.Errorf("peer device_id empty")
	}
	want := ids.DeviceIDFromPublicKey(ed25519.PublicKey(h.PubKey))
	if h.DeviceID != want {
		return fmt.Errorf("peer device_id does not match public key")
	}
	return nil
}

func helloTranscript(dialerHello, listenerHello helloFrame) []byte {
	a, _ := json.Marshal(dialerHello)
	b, _ := json.Marshal(listenerHello)
	out := make([]byte, 0, len(ProtocolVersion)+len(a)+len(b)+2)
	out = append(out, ProtocolVersion...)
	out = append(out, 0)
	out = append(out, a...)
	out = append(out, 0)
	out = append(out, b...)
	return out
}

func writeJSONFrame(w io.Writer, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(raw) > maxFrameBytes {
		return fmt.Errorf("frame too large")
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(raw)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(raw)
	return err
}

func readJSONFrame(r io.Reader, v any) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 || n > maxFrameBytes {
		return fmt.Errorf("invalid frame length %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}
