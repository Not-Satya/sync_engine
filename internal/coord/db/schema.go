package db

// Schema is the Phase 1 coordination store.
// No table holds file bytes, chunk payloads, or object blobs.
const Schema = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS users (
    user_id       TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT NOT NULL,
    created_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS devices (
    device_id  TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    platform   TEXT NOT NULL DEFAULT '',
    public_key BLOB NOT NULL,
    created_at TEXT NOT NULL,
    last_seen  TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_devices_user ON devices(user_id);

CREATE TABLE IF NOT EXISTS auth_tokens (
    token_hash TEXT PRIMARY KEY,
    device_id  TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_auth_tokens_device ON auth_tokens(device_id);

CREATE TABLE IF NOT EXISTS folders (
    folder_id  TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_folders_owner ON folders(owner_id);

CREATE TABLE IF NOT EXISTS subscriptions (
    folder_id     TEXT NOT NULL REFERENCES folders(folder_id) ON DELETE CASCADE,
    device_id     TEXT NOT NULL REFERENCES devices(device_id) ON DELETE CASCADE,
    subscribed_at TEXT NOT NULL,
    PRIMARY KEY (folder_id, device_id)
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_device ON subscriptions(device_id);

CREATE TABLE IF NOT EXISTS presence (
    device_id  TEXT PRIMARY KEY REFERENCES devices(device_id) ON DELETE CASCADE,
    status     TEXT NOT NULL CHECK (status IN ('online', 'offline')),
    endpoint   TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
`
