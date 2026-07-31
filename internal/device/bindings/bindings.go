package bindings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	FileVersion = 1
	FileName    = "folder_bindings.json"
	AppDirName  = "sync_engine"
)

var (
	ErrNotFound = errors.New("binding not found")
	ErrCorrupt  = errors.New("bindings store corrupt")
	ErrConflict = errors.New("binding conflict")
)

// Binding maps a coordinator FolderID to a local directory path on this device.
// Absolute paths never leave the device (ADR 12).
type Binding struct {
	FolderID   string    `json:"folder_id"`
	LocalPath  string    `json:"local_path"`
	Name       string    `json:"name,omitempty"` // display hint; server name is authoritative
	Subscribed bool      `json:"subscribed"`
	BoundAt    time.Time `json:"bound_at"`
}

type fileDoc struct {
	Version  int       `json:"version"`
	Bindings []Binding `json:"bindings"`
}

// Store is an on-disk list of folder bindings for one device.
type Store struct {
	path string
}

// DefaultPath returns %AppData%/sync_engine/folder_bindings.json (or OS equivalent).
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	return filepath.Join(base, AppDirName, FileName), nil
}

// Open returns a store rooted at path (file is created on first Put).
func Open(path string) *Store {
	return &Store{path: path}
}

// Path returns the store file path.
func (s *Store) Path() string { return s.path }

// List returns all bindings (empty slice if the file does not exist yet).
func (s *Store) List() ([]Binding, error) {
	doc, err := s.read()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Binding, len(doc.Bindings))
	copy(out, doc.Bindings)
	return out, nil
}

// Get returns the binding for folderID.
func (s *Store) Get(folderID string) (Binding, error) {
	doc, err := s.read()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Binding{}, ErrNotFound
		}
		return Binding{}, err
	}
	for _, b := range doc.Bindings {
		if b.FolderID == folderID {
			return b, nil
		}
	}
	return Binding{}, ErrNotFound
}

// GetByPath returns the binding for an exact local_path string.
func (s *Store) GetByPath(localPath string) (Binding, error) {
	doc, err := s.read()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Binding{}, ErrNotFound
		}
		return Binding{}, err
	}
	for _, b := range doc.Bindings {
		if b.LocalPath == localPath {
			return b, nil
		}
	}
	return Binding{}, ErrNotFound
}

// Put inserts or replaces a binding by folder_id.
// Fails with ErrConflict if local_path is already used by a different folder_id.
func (s *Store) Put(b Binding) error {
	if b.FolderID == "" || b.LocalPath == "" {
		return fmt.Errorf("folder_id and local_path required")
	}
	if b.BoundAt.IsZero() {
		b.BoundAt = time.Now().UTC()
	}

	doc, err := s.read()
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if errors.Is(err, ErrNotFound) {
		doc = fileDoc{Version: FileVersion}
	}

	for _, existing := range doc.Bindings {
		if existing.LocalPath == b.LocalPath && existing.FolderID != b.FolderID {
			return fmt.Errorf("%w: path already bound to %s", ErrConflict, existing.FolderID)
		}
	}

	replaced := false
	for i, existing := range doc.Bindings {
		if existing.FolderID == b.FolderID {
			doc.Bindings[i] = b
			replaced = true
			break
		}
	}
	if !replaced {
		doc.Bindings = append(doc.Bindings, b)
	}
	doc.Version = FileVersion
	return s.write(doc)
}

// Remove deletes the binding for folderID.
func (s *Store) Remove(folderID string) error {
	doc, err := s.read()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	out := doc.Bindings[:0]
	found := false
	for _, b := range doc.Bindings {
		if b.FolderID == folderID {
			found = true
			continue
		}
		out = append(out, b)
	}
	if !found {
		return ErrNotFound
	}
	doc.Bindings = out
	return s.write(doc)
}

// SetSubscribed updates the local subscribed flag for folderID.
func (s *Store) SetSubscribed(folderID string, subscribed bool) error {
	b, err := s.Get(folderID)
	if err != nil {
		return err
	}
	b.Subscribed = subscribed
	return s.Put(b)
}

func (s *Store) read() (fileDoc, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileDoc{}, ErrNotFound
		}
		return fileDoc{}, err
	}
	var doc fileDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fileDoc{}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if doc.Version != 0 && doc.Version != FileVersion {
		return fileDoc{}, fmt.Errorf("%w: unsupported version %d", ErrCorrupt, doc.Version)
	}
	if doc.Bindings == nil {
		doc.Bindings = []Binding{}
	}
	return doc, nil
}

func (s *Store) write(doc fileDoc) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
