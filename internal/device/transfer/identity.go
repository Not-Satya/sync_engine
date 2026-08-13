// Package transfer implements device-to-device byte transfer (Phase 5).
// The coordination server never sees these bytes — only introduces peers.
package transfer

import (
	"crypto/ed25519"
	"fmt"
)

// ProtocolVersion is stamped into the handshake transcript.
const ProtocolVersion = "sync-xfer-v1"

// Identity is this device's long-term Ed25519 key material (from the keystore).
type Identity struct {
	DeviceID   string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// Validate checks key lengths and that DeviceID matches the public key.
func (id Identity) Validate() error {
	if len(id.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("transfer: invalid public key length")
	}
	if len(id.PrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("transfer: invalid private key length")
	}
	if id.DeviceID == "" {
		return fmt.Errorf("transfer: device_id required")
	}
	return nil
}

// Session is the result of a successful mutual handshake (ADR 22).
// SessionKey is for AES-256-GCM frames in P5.3; Conn remains open for protocol.
type Session struct {
	PeerDeviceID string
	PeerPublicKey ed25519.PublicKey
	SessionKey   []byte // 32 bytes
	Dialer       bool   // true if we initiated the TCP connection
}

// PeerAllowFunc decides whether an authenticated peer may continue.
// nil means any peer whose DeviceID matches their public key is accepted
// (caller should pass a folder-peer allowlist in production).
type PeerAllowFunc func(deviceID string, pub ed25519.PublicKey) bool
