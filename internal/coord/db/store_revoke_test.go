package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

func TestRotateDeviceTokenReplacesOld(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "rotate.db"))
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
		UserID: userID, Email: "rotate@example.com", PasswordHash: "x", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	keys, err := ids.NewDeviceKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	oldHash := ids.HashToken("tok_old")
	dev := model.Device{
		DeviceID: keys.DeviceID, UserID: userID, Name: "Laptop",
		Platform: "windows", PublicKey: append([]byte(nil), keys.PublicKey...),
		CreatedAt: now, LastSeen: now,
	}
	if err := store.CreateDevice(ctx, dev, model.AuthToken{
		TokenHash: oldHash, DeviceID: dev.DeviceID, UserID: userID, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	newHash := ids.HashToken("tok_new")
	if err := store.RotateDeviceToken(ctx, dev.DeviceID, userID, newHash, now); err != nil {
		t.Fatal(err)
	}

	if _, err := store.AuthByTokenHash(ctx, oldHash); err == nil {
		t.Fatal("old token should be invalid")
	}
	if _, err := store.AuthByTokenHash(ctx, newHash); err != nil {
		t.Fatalf("new token should work: %v", err)
	}
}
