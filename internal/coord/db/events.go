package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

// IsDeviceSubscribed reports whether deviceID has an active subscription to folderID.
func (s *Store) IsDeviceSubscribed(ctx context.Context, folderID, deviceID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(1) FROM subscriptions WHERE folder_id = ? AND device_id = ?`,
		folderID, deviceID,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// AppendFolderEvents inserts metadata events, assigning seq. Duplicate (folder_id, event_id) are skipped.
// Returns the events as stored (including seq), in insertion order for newly accepted rows;
// duplicates are omitted from the result but do not fail the batch.
func (s *Store) AppendFolderEvents(ctx context.Context, folderID string, events []model.FolderEvent) ([]model.FolderEvent, error) {
	if folderID == "" {
		return nil, fmt.Errorf("folder_id required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	out := make([]model.FolderEvent, 0, len(events))
	now := time.Now().UTC()
	for _, ev := range events {
		if ev.EventID == "" || ev.DeviceID == "" || ev.Path == "" {
			return nil, fmt.Errorf("event_id, device_id, and path required")
		}
		if ev.FolderID == "" {
			ev.FolderID = folderID
		}
		if ev.FolderID != folderID {
			return nil, fmt.Errorf("event folder_id mismatch")
		}
		if err := validateMetaOp(ev.Op); err != nil {
			return nil, err
		}
		if ev.CreatedAt.IsZero() {
			ev.CreatedAt = now
		}

		var mtime any
		if !ev.ModTime.IsZero() {
			mtime = ev.ModTime.UTC().Format(time.RFC3339Nano)
		}
		var oldPath any
		if ev.OldPath != "" {
			oldPath = ev.OldPath
		}

		res, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO folder_events (
				event_id, folder_id, device_id, op, path, old_path,
				size, content_hash, mtime, hlc_wall, hlc_counter, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ev.EventID, ev.FolderID, ev.DeviceID, string(ev.Op), ev.Path, oldPath,
			ev.Size, ev.ContentHash, mtime, ev.HLC.Wall, ev.HLC.Counter,
			ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			continue // duplicate event_id for this folder
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		ev.Seq = id
		out = append(out, ev)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFolderEventsSince returns events with seq > since for folderID, ascending, limited.
func (s *Store) ListFolderEventsSince(ctx context.Context, folderID string, since int64, limit int) ([]model.FolderEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT seq, event_id, folder_id, device_id, op, path, old_path,
		       size, content_hash, mtime, hlc_wall, hlc_counter, created_at
		FROM folder_events
		WHERE folder_id = ? AND seq > ?
		ORDER BY seq ASC
		LIMIT ?`, folderID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.FolderEvent
	for rows.Next() {
		ev, err := scanFolderEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// MaxFolderEventSeq returns the highest seq for a folder (0 if none).
func (s *Store) MaxFolderEventSeq(ctx context.Context, folderID string) (int64, error) {
	var seq sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(seq) FROM folder_events WHERE folder_id = ?`, folderID).Scan(&seq)
	if err != nil {
		return 0, err
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}

func validateMetaOp(op model.MetaOp) error {
	switch op {
	case model.MetaOpUpsert, model.MetaOpDelete, model.MetaOpRename:
		return nil
	default:
		return fmt.Errorf("invalid op %q", op)
	}
}

func scanFolderEvent(row scannable) (model.FolderEvent, error) {
	var ev model.FolderEvent
	var op string
	var oldPath sql.NullString
	var mtime sql.NullString
	var created string
	if err := row.Scan(
		&ev.Seq, &ev.EventID, &ev.FolderID, &ev.DeviceID, &op, &ev.Path, &oldPath,
		&ev.Size, &ev.ContentHash, &mtime, &ev.HLC.Wall, &ev.HLC.Counter, &created,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.FolderEvent{}, ErrNotFound
		}
		return model.FolderEvent{}, err
	}
	ev.Op = model.MetaOp(op)
	if oldPath.Valid {
		ev.OldPath = oldPath.String
	}
	if mtime.Valid && mtime.String != "" {
		t, err := time.Parse(time.RFC3339Nano, mtime.String)
		if err != nil {
			return model.FolderEvent{}, err
		}
		ev.ModTime = t
	}
	var err error
	ev.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return model.FolderEvent{}, err
	}
	return ev, nil
}