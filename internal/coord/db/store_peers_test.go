package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

func TestListOnlineFolderPeers(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "peers.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	userID, _ := ids.NewUserID()
	_ = store.CreateUser(ctx, model.User{
		UserID: userID, Email: "p@example.com", PasswordHash: "x", CreatedAt: now,
	})

	mk := func(name string) model.Device {
		keys, _ := ids.NewDeviceKeyMaterial()
		d := model.Device{
			DeviceID: keys.DeviceID, UserID: userID, Name: name, Platform: "test",
			PublicKey: append([]byte(nil), keys.PublicKey...), CreatedAt: now, LastSeen: now,
		}
		_ = store.CreateDevice(ctx, d, model.AuthToken{
			TokenHash: ids.HashToken("tok-" + name), DeviceID: d.DeviceID, UserID: userID, CreatedAt: now,
		})
		return d
	}

	self := mk("Self")
	peer := mk("Peer")
	offline := mk("Offline")

	folderID, _ := ids.NewFolderID()
	_ = store.CreateFolder(ctx, model.Folder{
		FolderID: folderID, OwnerID: userID, Name: "Movies", CreatedAt: now,
	})
	for _, d := range []model.Device{self, peer, offline} {
		_ = store.Subscribe(ctx, model.Subscription{
			FolderID: folderID, DeviceID: d.DeviceID, SubscribedAt: now,
		})
	}

	_ = store.UpsertPresence(ctx, model.Presence{
		DeviceID: self.DeviceID, Status: model.PresenceOnline,
		Endpoint: "127.0.0.1:7901", UpdatedAt: now,
	})
	_ = store.UpsertPresence(ctx, model.Presence{
		DeviceID: peer.DeviceID, Status: model.PresenceOnline,
		Endpoint: "127.0.0.1:7900", UpdatedAt: now,
	})
	_ = store.UpsertPresence(ctx, model.Presence{
		DeviceID: offline.DeviceID, Status: model.PresenceOffline,
		Endpoint: "127.0.0.1:7902", UpdatedAt: now,
	})

	peers, err := store.ListOnlineFolderPeers(ctx, folderID, self.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].DeviceID != peer.DeviceID {
		t.Fatalf("want only online peer, got %+v", peers)
	}
	if peers[0].Endpoint != "127.0.0.1:7900" {
		t.Fatalf("endpoint=%q", peers[0].Endpoint)
	}
}
