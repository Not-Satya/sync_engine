package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

func TestRevokeDeviceWipesTokenAndBlocksReuse(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	userID, err := ids.NewUserID()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CreateUser(ctx, model.User{
		UserID: userID, Email: "a@example.com", PasswordHash: "x", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	keys, err := ids.NewDeviceKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	tokenHash := ids.HashToken("tok_test_plaintext")

	dev := model.Device{
		DeviceID: keys.DeviceID, UserID: userID, Name: "Laptop",
		Platform: "windows", PublicKey: append([]byte(nil), keys.PublicKey...),
		CreatedAt: now, LastSeen: now,
	}
	tok := model.AuthToken{
		TokenHash: tokenHash, DeviceID: dev.DeviceID, UserID: userID, CreatedAt: now,
	}
	if err := store.CreateDevice(ctx, dev, tok); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AuthByTokenHash(ctx, tokenHash); err != nil {
		t.Fatalf("token should work before revoke: %v", err)
	}

	if err := store.RevokeDevice(ctx, dev.DeviceID, now); err != nil {
		t.Fatal(err)
	}

	got, err := store.DeviceByID(ctx, dev.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Revoked() {
		t.Fatal("expected revoked")
	}

	if _, err := store.AuthByTokenHash(ctx, tokenHash); err == nil {
		t.Fatal("token should be gone after revoke")
	}

	pres, err := store.PresenceByDevice(ctx, dev.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if pres.Status != model.PresenceOffline {
		t.Fatalf("expected offline, got %s", pres.Status)
	}

	if err := store.RevokeDevice(ctx, dev.DeviceID, now); err != ErrRevoked {
		t.Fatalf("second revoke: want ErrRevoked, got %v", err)
	}
}
