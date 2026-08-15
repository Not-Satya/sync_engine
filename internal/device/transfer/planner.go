package transfer

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Not-Satya/sync_engine/internal/device/client"
	"github.com/Not-Satya/sync_engine/internal/device/index"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

// PeerDiscoverer lists online peers for a folder. *client.Client implements it.
type PeerDiscoverer interface {
	ListFolderPeers(ctx context.Context, folderID string) ([]client.FolderPeer, error)
}

// PullFunc performs one verified pull. It is injectable for testing.
type PullFunc func(
	ctx context.Context,
	addr string,
	id Identity,
	allow PeerAllowFunc,
	contentHash string,
	destPath string,
) error

// Planner compares expected index entries to local disk, then obtains missing
// bytes directly from an online folder peer. The coordinator is used only for
// peer introduction (ADR 21 / 26).
type Planner struct {
	Peers PeerDiscoverer
	Index *index.Store
	ID    Identity
	Pull  PullFunc
}

// Candidate is one live metadata entry whose expected bytes are absent or
// differ from its content hash on disk.
type Candidate struct {
	Entry index.Entry
	Path  string // absolute destination under the local folder root
}

// Missing lists entries requiring a verified pull. A same-sized local file is
// still SHA-256 checked: size alone is not content identity (ADR 17).
func (p Planner) Missing(ctx context.Context, folderID, root string) ([]Candidate, error) {
	if p.Index == nil {
		return nil, fmt.Errorf("transfer planner: nil index")
	}
	entries, err := p.Index.List(ctx, folderID)
	if err != nil {
		return nil, err
	}
	out := make([]Candidate, 0)
	for _, entry := range entries {
		if entry.ContentHash == "" {
			continue
		}
		dest := filepath.Join(root, filepath.FromSlash(entry.Path))
		ok, err := matchesEntry(dest, entry)
		if err != nil {
			return nil, err
		}
		if !ok {
			out = append(out, Candidate{Entry: entry, Path: dest})
		}
	}
	return out, nil
}

// FetchFolder attempts each missing file against each dialable folder peer.
// A peer may not have a particular hash (ADR 26), so not-found and transient
// pull failures fall through to the next peer. It returns one result per
// candidate and never deletes the metadata entry when no peer can provide it.
func (p Planner) FetchFolder(ctx context.Context, folderID, root string) ([]FetchResult, error) {
	if p.Peers == nil {
		return nil, fmt.Errorf("transfer planner: nil peer discoverer")
	}
	if err := p.ID.Validate(); err != nil {
		return nil, err
	}
	pull := p.Pull
	if pull == nil {
		pull = PullFrom
	}

	candidates, err := p.Missing(ctx, folderID, root)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return []FetchResult{}, nil
	}
	peers, err := p.Peers.ListFolderPeers(ctx, folderID)
	if err != nil {
		return nil, err
	}
	peers = client.DialablePeers(peers)

	results := make([]FetchResult, 0, len(candidates))
	for _, candidate := range candidates {
		result := FetchResult{Candidate: candidate}
		for _, peer := range peers {
			allow, err := allowOnlyPeer(peer)
			if err != nil {
				continue // malformed coordinator response is not dialable
			}
			result.Attempts++
			err = pull(ctx, peer.Endpoint, p.ID, allow, candidate.Entry.ContentHash, candidate.Path)
			if err == nil {
				result.Peer = peer
				result.Fetched = true
				break
			}
			result.Err = err
		}
		if !result.Fetched && result.Err == nil {
			result.Err = fmt.Errorf("no dialable peers for folder %s", folderID)
		}
		results = append(results, result)
	}
	return results, nil
}

// FetchResult reports a planner attempt without hiding failures that should be
// retried once a peer comes online.
type FetchResult struct {
	Candidate Candidate
	Peer      client.FolderPeer
	Attempts  int
	Fetched   bool
	Err       error
}

func matchesEntry(path string, entry index.Entry) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.IsDir() || info.Size() != entry.Size {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == entry.ContentHash, nil
}

func allowOnlyPeer(peer client.FolderPeer) (PeerAllowFunc, error) {
	pub, err := hex.DecodeString(peer.PublicKeyHex)
	if err != nil {
		return nil, fmt.Errorf("decode peer public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid peer public key length")
	}
	key := ed25519.PublicKey(pub)
	if ids.DeviceIDFromPublicKey(key) != peer.DeviceID {
		return nil, fmt.Errorf("peer device_id does not match advertised public key")
	}
	return func(deviceID string, got ed25519.PublicKey) bool {
		return deviceID == peer.DeviceID && string(got) == string(key)
	}, nil
}
