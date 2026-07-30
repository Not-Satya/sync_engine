//go:build windows

package keystore

import (
	"bytes"
	"crypto/ed25519"
	"path/filepath"
	"testing"
)

func TestDPAPIRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keystore.json")

	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := Record{
		UserID: "usr_dpapi", DeviceID: "dev_dpapi", CoordURL: "http://localhost:8080",
		PublicKey: pub,
		Secrets:   Secrets{PrivateKey: priv, Token: "tok_dpapi"},
	}
	if err := Save(path, rec, Options{Method: WrapDPAPI}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Secrets.PrivateKey, priv) || got.Secrets.Token != "tok_dpapi" {
		t.Fatal("round-trip mismatch")
	}
}
