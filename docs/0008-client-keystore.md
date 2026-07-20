## Decision 8
Question
--------
How does a device persist its private key and bearer token on disk so it can
restart without re-linking?

Options
-------
A. Plaintext JSON file in the app data directory
B. OS keychain / credential store only (DPAPI, Keychain, libsecret)
C. Local file encrypted with a key derived from an unlock passphrase
D. Hybrid: file encrypted with a randomly generated key wrapped by OS
   keychain (or DPAPI on Windows) when available; passphrase fallback

Decision
--------
D for Phase 2. Store a single keystore file under the user config dir
(e.g. `%AppData%/sync_engine/keystore.json` on Windows). Contents:

- `user_id`, `device_id`, `coord_url`
- `public_key` (hex)
- `private_key` ciphertext (Ed25519 seed/private key)
- `token` ciphertext (bearer)
- `kdf` / `wrap` metadata (which protector was used)

Default protector on Windows: DPAPI (`CryptProtectData`) so the user is not
prompted for a passphrase on every start. Optional passphrase-based AES-256-GCM
wrap as an explicit opt-in for portable/shared machines.

The coordination server never sees the private key after issuance and never
stores the plaintext token.

Reason
------
Plaintext on disk is unacceptable once the agent is real. Full OS-keychain-only
APIs differ per platform and complicate the first Windows-focused agent.
DPAPI-backed file encryption is enough for v1 desktop, keeps the file
inspectable for debugging (ciphertext + metadata), and leaves a clean path to
macOS Keychain / Android Keystore later without changing the logical fields.
Passphrase wrap covers the "USB stick / shared PC" case without forcing it on
everyone.
