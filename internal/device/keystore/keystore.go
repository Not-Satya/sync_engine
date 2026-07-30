package keystore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	FileVersion = 1
	FileName    = "keystore.json"
	AppDirName  = "sync_engine"
)

var (
	ErrNotFound           = errors.New("keystore not found")
	ErrCorrupt            = errors.New("keystore corrupt")
	ErrWrongPassphrase    = errors.New("wrong passphrase")
	ErrUnsupportedWrap    = errors.New("unsupported wrap method")
	ErrPassphraseRequired = errors.New("passphrase required")
)

// WrapMethod selects how the data-encryption key is protected at rest.
type WrapMethod string

const (
	WrapDPAPI      WrapMethod = "dpapi"
	WrapPassphrase WrapMethod = "passphrase"
)

// Secrets are the plaintext values that must never be written to disk.
type Secrets struct {
	PrivateKey []byte // Ed25519 private key
	Token      string // bearer token plaintext
}

// Record is the decrypted in-memory keystore.
type Record struct {
	UserID    string
	DeviceID  string
	CoordURL  string
	PublicKey []byte // raw Ed25519 public key
	Secrets   Secrets
}

// fileDoc is the on-disk JSON shape (ciphertext + metadata only).
type fileDoc struct {
	Version      int        `json:"version"`
	UserID       string     `json:"user_id"`
	DeviceID     string     `json:"device_id"`
	CoordURL     string     `json:"coord_url"`
	PublicKeyHex string     `json:"public_key_hex"`
	Wrap         wrapMeta   `json:"wrap"`
	PrivateKey   sealedBlob `json:"private_key"`
	Token        sealedBlob `json:"token"`
}

type wrapMeta struct {
	Method     WrapMethod `json:"method"`
	SaltB64    string     `json:"salt,omitempty"`
	WrappedKey string     `json:"wrapped_key"`     // base64 ciphertext of 32-byte data key
	NonceB64   string     `json:"nonce,omitempty"` // for passphrase-wrapped key
}

type sealedBlob struct {
	NonceB64      string `json:"nonce"`
	CiphertextB64 string `json:"ciphertext"`
}

// Options control how Save protects secrets.
type Options struct {
	// Method defaults to DPAPI on Windows, passphrase elsewhere.
	Method WrapMethod
	// Passphrase is required when Method is WrapPassphrase.
	Passphrase string
}

// DefaultPath returns %AppData%/sync_engine/keystore.json (or OS equivalent).
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, AppDirName, FileName), nil
}

// Save encrypts secrets and writes the keystore atomically.
func Save(path string, rec Record, opts Options) error {
	if rec.UserID == "" || rec.DeviceID == "" || rec.CoordURL == "" {
		return fmt.Errorf("user_id, device_id, and coord_url required")
	}
	if len(rec.PublicKey) == 0 || len(rec.Secrets.PrivateKey) == 0 || rec.Secrets.Token == "" {
		return fmt.Errorf("public_key, private_key, and token required")
	}

	method := opts.Method
	if method == "" {
		method = defaultWrapMethod()
	}

	dataKey, err := newDataKey()
	if err != nil {
		return err
	}

	privSeal, err := seal(dataKey, rec.Secrets.PrivateKey)
	if err != nil {
		return err
	}
	tokSeal, err := seal(dataKey, []byte(rec.Secrets.Token))
	if err != nil {
		return err
	}

	wrap, err := wrapDataKey(method, dataKey, opts.Passphrase)
	if err != nil {
		return err
	}

	doc := fileDoc{
		Version:      FileVersion,
		UserID:       rec.UserID,
		DeviceID:     rec.DeviceID,
		CoordURL:     rec.CoordURL,
		PublicKeyHex: encodeHex(rec.PublicKey),
		Wrap:         wrap,
		PrivateKey:   privSeal,
		Token:        tokSeal,
	}

	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load decrypts and returns the keystore. Passphrase is required for passphrase wrap.
func Load(path string, passphrase string) (Record, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, ErrNotFound
		}
		return Record{}, err
	}
	var doc fileDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Record{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if doc.Version != FileVersion {
		return Record{}, fmt.Errorf("%w: unsupported version %d", ErrCorrupt, doc.Version)
	}

	dataKey, err := unwrapDataKey(doc.Wrap, passphrase)
	if err != nil {
		return Record{}, err
	}

	priv, err := unseal(dataKey, doc.PrivateKey)
	if err != nil {
		return Record{}, fmt.Errorf("%w: private key", ErrCorrupt)
	}
	tok, err := unseal(dataKey, doc.Token)
	if err != nil {
		return Record{}, fmt.Errorf("%w: token", ErrCorrupt)
	}
	pub, err := decodeHex(doc.PublicKeyHex)
	if err != nil {
		return Record{}, fmt.Errorf("%w: public key", ErrCorrupt)
	}

	return Record{
		UserID:    doc.UserID,
		DeviceID:  doc.DeviceID,
		CoordURL:  doc.CoordURL,
		PublicKey: pub,
		Secrets: Secrets{
			PrivateKey: priv,
			Token:      string(tok),
		},
	}, nil
}

// Exists reports whether a keystore file is present at path.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Remove deletes the keystore file if present.
func Remove(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
