package ids

import (
	"crypto/ed25519"
	"strings"
	"testing"
)

func TestDeviceIDFromPublicKeyStable(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	a := DeviceIDFromPublicKey(pub)
	b := DeviceIDFromPublicKey(pub)
	if a != b {
		t.Fatalf("unstable id: %s vs %s", a, b)
	}
	if !strings.HasPrefix(a, "dev_") {
		t.Fatalf("missing prefix: %s", a)
	}
	if len(a) != len("dev_")+32 {
		t.Fatalf("unexpected length: %d (%s)", len(a), a)
	}
}

func TestNewDeviceKeyMaterial(t *testing.T) {
	m, err := NewDeviceKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	if got := DeviceIDFromPublicKey(m.PublicKey); got != m.DeviceID {
		t.Fatalf("mismatch: material=%s derived=%s", m.DeviceID, got)
	}
}
