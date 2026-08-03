package bindings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrInvalidPath = errors.New("invalid path")
	ErrNotDir      = errors.New("path is not a directory")
)

// ValidateAndNormalizePath resolves path to an absolute real directory suitable for binding.
// Policy (ADR 13): Abs + EvalSymlinks, must exist, must be a directory.
func ValidateAndNormalizePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidPath)
	}

	cleaned := filepath.Clean(path)
	// After Clean, a lingering ".." volume-relative escape is rejected.
	if containsDotDot(cleaned) {
		return "", fmt.Errorf("%w: path must not contain '..' after cleaning", ErrInvalidPath)
	}

	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("%w: resolve absolute: %v", ErrInvalidPath, err)
	}
	abs = filepath.Clean(abs)

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Distinguish missing path from other symlink errors.
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: does not exist: %s", ErrInvalidPath, abs)
		}
		// On some platforms EvalSymlinks fails if a component is missing.
		if _, statErr := os.Stat(abs); os.IsNotExist(statErr) {
			return "", fmt.Errorf("%w: does not exist: %s", ErrInvalidPath, abs)
		}
		return "", fmt.Errorf("%w: resolve symlinks: %v", ErrInvalidPath, err)
	}
	real = filepath.Clean(real)

	info, err := os.Stat(real)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: does not exist: %s", ErrInvalidPath, real)
		}
		return "", fmt.Errorf("%w: stat: %v", ErrInvalidPath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s", ErrNotDir, real)
	}
	return real, nil
}

// PathsEqual reports whether two filesystem paths refer to the same binding location.
// On Windows comparison is case-insensitive after Clean.
func PathsEqual(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func containsDotDot(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	// Also check slash form on Windows inputs that used '/'.
	if filepath.Separator != '/' {
		for _, part := range strings.Split(path, "/") {
			if part == ".." {
				return true
			}
		}
	}
	return false
}
