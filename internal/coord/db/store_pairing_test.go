package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

func TestPairingCodeRedeemAndSingleUse(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "pair.db"))
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
		UserID: userID, Email: "pair@example.com", PasswordHash: "x", CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	creatorKeys, err := ids.NewDeviceKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	creator := model.Device{
		DeviceID: creatorKeys.DeviceID, UserID: userID, Name: "Laptop",
		Platform: "windows", PublicKey: append([]byte(nil), creatorKeys.PublicKey...),
		CreatedAt: now, LastSeen: now,
	}
	if err := store.CreateDevice(ctx, creator, model.AuthToken{
		TokenHash: ids.HashToken("tok_creator"), DeviceID: creator.DeviceID, UserID: userID, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	plain, err := ids.NewPairingCode()
	if err != nil {
		t.Fatal(err)
	}
	hash := ids.HashToken(ids.NormalizePairingCode(plain))
	if err := store.CreatePairingCode(ctx, model.PairingCode{
		CodeHash: hash, UserID: userID, CreatedBy: creator.DeviceID,
		CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	newKeys, err := ids.NewDeviceKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	newDev := model.Device{
		DeviceID: newKeys.DeviceID, Name: "Phone", Platform: "android",
		PublicKey: append([]byte(nil), newKeys.PublicKey...),
		CreatedAt: now, LastSeen: now,
	}
	gotUser, err := store.RedeemPairingCode(ctx, hash, newDev, model.AuthToken{
		TokenHash: ids.HashToken("tok_phone"), DeviceID: newDev.DeviceID, CreatedAt: now,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if gotUser != userID {
		t.Fatalf("user: got %s want %s", gotUser, userID)
	}

	_, err = store.RedeemPairingCode(ctx, hash, newDev, model.AuthToken{
		TokenHash: ids.HashToken("tok_again"), DeviceID: "dev_x", CreatedAt: now,
	}, now)
	if err != ErrConflict {
		t.Fatalf("second redeem: want ErrConflict, got %v", err)
	}
}

func TestPairingCodeExpired(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "pair_exp.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	userID, _ := ids.NewUserID()
	_ = store.CreateUser(ctx, model.User{
		UserID: userID, Email: "exp@example.com", PasswordHash: "x", CreatedAt: now,
	})
	keys, _ := ids.NewDeviceKeyMaterial()
	creator := model.Device{
		DeviceID: keys.DeviceID, UserID: userID, Name: "Laptop",
		PublicKey: append([]byte(nil), keys.PublicKey...), CreatedAt: now, LastSeen: now,
	}
	_ = store.CreateDevice(ctx, creator, model.AuthToken{
		TokenHash: ids.HashToken("tok"), DeviceID: creator.DeviceID, UserID: userID, CreatedAt: now,
	})

	hash := ids.HashToken("ABCDEFGH")
	_ = store.CreatePairingCode(ctx, model.PairingCode{
		CodeHash: hash, UserID: userID, CreatedBy: creator.DeviceID,
		CreatedAt: now.Add(-20 * time.Minute), ExpiresAt: now.Add(-10 * time.Minute),
	})

	newKeys, _ := ids.NewDeviceKeyMaterial()
	_, err = store.RedeemPairingCode(ctx, hash, model.Device{
		DeviceID: newKeys.DeviceID, Name: "Phone",
		PublicKey: append([]byte(nil), newKeys.PublicKey...), CreatedAt: now, LastSeen: now,
	}, model.AuthToken{TokenHash: ids.HashToken("n"), DeviceID: newKeys.DeviceID, CreatedAt: now}, now)
	if err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}
