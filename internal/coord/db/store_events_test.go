package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

func TestAppendAndListFolderEvents(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	userID, _ := ids.NewUserID()
	_ = store.CreateUser(ctx, model.User{
		UserID: userID, Email: "ev@example.com", PasswordHash: "x", CreatedAt: now,
	})
	keys, _ := ids.NewDeviceKeyMaterial()
	dev := model.Device{
		DeviceID: keys.DeviceID, UserID: userID, Name: "Laptop",
		PublicKey: append([]byte(nil), keys.PublicKey...), CreatedAt: now, LastSeen: now,
	}
	_ = store.CreateDevice(ctx, dev, model.AuthToken{
		TokenHash: ids.HashToken("tok"), DeviceID: dev.DeviceID, UserID: userID, CreatedAt: now,
	})
	folderID, _ := ids.NewFolderID()
	_ = store.CreateFolder(ctx, model.Folder{
		FolderID: folderID, OwnerID: userID, Name: "Movies", CreatedAt: now,
	})
	_ = store.Subscribe(ctx, model.Subscription{
		FolderID: folderID, DeviceID: dev.DeviceID, SubscribedAt: now,
	})

	ok, err := store.IsDeviceSubscribed(ctx, folderID, dev.DeviceID)
	if err != nil || !ok {
		t.Fatalf("subscribed: %v %v", ok, err)
	}

	batch := []model.FolderEvent{{
		EventID: "evt_1", FolderID: folderID, DeviceID: dev.DeviceID,
		Op: model.MetaOpUpsert, Path: "a.txt", Size: 3, ContentHash: "abc",
		HLC: model.HLC{Wall: now.UnixNano(), Counter: 0},
	}}
	accepted, err := store.AppendFolderEvents(ctx, folderID, batch)
	if err != nil || len(accepted) != 1 || accepted[0].Seq < 1 {
		t.Fatalf("append: %+v %v", accepted, err)
	}

	// Idempotent duplicate
	again, err := store.AppendFolderEvents(ctx, folderID, batch)
	if err != nil || len(again) != 0 {
		t.Fatalf("dup: %+v %v", again, err)
	}

	listed, err := store.ListFolderEventsSince(ctx, folderID, 0, 10)
	if err != nil || len(listed) != 1 || listed[0].Path != "a.txt" {
		t.Fatalf("list: %+v %v", listed, err)
	}
	after, err := store.ListFolderEventsSince(ctx, folderID, listed[0].Seq, 10)
	if err != nil || len(after) != 0 {
		t.Fatalf("since: %+v %v", after, err)
	}
}
