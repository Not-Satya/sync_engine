package index

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	FileName   = "file_index.db"
	AppDirName = "sync_engine"
)

// Entry is local metadata for one relative path inside a sync folder.
// File bytes live on disk under the binding path — never in this DB.
type Entry struct {
	FolderID    string
	Path        string // relative, forward slashes
	Size        int64
	ContentHash string
	ModTime     time.Time
	HLCWall     int64
	HLCCounter  int64
	Deleted     bool
	DeviceID    string // last writer device (for LWW tie-break)
	UpdatedAt   time.Time
}

// NormalizePath cleans a relative path to a canonical forward-slash form.
func NormalizePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, `\`, `/`)
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		return "", fmt.Errorf("empty path")
	}
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("path must not contain '..'")
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return "", fmt.Errorf("empty path")
	}
	return strings.Join(out, "/"), nil
}

// HLCLess reports whether a is strictly less than b (ADR 16 LWW ordering).
func HLCLess(aWall, aCounter int64, aDevice string, bWall, bCounter int64, bDevice string) bool {
	if aWall != bWall {
		return aWall < bWall
	}
	if aCounter != bCounter {
		return aCounter < bCounter
	}
	return aDevice < bDevice
}

// DefaultPath returns %AppData%/sync_engine/file_index.db (or OS equivalent).
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, AppDirName, FileName), nil
}
