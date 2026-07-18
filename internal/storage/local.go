package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const chunkSize = 1024 * 1024 // 1 MiB for resumable uploads

type LocalStorage struct {
	root string
	mu   sync.RWMutex
}

// Builder
func NewLocalStorage(root string) (*LocalStorage, error) {
	dirs := []string{
		filepath.Join(root, "objects"),
		filepath.Join(root, "index"),
		filepath.Join(root, "uploads"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return &LocalStorage{root: root}, nil
}

// objectPath and indexPath maps a storage key to its corresponding
// file or metadata location under the file or index directory.
//
// TODO: Implement a robust path validation to prevent traversal attacks.
func (s *LocalStorage) objectPath(key string) string {
	safe := strings.ReplaceAll(key, ". .", "")
	return filepath.Join(s.root, "object", filepath.FromSlash(safe))
}

func (s *LocalStorage) indexPath(key string) string {
	safe := strings.ReplaceAll(key, ". .", "")
	return filepath.Join(s.root, "index", filepath.FromSlash(safe)+".json")
}

func (s *LocalStorage) loadMeta(key string) (ObjectMeta, error) {
	data, err := os.ReadFile(s.indexPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return ObjectMeta{}, ErrNotFound
		}
		return ObjectMeta{}, err
	}
	var meta ObjectMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return ObjectMeta{}, err
	}
	return meta, nil
}

func (s *LocalStorage) saveMeta(meta ObjectMeta) error {
	if err := os.MkdirAll(filepath.Dir(s.indexPath(meta.Key)), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath(meta.Key), data, 0o644)
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]ObjectMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	indexRoot := filepath.Join(s.root, "index")
	var result []ObjectMeta

	err := filepath.WalkDir(indexRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var meta ObjectMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return err
		}
		if meta.Deleted {
			return nil
		}
		if prefix == "" || strings.HasPrefix(meta.Key, prefix) {
			result = append(result, meta)
		}
		return nil
	})
	return result, err
}

func (s *LocalStorage) Stat(ctx context.Context, key string) (ObjectMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	select {
	case <-ctx.Done():
		return ObjectMeta{}, ctx.Err()
	default:
	}
	meta, err := s.loadMeta(key)
	if err != nil {
		return ObjectMeta{}, err
	}
	if meta.Deleted {
		return ObjectMeta{}, ErrNotFound
	}
	return meta, nil
}
