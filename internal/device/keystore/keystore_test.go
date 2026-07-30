package keystore

import (
	"bytes"
	"crypto/ed25519"
	"path/filepath"
	"testing"
)

func TestPassphraseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keystore.json")

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := Record{
		UserID:    "usr_test",
		DeviceID:  "dev_test",
		CoordURL:  "http://localhost:8080",
		PublicKey: pub,
		Secrets: Secrets{
			PrivateKey: priv,
			Token:      "tok_secret_value",
		},
	}
	if err := Save(path, rec, Options{Method: WrapPassphrase, Passphrase: "correct horse"}); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != rec.UserID || got.DeviceID != rec.DeviceID || got.CoordURL != rec.CoordURL {
		t.Fatalf("metadata mismatch: %+v", got)
	}
	if !bytes.Equal(got.PublicKey, pub) {
		t.Fatal("public key mismatch")
	}
	if !bytes.Equal(got.Secrets.PrivateKey, priv) {
		t.Fatal("private key mismatch")
	}
	if got.Secrets.Token != "tok_secret_value" {
		t.Fatalf("token mismatch: %q", got.Secrets.Token)
	}

	if _, err := Load(path, "wrong"); err != ErrWrongPassphrase {
		t.Fatalf("want ErrWrongPassphrase, got %v", err)
	}
}

func TestSaveRequiresPassphraseWhenChosen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keystore.json")
	pub, priv, _ := ed25519.GenerateKey(nil)
	err := Save(path, Record{
		UserID: "u", DeviceID: "d", CoordURL: "http://x", PublicKey: pub,
		Secrets: Secrets{PrivateKey: priv, Token: "t"},
	}, Options{Method: WrapPassphrase})
	if err != ErrPassphraseRequired {
		t.Fatalf("want ErrPassphraseRequired, got %v", err)
	}
}
