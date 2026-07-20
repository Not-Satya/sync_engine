package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrRevoked      = errors.New("device revoked")
)

// Store is the coordination persistence layer. It never stores file bytes.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite writer simplicity for v1
	if _, err := db.Exec(Schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateUser(ctx context.Context, u model.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (user_id, email, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		u.UserID, u.Email, u.PasswordHash, u.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (model.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id, email, password_hash, created_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (s *Store) UserByID(ctx context.Context, userID string) (model.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT user_id, email, password_hash, created_at FROM users WHERE user_id = ?`, userID)
	return scanUser(row)
}

func (s *Store) CreateDevice(ctx context.Context, d model.Device, token model.AuthToken) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO devices (device_id, user_id, name, platform, public_key, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.DeviceID, d.UserID, d.Name, d.Platform, d.PublicKey,
		d.CreatedAt.UTC().Format(time.RFC3339Nano),
		d.LastSeen.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}

	var expires any
	if !token.ExpiresAt.IsZero() {
		expires = token.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO auth_tokens (token_hash, device_id, user_id, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		token.TokenHash, token.DeviceID, token.UserID,
		token.CreatedAt.UTC().Format(time.RFC3339Nano), expires,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO presence (device_id, status, endpoint, updated_at)
		VALUES (?, 'offline', '', ?)`,
		d.DeviceID, d.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeviceByID(ctx context.Context, deviceID string) (model.Device, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT device_id, user_id, name, platform, public_key, created_at, last_seen, revoked_at
		FROM devices WHERE device_id = ?`, deviceID)
	return scanDevice(row)
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]model.Device, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT device_id, user_id, name, platform, public_key, created_at, last_seen, revoked_at
		FROM devices WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RevokeDevice soft-revokes a device: sets revoked_at, deletes tokens, forces offline.
// Caller must already have verified the acting device owns the same account.
func (s *Store) RevokeDevice(ctx context.Context, deviceID string, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var revoked sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT revoked_at FROM devices WHERE device_id = ?`, deviceID,
	).Scan(&revoked)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if revoked.Valid && revoked.String != "" {
		return ErrRevoked // already revoked — idempotent enough to treat as done? use ErrRevoked for 409
	}

	res, err := tx.ExecContext(ctx,
		`UPDATE devices SET revoked_at = ? WHERE device_id = ? AND (revoked_at IS NULL OR revoked_at = '')`,
		at.UTC().Format(time.RFC3339Nano), deviceID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrRevoked
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM auth_tokens WHERE device_id = ?`, deviceID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE presence SET status = 'offline', endpoint = '', updated_at = ?
		WHERE device_id = ?`,
		at.UTC().Format(time.RFC3339Nano), deviceID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) AuthByTokenHash(ctx context.Context, tokenHash string) (model.AuthToken, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT token_hash, device_id, user_id, created_at, expires_at
		FROM auth_tokens WHERE token_hash = ?`, tokenHash)
	var t model.AuthToken
	var created string
	var expires sql.NullString
	if err := row.Scan(&t.TokenHash, &t.DeviceID, &t.UserID, &created, &expires); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.AuthToken{}, ErrUnauthorized
		}
		return model.AuthToken{}, err
	}
	var err error
	t.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return model.AuthToken{}, err
	}
	if expires.Valid && expires.String != "" {
		t.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return model.AuthToken{}, err
		}
		if time.Now().After(t.ExpiresAt) {
			return model.AuthToken{}, ErrUnauthorized
		}
	}
	return t, nil
}

func (s *Store) TouchDevice(ctx context.Context, deviceID string, at time.Time) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE devices SET last_seen = ? WHERE device_id = ?`,
		at.UTC().Format(time.RFC3339Nano), deviceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CreateFolder(ctx context.Context, f model.Folder) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO folders (folder_id, owner_id, name, created_at) VALUES (?, ?, ?, ?)`,
		f.FolderID, f.OwnerID, f.Name, f.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) FolderByID(ctx context.Context, folderID string) (model.Folder, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT folder_id, owner_id, name, created_at FROM folders WHERE folder_id = ?`, folderID)
	var f model.Folder
	var created string
	if err := row.Scan(&f.FolderID, &f.OwnerID, &f.Name, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Folder{}, ErrNotFound
		}
		return model.Folder{}, err
	}
	var err error
	f.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return f, err
}

func (s *Store) ListFolders(ctx context.Context, userID string) ([]model.Folder, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT folder_id, owner_id, name, created_at FROM folders
		WHERE owner_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Folder
	for rows.Next() {
		var f model.Folder
		var created string
		if err := rows.Scan(&f.FolderID, &f.OwnerID, &f.Name, &created); err != nil {
			return nil, err
		}
		f.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) Subscribe(ctx context.Context, sub model.Subscription) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO subscriptions (folder_id, device_id, subscribed_at) VALUES (?, ?, ?)
		ON CONFLICT(folder_id, device_id) DO NOTHING`,
		sub.FolderID, sub.DeviceID, sub.SubscribedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) Unsubscribe(ctx context.Context, folderID, deviceID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM subscriptions WHERE folder_id = ? AND device_id = ?`, folderID, deviceID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListSubscriptionsByDevice(ctx context.Context, deviceID string) ([]model.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT folder_id, device_id, subscribed_at FROM subscriptions
		WHERE device_id = ? ORDER BY subscribed_at`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Subscription
	for rows.Next() {
		var sub model.Subscription
		var at string
		if err := rows.Scan(&sub.FolderID, &sub.DeviceID, &at); err != nil {
			return nil, err
		}
		var err error
		sub.SubscribedAt, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Store) UpsertPresence(ctx context.Context, p model.Presence) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO presence (device_id, status, endpoint, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_id) DO UPDATE SET
			status = excluded.status,
			endpoint = excluded.endpoint,
			updated_at = excluded.updated_at`,
		p.DeviceID, string(p.Status), p.Endpoint, p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) PresenceByDevice(ctx context.Context, deviceID string) (model.Presence, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT device_id, status, endpoint, updated_at FROM presence WHERE device_id = ?`, deviceID)
	return scanPresence(row)
}

func (s *Store) ListPresenceForUser(ctx context.Context, userID string) ([]model.Presence, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.device_id, p.status, p.endpoint, p.updated_at
		FROM presence p
		JOIN devices d ON d.device_id = p.device_id
		WHERE d.user_id = ?
		ORDER BY p.updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Presence
	for rows.Next() {
		p, err := scanPresence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ExpireStalePresence marks devices offline when heartbeat TTL has elapsed.
func (s *Store) ExpireStalePresence(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE presence SET status = 'offline', updated_at = ?
		WHERE status = 'online' AND updated_at < ?`,
		time.Now().UTC().Format(time.RFC3339Nano),
		olderThan.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (model.User, error) {
	var u model.User
	var created string
	if err := row.Scan(&u.UserID, &u.Email, &u.PasswordHash, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, ErrNotFound
		}
		return model.User{}, err
	}
	var err error
	u.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return u, err
}

func scanDevice(row scannable) (model.Device, error) {
	var d model.Device
	var created, lastSeen string
	var revoked sql.NullString
	if err := row.Scan(&d.DeviceID, &d.UserID, &d.Name, &d.Platform, &d.PublicKey, &created, &lastSeen, &revoked); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Device{}, ErrNotFound
		}
		return model.Device{}, err
	}
	var err error
	d.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return model.Device{}, err
	}
	d.LastSeen, err = time.Parse(time.RFC3339Nano, lastSeen)
	if err != nil {
		return model.Device{}, err
	}
	if revoked.Valid && revoked.String != "" {
		t, err := time.Parse(time.RFC3339Nano, revoked.String)
		if err != nil {
			return model.Device{}, err
		}
		d.RevokedAt = &t
	}
	return d, nil
}

func scanPresence(row scannable) (model.Presence, error) {
	var p model.Presence
	var status, updated string
	if err := row.Scan(&p.DeviceID, &status, &p.Endpoint, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Presence{}, ErrNotFound
		}
		return model.Presence{}, err
	}
	p.Status = model.PresenceStatus(status)
	var err error
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return p, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") ||
		strings.Contains(msg, "constraint failed")
}
