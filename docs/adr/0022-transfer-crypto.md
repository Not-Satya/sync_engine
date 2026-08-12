## Decision 22
Phase: 5
Subphase: P5.0 / P5.2 / P5.3
Question
--------
How do two devices authenticate each other and encrypt the transfer stream?

Options
-------
A. Trust TCP on LAN; no crypto (demo only)
B. TLS with coordinator-issued certificates
C. Mutual Ed25519 challenge using each device's existing keypair, then
   ECDH-derived (or similar) session key with AES-256-GCM for the stream
D. Reuse the opaque bearer token over the P2P socket

Decision
--------
C. Devices already have Ed25519 keys (ADR 3) and DeviceIDs derived from
public keys. Transfer handshake:

1. Dialer and listener exchange DeviceID + public key (and prove
   possession with a signature over a nonce).
2. Reject if DeviceID is not on the same account / not a known peer for
   this folder (caller got the peer list from the coordinator).
3. Derive a one-time session key; all subsequent frames are AES-256-GCM
   (ADR 5 stack confirmation).

Bearer tokens (D) stay on the coordinator HTTP path only — they must not
be sent to peers. Plain LAN trust (A) is unacceptable once keys exist.
Full PKI/TLS (B) is heavier than needed for personal multi-device v1.

Reason
------
Key-derived DeviceIDs were chosen in Phase 1 specifically to unlock peer
auth without a redesign. AES-256-GCM was stacked for transfer encryption.
Keeping coordinator bearer tokens off the P2P wire preserves revoke
semantics and avoids teaching devices to impersonate each other with a
stolen HTTP token.
