package transfer

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/Not-Satya/sync_engine/internal/device/bindings"
	"github.com/Not-Satya/sync_engine/internal/device/index"
)

// IndexBlobStore serves whole-file bytes by looking up content hashes in the
// local index and opening the matching path under a bound folder root.
// The coordinator never sees these bytes.
type IndexBlobStore struct {
	Index    *index.Store
	Bindings *bindings.Store
}

// Open finds a local file with contentHash and returns a reader.
func (s IndexBlobStore) Open(ctx context.Context, contentHash string) (int64, io.ReadCloser, error) {
	if s.Index == nil || s.Bindings == nil {
		return 0, nil, ErrBlobNotFound
	}
	entries, err := s.Index.FindByContentHash(ctx, contentHash)
	if err != nil {
		return 0, nil, err
	}
	for _, e := range entries {
		b, err := s.Bindings.Get(e.FolderID)
		if err != nil {
			continue
		}
		if h, _ := bindings.CheckPath(b.LocalPath); h != bindings.PathOK {
			continue
		}
		abs := filepath.Join(b.LocalPath, filepath.FromSlash(e.Path))
		f, err := os.Open(abs)
		if err != nil {
			continue
		}
		st, err := f.Stat()
		if err != nil {
			_ = f.Close()
			continue
		}
		if st.IsDir() {
			_ = f.Close()
			continue
		}
		return st.Size(), f, nil
	}
	return 0, nil, ErrBlobNotFound
}
