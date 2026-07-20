package db

import (
	"database/sql"
	"strings"
)

// migrate applies additive schema changes for existing SQLite files.
// CREATE TABLE IF NOT EXISTS does not add new columns to old DBs.
func migrate(db *sql.DB) error {
	stmts := []string{
		`ALTER TABLE devices ADD COLUMN revoked_at TEXT`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			if isDuplicateColumn(err) {
				continue
			}
			return err
		}
	}
	return nil
}

func isDuplicateColumn(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column")
}
