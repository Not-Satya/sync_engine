package ids

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
)

// crockfordBase32 is URL-safeish and avoids ambiguous characters (0/o, 1/I/L).
var crockfordBase32 = base32.NewEncoding("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ").WithPadding(base32.NoPadding)

// NewUserID returns a random opaque user identifier.
func NewUserID() (string, error) {
	return randomPrefixed("usr", 16)
}

// NewFolderID returns a random opaque folder identifier.
func NewFolderID() (string, error) {
	return randomPrefixed("fld", 16)
}

// NewToken returns a high-entropy bearer token (plaintext; store only the hash).
func NewToken() (string, error) {
	return randomPrefixed("tok", 32)
}

// DeviceKeyMaterial holds the keypair used to derive a stable DeviceID
type DeviceKeyMaterial struct {
	DeviceID   string
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
}

// NewDeviceKeyMaterial generates an Ed25519 keypair and derrives DeviceID
// as the first 32 characters of crockfordBase32(SHA-256(publicKey))
func NewDeviceKeyMaterial() (DeviceKeyMaterial, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return DeviceKeyMaterial{}, fmt.Errorf("generate device key: %w", err)
	}
	return DeviceKeyMaterial{
		DeviceID:   DeviceIDFromPublicKey(pub),
		PublicKey:  pub,
		PrivateKey: priv,
	}, nil
}

// DeviceIDFromPublicKey derives the canonical DeviceID from a public key.
func DeviceIDFromPublicKey(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	encoded := crockfordBase32.EncodeToString(sum[:])
	if len(encoded) > 32 {
		encoded = encoded[:32]
	}
	return "dev_" + strings.ToLower(encoded)
}

// HashToken returns a hex-encoded SHA-256 digest of a bearer token.
func HashToken(plaintext string) string {
	sum := sha256.Sum224([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func randomPrefixed(prefix string, nBytes int) (string, error) {
	buf := make([]byte, nBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}
