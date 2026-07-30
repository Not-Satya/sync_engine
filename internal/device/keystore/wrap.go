package keystore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

func wrapDataKey(method WrapMethod, dataKey []byte, passphrase string) (wrapMeta, error) {
	switch method {
	case WrapDPAPI:
		wrapped, err := dpapiProtect(dataKey)
		if err != nil {
			return wrapMeta{}, err
		}
		return wrapMeta{
			Method:     WrapDPAPI,
			WrappedKey: base64.StdEncoding.EncodeToString(wrapped),
		}, nil
	case WrapPassphrase:
		if passphrase == "" {
			return wrapMeta{}, ErrPassphraseRequired
		}
		salt := make([]byte, saltLen)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return wrapMeta{}, err
		}
		kek := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
		nonce, ct, err := aesGCMSeal(kek, dataKey)
		if err != nil {
			return wrapMeta{}, err
		}
		return wrapMeta{
			Method:     WrapPassphrase,
			SaltB64:    base64.StdEncoding.EncodeToString(salt),
			NonceB64:   base64.StdEncoding.EncodeToString(nonce),
			WrappedKey: base64.StdEncoding.EncodeToString(ct),
		}, nil
	default:
		return wrapMeta{}, fmt.Errorf("%w: %s", ErrUnsupportedWrap, method)
	}
}

func unwrapDataKey(meta wrapMeta, passphrase string) ([]byte, error) {
	wrapped, err := base64.StdEncoding.DecodeString(meta.WrappedKey)
	if err != nil {
		return nil, fmt.Errorf("%w: wrapped key", ErrCorrupt)
	}
	switch meta.Method {
	case WrapDPAPI:
		return dpapiUnprotect(wrapped)
	case WrapPassphrase:
		if passphrase == "" {
			return nil, ErrPassphraseRequired
		}
		salt, err := base64.StdEncoding.DecodeString(meta.SaltB64)
		if err != nil {
			return nil, fmt.Errorf("%w: salt", ErrCorrupt)
		}
		nonce, err := base64.StdEncoding.DecodeString(meta.NonceB64)
		if err != nil {
			return nil, fmt.Errorf("%w: wrap nonce", ErrCorrupt)
		}
		kek := argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
		plain, err := aesGCMOpen(kek, nonce, wrapped)
		if err != nil {
			return nil, ErrWrongPassphrase
		}
		return plain, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedWrap, meta.Method)
	}
}

func aesGCMSeal(key, plaintext []byte) (nonce, ciphertext []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	return nonce, gcm.Seal(nil, nonce, plaintext, nil), nil
}

func aesGCMOpen(key, nonce, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
