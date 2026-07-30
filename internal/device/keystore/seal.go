package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

func newDataKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("data key: %w", err)
	}
	return key, nil
}

func seal(key, plaintext []byte) (sealedBlob, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return sealedBlob{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return sealedBlob{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return sealedBlob{}, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return sealedBlob{
		NonceB64:      base64.StdEncoding.EncodeToString(nonce),
		CiphertextB64: base64.StdEncoding.EncodeToString(ct),
	}, nil
}

func unseal(key []byte, blob sealedBlob) ([]byte, error) {
	nonce, err := base64.StdEncoding.DecodeString(blob.NonceB64)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(blob.CiphertextB64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ct, nil)
}

func encodeHex(b []byte) string { return hex.EncodeToString(b) }

func decodeHex(s string) ([]byte, error) { return hex.DecodeString(s) }
