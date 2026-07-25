## Decision 2
Question
--------
How do devices authenticate to the coordination server in v1?

Options
-------
A. Password + JWT (short-lived access, refresh token)
B. Per-device opaque bearer token (issued at device link, hashed at rest)
C. mTLS / device certificates only
D. OAuth / OIDC third-party IdP

Decision
--------
B for Phase 1, with a thin account password only for initial user registration
and first device link. Subsequent devices receive their own opaque bearer
token. Tokens are stored as SHA-256 hashes; plaintext is shown once at issue.

Reason
------
The sync engine's primary actor is a *device*, not a browser session. Opaque
per-device tokens map cleanly to revoke-one-device without killing the account.
JWTs add expiry/refresh machinery we do not need before metadata sync exists.
mTLS and OIDC are right later for hardened linking; they overcomplicate the
skeleton. Password hashes (argon2id) protect account bootstrap only — devices
present bearer tokens on every coordination call.
