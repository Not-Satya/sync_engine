package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrNotFound = errors.New("index entry not found")
	// ErrNotAlive means the path is missing or already a tombstone — cannot omit.
	ErrNotAlive = errors.New("index entry is not a live file")
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
    omit_local   INTEGER NOT NULL DEFAULT 0,
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("index dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open index: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("index schema: %w", err)
	}
	if err := migrateOmitLocal(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("index migrate omit_local: %w", err)
	}
	return &Store{db: db}, nil
}

// migrateOmitLocal adds omit_local to DBs created before ADR 29.
func migrateOmitLocal(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(file_entries)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == "omit_local" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE file_entries ADD COLUMN omit_local INTEGER NOT NULL DEFAULT 0`)
	return err
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
	omit := 0
	if e.OmitLocal {
		omit = 1
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO file_entries (
			folder_id, path, size, content_hash, mtime,
			hlc_wall, hlc_counter, deleted, omit_local, device_id, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(folder_id, path) DO UPDATE SET
			size = excluded.size,
			content_hash = excluded.content_hash,
			mtime = excluded.mtime,
			hlc_wall = excluded.hlc_wall,
			hlc_counter = excluded.hlc_counter,
			deleted = excluded.deleted,
			omit_local = excluded.omit_local,
			device_id = excluded.device_id,
			updated_at = excluded.updated_at`,
		e.FolderID, e.Path, e.Size, e.ContentHash, mtime,
		e.HLCWall, e.HLCCounter, deleted, omit, e.DeviceID,
		e.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// ApplyRemote applies an entry using HLC LWW. Returns true if the index changed.
// OmitLocal is device-local (ADR 29): preserved when content_hash is unchanged,
// cleared on tombstone or when a peer publishes a different hash.
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
		switch {
		case e.Deleted:
			e.OmitLocal = false
		case existing.ContentHash == e.ContentHash:
			e.OmitLocal = existing.OmitLocal
		default:
			e.OmitLocal = false
		}
	} else {
		e.OmitLocal = false
	}
	if err := s.Upsert(ctx, e); err != nil {
		return false, err
	}
	return true, nil
}

// SetOmitLocal marks or clears the device-local blob omit flag (ADR 29).
// The entry must be live (not a tombstone). Does not enqueue metadata events.
func (s *Store) SetOmitLocal(ctx context.Context, folderID, path string, omit bool) error {
	path, err := NormalizePath(path)
	if err != nil {
		return err
	}
	existing, err := s.Get(ctx, folderID, path)
	if err != nil {
		return err
	}
	if existing.Deleted {
		return ErrNotAlive
	}
	flag := 0
	if omit {
		flag = 1
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE file_entries SET omit_local = ?, updated_at = ?
		WHERE folder_id = ? AND path = ? AND deleted = 0`,
		flag, time.Now().UTC().Format(time.RFC3339Nano), folderID, path)
	return err
}

// Get returns an entry including tombstones.
func (s *Store) Get(ctx context.Context, folderID, path string) (Entry, error) {
	path, err := NormalizePath(path)
	if err != nil {
		return Entry{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT folder_id, path, size, content_hash, mtime,
		       hlc_wall, hlc_counter, deleted, omit_local, device_id, updated_at
		FROM file_entries WHERE folder_id = ? AND path = ?`, folderID, path)
	return scanEntry(row)
}

// List returns non-deleted entries for a folder (includes omit_local rows).
func (s *Store) List(ctx context.Context, folderID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT folder_id, path, size, content_hash, mtime,
		       hlc_wall, hlc_counter, deleted, omit_local, device_id, updated_at
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

// FindByContentHash returns non-deleted entries with the given content hash
// across all folders (used by the P2P blob server).
func (s *Store) FindByContentHash(ctx context.Context, contentHash string) ([]Entry, error) {
	if contentHash == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT folder_id, path, size, content_hash, mtime,
		       hlc_wall, hlc_counter, deleted, omit_local, device_id, updated_at
		FROM file_entries
		WHERE content_hash = ? AND deleted = 0
		ORDER BY folder_id, path`, contentHash)
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

// CountOmitted returns live entries with omit_local set for a folder (ADR 29).
func (s *Store) CountOmitted(ctx context.Context, folderID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM file_entries
		WHERE folder_id = ? AND deleted = 0 AND omit_local = 1`, folderID).Scan(&n)
	return n, err
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
	var deleted, omit int
	var updated string
	if err := row.Scan(
		&e.FolderID, &e.Path, &e.Size, &e.ContentHash, &mtime,
		&e.HLCWall, &e.HLCCounter, &deleted, &omit, &e.DeviceID, &updated,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	e.Deleted = deleted != 0
	e.OmitLocal = omit != 0
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
