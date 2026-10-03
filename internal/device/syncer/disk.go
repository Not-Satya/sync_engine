package syncer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Not-Satya/sync_engine/internal/device/index"
)

// RemoveLocalCopy frees disk on this device only (ADR 29): sets omit_local on
// a live index entry and unlinks the file under root. It does not enqueue
// MetaOpDelete — peers keep their copies and the coordinator notebook is unchanged.
func RemoveLocalCopy(ctx context.Context, idx *index.Store, folderID, relPath, root string) error {
	if idx == nil {
		return fmt.Errorf("syncer: nil index")
	}
	if err := idx.SetOmitLocal(ctx, folderID, relPath, true); err != nil {
		return err
	}
	return UnlinkUnderRoot(root, relPath)
}

// ClearOmitLocal clears the device-local omit flag so a later fetch can
// materialize the blob again (ADR 29 / 30). Does not pull bytes by itself.
func ClearOmitLocal(ctx context.Context, idx *index.Store, folderID, relPath string) error {
	if idx == nil {
		return fmt.Errorf("syncer: nil index")
	}
	return idx.SetOmitLocal(ctx, folderID, relPath, false)
}

// UnlinkUnderRoot removes a regular file at relPath under root after verifying
// the resolved path cannot escape the folder binding (ADR 28). Missing files
// and directories are no-ops. Symlinks are removed as links (not followed).
func UnlinkUnderRoot(root, relPath string) error {
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("syncer: empty local root")
	}
	norm, err := index.NormalizePath(relPath)
	if err != nil {
		return err
	}
	rootClean := filepath.Clean(root)
	abs := filepath.Clean(filepath.Join(rootClean, filepath.FromSlash(norm)))
	rel, err := filepath.Rel(rootClean, abs)
	if err != nil {
		return fmt.Errorf("syncer: path escape: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("syncer: path escapes folder root: %s", relPath)
	}

	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		// Phase 6 v1: files are the unit; leave directories alone.
		return nil
	}
	return os.Remove(abs)
}
