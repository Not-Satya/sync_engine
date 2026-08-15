package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

type fakePeerDiscoverer struct {
	peers []client.FolderPeer
	err   error
}

func (f fakePeerDiscoverer) ListFolderPeers(_ context.Context, _ string) ([]client.FolderPeer, error) {
	return f.peers, f.err
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func openPlannerIndex(t *testing.T) *index.Store {
	t.Helper()
	store, err := index.Open(filepath.Join(t.TempDir(), index.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestPlannerMissingSkipsMatchingLocalFile(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := openPlannerIndex(t)

	good := []byte("already present")
	missing := []byte("needs fetch")
	if err := os.WriteFile(filepath.Join(root, "good.txt"), good, 0o600); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string][]byte{"good.txt": good, "sub/missing.txt": missing} {
		if err := store.Upsert(ctx, index.Entry{
			FolderID: "fld_1", Path: path, Size: int64(len(data)), ContentHash: hashOf(data),
			HLCWall: 1, DeviceID: "dev_peer",
		}); err != nil {
			t.Fatal(err)
		}
	}

	planner := Planner{Index: store}
	candidates, err := planner.Missing(ctx, "fld_1", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Entry.Path != "sub/missing.txt" {
		t.Fatalf("candidates: %+v", candidates)
	}
	if candidates[0].Path != filepath.Join(root, "sub", "missing.txt") {
		t.Fatalf("destination: %s", candidates[0].Path)
	}
}

func TestPlannerFetchFallsBackToNextPeer(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := openPlannerIndex(t)
	data := []byte("fetch from second peer")
	hash := hashOf(data)
	if err := store.Upsert(ctx, index.Entry{
		FolderID: "fld_1", Path: "movie.bin", Size: int64(len(data)), ContentHash: hash,
		HLCWall: 1, DeviceID: "dev_peer",
	}); err != nil {
		t.Fatal(err)
	}

	localID := testIdentity(t)
	peer1 := testIdentity(t)
	peer2 := testIdentity(t)
	discoverer := fakePeerDiscoverer{peers: []client.FolderPeer{
		{
			DeviceID: peer1.DeviceID, Endpoint: "127.0.0.1:1", Status: "online",
			PublicKeyHex: hex.EncodeToString(peer1.PublicKey),
		},
		{
			DeviceID: peer2.DeviceID, Endpoint: "127.0.0.1:2", Status: "online",
			PublicKeyHex: hex.EncodeToString(peer2.PublicKey),
		},
	}}

	pulls := 0
	planner := Planner{
		Peers: discoverer,
		Index: store,
		ID:    localID,
		Pull: func(_ context.Context, addr string, _ Identity, allow PeerAllowFunc, contentHash, dest string) error {
			pulls++
			if addr == "127.0.0.1:1" {
				return ErrBlobNotFound
			}
			if contentHash != hash {
				t.Fatalf("hash=%s want=%s", contentHash, hash)
			}
			if !allow(peer2.DeviceID, peer2.PublicKey) || allow(peer1.DeviceID, peer1.PublicKey) {
				t.Fatal("allowlist did not bind the selected peer key")
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
				return err
			}
			return os.WriteFile(dest, data, 0o600)
		},
	}

	results, err := planner.FetchFolder(ctx, "fld_1", root)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Fetched || results[0].Attempts != 2 {
		t.Fatalf("results: %+v", results)
	}
	if results[0].Peer.DeviceID != peer2.DeviceID || pulls != 2 {
		t.Fatalf("peer=%s pulls=%d", results[0].Peer.DeviceID, pulls)
	}
	got, err := os.ReadFile(filepath.Join(root, "movie.bin"))
	if err != nil || string(got) != string(data) {
		t.Fatalf("file=%q err=%v", got, err)
	}
}

func TestPlannerReportsNoDialablePeers(t *testing.T) {
	ctx := context.Background()
	store := openPlannerIndex(t)
	data := []byte("missing")
	if err := store.Upsert(ctx, index.Entry{
		FolderID: "fld_1", Path: "a.txt", Size: int64(len(data)), ContentHash: hashOf(data),
		HLCWall: 1, DeviceID: "dev_peer",
	}); err != nil {
		t.Fatal(err)
	}
	planner := Planner{
		Peers: fakePeerDiscoverer{peers: []client.FolderPeer{{Status: "online"}}},
		Index: store,
		ID:    testIdentity(t),
	}
	results, err := planner.FetchFolder(ctx, "fld_1", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Fetched || results[0].Err == nil {
		t.Fatalf("results: %+v", results)
	}
	if results[0].Attempts != 0 {
		t.Fatalf("unexpected result: %+v", results[0])
	}
}
