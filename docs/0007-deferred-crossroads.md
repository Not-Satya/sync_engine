## Decision 7 (deferred — not locked in Phase 1)
Question
--------
What do we choose for conflict resolution, chunking, and realtime metadata
transport when those phases arrive?

Options
-------
Conflict clocks: Hybrid Logical Clock (HLC) | vector clocks | CRDT
Chunking: fixed-size | content-defined (Rabin) | whole-file + bsdiff only
Realtime metadata: WebSocket | long poll | push wake (FCM) + pull

Decision
--------
Deferred. Tentative lean (not binding): HLC + LWW for personal file metadata;
content-defined chunking evaluated before locking go-bsdiff as the primary
delta path; WebSocket for metadata fan-out (see ADR 4).

Reason
------
These are Layer 3 choices. Locking them before a folder subscription model and
metadata sync loop exists produces speculative code. Phase 1 only needs
identity, auth, folders, subscriptions, and presence.

