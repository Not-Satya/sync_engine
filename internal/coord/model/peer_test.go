package model

import "testing"

func TestNormalizeTransferEndpoint(t *testing.T) {
	ok, err := NormalizeTransferEndpoint(" 127.0.0.1:7900 ")
	if err != nil || ok != "127.0.0.1:7900" {
		t.Fatalf("got %q %v", ok, err)
	}
	if _, err := NormalizeTransferEndpoint(""); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeTransferEndpoint("http://127.0.0.1:7900"); err == nil {
		t.Fatal("expected URL reject")
	}
	if _, err := NormalizeTransferEndpoint("127.0.0.1"); err == nil {
		t.Fatal("expected missing port reject")
	}
	if _, err := NormalizeTransferEndpoint("[::1]:7900"); err != nil {
		t.Fatalf("ipv6: %v", err)
	}
}
