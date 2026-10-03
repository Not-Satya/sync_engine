## Decision 7 (deferred — resolved across Phase 4–5)
Phase: 1 deferred → Phase 4–5
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
**Phase 4 locked:**
- Conflict / ordering → HLC + LWW (ADR 16)
- Metadata transport → HTTP push/pull + poll first (ADR 19); WebSocket later
- Content identity for metadata → SHA-256 whole file (ADR 17)

**Phase 5 locked:**
- Chunking / transfer unit → whole-file by SHA-256 (ADR 20); CDC/bsdiff later
- Peer introduction → presence `endpoint` + folder peer list (ADR 21)
- Transfer crypto → Ed25519 mutual auth + AES-256-GCM (ADR 22)
- Transport → direct TCP; no TURN in MVP (ADR 23)
- Resume → retry whole file after temp+verify failure (ADR 24)

**Phase 6 locked:**
- FS delete → sync delete / tombstone (ADR 27); remove-local-copy is explicit
- Remote tombstone → unlink under binding root (ADR 28)
- Device-local `omit_local` on live index rows (ADR 29); not on the oplog
- CLI → `files remove-local` / `materialize` (ADR 30)

Reason
------
This ADR remains the index for originally deferred crossroads so readers
do not hunt across files. Detail lives in ADR 16–30.
