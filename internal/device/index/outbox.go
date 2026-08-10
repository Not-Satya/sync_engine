package index

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

// OutboxItem is a locally generated metadata event awaiting push to the
// coordinator (P4.5 drains this). It mirrors model.FolderEvent minus the
// server-assigned Seq. event_id gives the coordinator idempotency.
type OutboxItem struct {
	EventID     string
	FolderID    string
	Op          model.MetaOp
	Path        string
	OldPath     string
	Size        int64
	ContentHash string
	ModTime     time.Time
	HLCWall     int64
	HLCCounter  int64
	CreatedAt   time.Time
}

// EnqueueOutbox stores a pending event. Duplicate event_ids are ignored so a
// re-scan of the same change does not queue it twice.
func (s *Store) EnqueueOutbox(ctx context.Context, it OutboxItem) error {
	if it.EventID == "" || it.FolderID == "" {
		return fmt.Errorf("outbox: event_id and folder_id required")
	}
	path, err := NormalizePath(it.Path)
	if err != nil {
		return err
	}
	it.Path = path
	if it.CreatedAt.IsZero() {
		it.CreatedAt = time.Now().UTC()
	}
	var mtime any
	if !it.ModTime.IsZero() {
		mtime = it.ModTime.UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO outbox (
			event_id, folder_id, op, path, old_path,
			size, content_hash, mtime, hlc_wall, hlc_counter, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(event_id) DO NOTHING`,
		it.EventID, it.FolderID, string(it.Op), it.Path, it.OldPath,
		it.Size, it.ContentHash, mtime, it.HLCWall, it.HLCCounter,
		it.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// ListOutbox returns pending events for a folder, oldest first (by HLC then id).
func (s *Store) ListOutbox(ctx context.Context, folderID string, limit int) ([]OutboxItem, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, folder_id, op, path, old_path,
		       size, content_hash, mtime, hlc_wall, hlc_counter, created_at
		FROM outbox
		WHERE folder_id = ?
		ORDER BY hlc_wall, hlc_counter, event_id
		LIMIT ?`, folderID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxItem
	for rows.Next() {
		it, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// OutboxCount returns the number of pending events for a folder.
func (s *Store) OutboxCount(ctx context.Context, folderID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM outbox WHERE folder_id = ?`, folderID).Scan(&n)
	return n, err
}

// AckOutbox removes events that the coordinator has accepted.
func (s *Store) AckOutbox(ctx context.Context, eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `DELETE FROM outbox WHERE event_id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range eventIDs {
		if _, err := stmt.ExecContext(ctx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanOutbox(row scannable) (OutboxItem, error) {
	var it OutboxItem
	var op string
	var mtime sql.NullString
	var created string
	if err := row.Scan(
		&it.EventID, &it.FolderID, &op, &it.Path, &it.OldPath,
		&it.Size, &it.ContentHash, &mtime, &it.HLCWall, &it.HLCCounter, &created,
	); err != nil {
		return OutboxItem{}, err
	}
	it.Op = model.MetaOp(op)
	if mtime.Valid && mtime.String != "" {
		t, err := time.Parse(time.RFC3339Nano, mtime.String)
		if err != nil {
			return OutboxItem{}, err
		}
		it.ModTime = t
	}
	if created != "" {
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return OutboxItem{}, err
		}
		it.CreatedAt = t
	}
	return it, nil
}
