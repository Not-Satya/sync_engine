package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("index entry not found")
)

const schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS file_entries (
    folder_id    TEXT NOT NULL,
    path         TEXT NOT NULL,
    size         INTEGER NOT NULL DEFAULT 0,
    content_hash TEXT NOT NULL DEFAULT '',
    mtime        TEXT,
    hlc_wall     INTEGER NOT NULL,
    hlc_counter  INTEGER NOT NULL,
    deleted      INTEGER NOT NULL DEFAULT 0,
    device_id    TEXT NOT NULL DEFAULT '',
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (folder_id, path)
);

CREATE INDEX IF NOT EXISTS idx_file_entries_folder ON file_entries(folder_id);

CREATE TABLE IF NOT EXISTS folder_cursors (
    folder_id TEXT PRIMARY KEY,
    last_seq  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS outbox (
    event_id     TEXT PRIMARY KEY,
    folder_id    TEXT NOT NULL,
    op           TEXT NOT NULL,
    path         TEXT NOT NULL,
    old_path     TEXT NOT NULL DEFAULT '',
    size         INTEGER NOT NULL DEFAULT 0,
    content_hash TEXT NOT NULL DEFAULT '',
    mtime        TEXT,
    hlc_wall     INTEGER NOT NULL,
    hlc_counter  INTEGER NOT NULL,
    created_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_outbox_folder ON outbox(folder_id, event_id);
`

// Store is the device-local metadata index (SQLite). No file bytes.
type Store struct {
	db *sql.DB
}

// Open opens or creates the index database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("index schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Upsert writes an entry unconditionally (local mutation already stamped with HLC).
func (s *Store) Upsert(ctx context.Context, e Entry) error {
	path, err := NormalizePath(e.Path)
	if err != nil {
		return err
	}
	e.Path = path
	if e.FolderID == "" {
		return fmt.Errorf("folder_id required")
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = time.Now().UTC()
	}
	var mtime any
	if !e.ModTime.IsZero() {
		mtime = e.ModTime.UTC().Format(time.RFC3339Nano)
	}
	deleted := 0
	if e.Deleted {
		deleted = 1
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO file_entries (
			folder_id, path, size, content_hash, mtime,
			hlc_wall, hlc_counter, deleted, device_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(folder_id, path) DO UPDATE SET
			size = excluded.size,
			content_hash = excluded.content_hash,
			mtime = excluded.mtime,
			hlc_wall = excluded.hlc_wall,
			hlc_counter = excluded.hlc_counter,
			deleted = excluded.deleted,
			device_id = excluded.device_id,
			updated_at = excluded.updated_at`,
		e.FolderID, e.Path, e.Size, e.ContentHash, mtime,
		e.HLCWall, e.HLCCounter, deleted, e.DeviceID,
		e.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// ApplyRemote applies an entry using HLC LWW. Returns true if the index changed.
func (s *Store) ApplyRemote(ctx context.Context, e Entry) (bool, error) {
	path, err := NormalizePath(e.Path)
	if err != nil {
		return false, err
	}
	e.Path = path

	existing, err := s.Get(ctx, e.FolderID, e.Path)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return false, err
	}
	if err == nil {
		incomingWins := HLCLess(
			existing.HLCWall, existing.HLCCounter, existing.DeviceID,
			e.HLCWall, e.HLCCounter, e.DeviceID,
		)
		if !incomingWins {
			return false, nil
		}
	}
	if err := s.Upsert(ctx, e); err != nil {
		return false, err
	}
	return true, nil
}

// Get returns an entry including tombstones.
func (s *Store) Get(ctx context.Context, folderID, path string) (Entry, error) {
	path, err := NormalizePath(path)
	if err != nil {
		return Entry{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT folder_id, path, size, content_hash, mtime,
		       hlc_wall, hlc_counter, deleted, device_id, updated_at
		FROM file_entries WHERE folder_id = ? AND path = ?`, folderID, path)
	return scanEntry(row)
}

// List returns non-deleted entries for a folder.
func (s *Store) List(ctx context.Context, folderID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT folder_id, path, size, content_hash, mtime,
		       hlc_wall, hlc_counter, deleted, device_id, updated_at
		FROM file_entries
		WHERE folder_id = ? AND deleted = 0
		ORDER BY path`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Count returns alive and tombstone counts for a folder.
func (s *Store) Count(ctx context.Context, folderID string) (alive, tombstones int, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN deleted = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN deleted = 1 THEN 1 ELSE 0 END), 0)
		FROM file_entries WHERE folder_id = ?`, folderID,
	).Scan(&alive, &tombstones)
	if err != nil {
		return 0, 0, err
	}
	return alive, tombstones, nil
}

// Cursor returns the last pulled server seq for a folder.
func (s *Store) Cursor(ctx context.Context, folderID string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `
		SELECT last_seq FROM folder_cursors WHERE folder_id = ?`, folderID).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return seq, err
}

// SetCursor stores the last pulled server seq for a folder.
func (s *Store) SetCursor(ctx context.Context, folderID string, seq int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO folder_cursors (folder_id, last_seq) VALUES (?, ?)
		ON CONFLICT(folder_id) DO UPDATE SET last_seq = excluded.last_seq`,
		folderID, seq)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanEntry(row scannable) (Entry, error) {
	var e Entry
	var mtime sql.NullString
	var deleted int
	var updated string
	if err := row.Scan(
		&e.FolderID, &e.Path, &e.Size, &e.ContentHash, &mtime,
		&e.HLCWall, &e.HLCCounter, &deleted, &e.DeviceID, &updated,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	e.Deleted = deleted != 0
	if mtime.Valid && mtime.String != "" {
		t, err := time.Parse(time.RFC3339Nano, mtime.String)
		if err != nil {
			return Entry{}, err
		}
		e.ModTime = t
	}
	var err error
	e.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return e, err
}
