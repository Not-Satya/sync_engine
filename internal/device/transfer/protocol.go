package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Pull message types (JSON control frames after handshake).
const (
	MsgPullReq  = "pull_req"
	MsgPullOK   = "pull_ok"
	MsgPullErr  = "pull_err"
	MsgPullDone = "pull_done"
)

// ErrBlobNotFound means this device does not have the requested content hash.
var ErrBlobNotFound = errors.New("transfer: blob not found")

// BlobStore serves whole-file bytes by content hash (ADR 20).
type BlobStore interface {
	Open(ctx context.Context, contentHash string) (size int64, r io.ReadCloser, err error)
}

// MapBlobStore is an in-memory BlobStore for tests.
type MapBlobStore map[string][]byte

func (m MapBlobStore) Open(_ context.Context, contentHash string) (int64, io.ReadCloser, error) {
	b, ok := m[contentHash]
	if !ok {
		return 0, nil, ErrBlobNotFound
	}
	return int64(len(b)), io.NopCloser(newBytesReader(b)), nil
}

type bytesReader struct {
	b []byte
	i int
}

func newBytesReader(b []byte) *bytesReader { return &bytesReader{b: b} }

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

// PathBlobStore maps content hashes to absolute file paths.
type PathBlobStore map[string]string

func (p PathBlobStore) Open(_ context.Context, contentHash string) (int64, io.ReadCloser, error) {
	path, ok := p[contentHash]
	if !ok {
		return 0, nil, ErrBlobNotFound
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil, ErrBlobNotFound
		}
		return 0, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return 0, nil, err
	}
	return st.Size(), f, nil
}

type pullReq struct {
	Type        string `json:"type"`
	ContentHash string `json:"content_hash"`
}

type pullOK struct {
	Type        string `json:"type"`
	ContentHash string `json:"content_hash"`
	Size        int64  `json:"size"`
}

type pullErr struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type pullDone struct {
	Type string `json:"type"`
}

// ServePull handles one pull_req on an already-handshaken secure connection.
func ServePull(ctx context.Context, sc *SecureConn, store BlobStore) error {
	if store == nil {
		return fmt.Errorf("transfer: nil blob store")
	}
	var req pullReq
	if err := sc.ReadJSON(&req); err != nil {
		return err
	}
	if req.Type != MsgPullReq || req.ContentHash == "" {
		return fmt.Errorf("invalid pull_req")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	size, r, err := store.Open(ctx, req.ContentHash)
	if err != nil {
		code := "error"
		if errors.Is(err, ErrBlobNotFound) {
			code = "not_found"
		}
		return sc.WriteJSON(pullErr{Type: MsgPullErr, Code: code, Message: err.Error()})
	}
	defer r.Close()

	if err := sc.WriteJSON(pullOK{
		Type: MsgPullOK, ContentHash: req.ContentHash, Size: size,
	}); err != nil {
		return err
	}

	buf := make([]byte, maxPlain)
	var sent int64
	for sent < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := r.Read(buf)
		if n > 0 {
			if werr := sc.WriteChunk(buf[:n]); werr != nil {
				return werr
			}
			sent += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if sent != size {
		return fmt.Errorf("short read: sent %d want %d", sent, size)
	}
	return sc.WriteJSON(pullDone{Type: MsgPullDone})
}

// PullFile requests contentHash from the peer and writes it to destPath
// via temp + SHA-256 verify + rename (ADR 20 / 24).
func PullFile(ctx context.Context, sc *SecureConn, contentHash, destPath string) error {
	if contentHash == "" {
		return fmt.Errorf("content_hash required")
	}
	if destPath == "" {
		return fmt.Errorf("dest path required")
	}
	if err := sc.WriteJSON(pullReq{Type: MsgPullReq, ContentHash: contentHash}); err != nil {
		return err
	}

	typ, payload, err := sc.readControl()
	if err != nil {
		return err
	}
	switch typ {
	case MsgPullErr:
		var pe pullErr
		if err := json.Unmarshal(payload, &pe); err != nil {
			return err
		}
		if pe.Code == "not_found" {
			return ErrBlobNotFound
		}
		return fmt.Errorf("peer pull error: %s: %s", pe.Code, pe.Message)
	case MsgPullOK:
		// continue
	default:
		return fmt.Errorf("unexpected message %q", typ)
	}
	var okMsg pullOK
	if err := json.Unmarshal(payload, &okMsg); err != nil {
		return err
	}
	if okMsg.ContentHash != contentHash {
		return fmt.Errorf("hash mismatch in pull_ok")
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		return err
	}
	tmp := destPath + ".partial"
	_ = os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = f.Close()
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()

	h := sha256.New()
	var got int64
	for got < okMsg.Size {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := sc.ReadChunk()
		if err != nil {
			return err
		}
		if _, err := f.Write(chunk); err != nil {
			return err
		}
		_, _ = h.Write(chunk)
		got += int64(len(chunk))
	}
	if got != okMsg.Size {
		return fmt.Errorf("size mismatch: got %d want %d", got, okMsg.Size)
	}

	var done pullDone
	if err := sc.ReadJSON(&done); err != nil {
		return err
	}
	if done.Type != MsgPullDone {
		return fmt.Errorf("expected pull_done, got %q", done.Type)
	}

	sum := hex.EncodeToString(h.Sum(nil))
	if sum != contentHash {
		return fmt.Errorf("content hash mismatch: got %s want %s", sum, contentHash)
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, destPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// PullFrom dials addr, handshakes, pulls one file, then closes.
func PullFrom(ctx context.Context, addr string, id Identity, allow PeerAllowFunc, contentHash, destPath string) error {
	sess, conn, err := Dial(ctx, addr, id, allow)
	if err != nil {
		return err
	}
	defer conn.Close()
	sc, err := NewSecureConn(conn, sess)
	if err != nil {
		return err
	}
	return PullFile(ctx, sc, contentHash, destPath)
}

func (s *SecureConn) readControl() (typ string, raw []byte, err error) {
	plain, err := s.readSealed()
	if err != nil {
		return "", nil, err
	}
	if len(plain) < 1 || plain[0] != frameJSON {
		return "", nil, fmt.Errorf("expected JSON control frame")
	}
	raw = plain[1:]
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return "", nil, err
	}
	return probe.Type, raw, nil
}
