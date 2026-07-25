package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

// CreatePairingCode stores a hashed pairing code. Plaintext is never persisted.
func (s *Store) CreatePairingCode(ctx context.Context, pc model.PairingCode) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pairing_codes (
			code_hash, user_id, created_by_device_id, created_at, expires_at, used_at, used_by_device_id
		) VALUES (?, ?, ?, ?, ?, NULL, NULL)`,
		pc.CodeHash, pc.UserID, pc.CreatedBy,
		pc.CreatedAt.UTC().Format(time.RFC3339Nano),
		pc.ExpiresAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// CancelPairingCode deletes an unused code created by the given device for its user.
func (s *Store) CancelPairingCode(ctx context.Context, codeHash, userID, createdByDeviceID string) error {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM pairing_codes
		WHERE code_hash = ? AND user_id = ? AND created_by_device_id = ?
		  AND used_at IS NULL`,
		codeHash, userID, createdByDeviceID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RedeemPairingCode consumes a valid unused code and creates the new device in one transaction.
func (s *Store) RedeemPairingCode(ctx context.Context, codeHash string, d model.Device, token model.AuthToken, at time.Time) (userID string, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		ownerID   string
		expiresAt string
		usedAt    sql.NullString
	)
	err = tx.QueryRowContext(ctx, `
		SELECT user_id, expires_at, used_at FROM pairing_codes WHERE code_hash = ?`, codeHash,
	).Scan(&ownerID, &expiresAt, &usedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if usedAt.Valid && usedAt.String != "" {
		return "", ErrConflict
	}
	exp, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return "", err
	}
	if at.After(exp) {
		return "", ErrExpired
	}

	d.UserID = ownerID
	token.UserID = ownerID

	_, err = tx.ExecContext(ctx, `
		INSERT INTO devices (device_id, user_id, name, platform, public_key, created_at, last_seen)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		d.DeviceID, d.UserID, d.Name, d.Platform, d.PublicKey,
		d.CreatedAt.UTC().Format(time.RFC3339Nano),
		d.LastSeen.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return "", ErrConflict
		}
		return "", err
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
		return "", err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO presence (device_id, status, endpoint, updated_at)
		VALUES (?, 'offline', '', ?)`,
		d.DeviceID, d.CreatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return "", err
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE pairing_codes
		SET used_at = ?, used_by_device_id = ?
		WHERE code_hash = ? AND used_at IS NULL`,
		at.UTC().Format(time.RFC3339Nano), d.DeviceID, codeHash,
	)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return ownerID, nil
}
