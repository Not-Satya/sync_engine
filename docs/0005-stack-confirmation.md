## Decision 5
Question
--------
Is the proposed stack (Go, chi, modernc.org/sqlite, fsnotify, AES-256-GCM,
go-bsdiff) correct for an offline-first P2P sync engine where the server
never stores file bytes?

Options
-------
A. Accept the stack as proposed
B. Challenge and replace pieces that conflict with the architecture
C. Accept for device engine; adjust server and defer transfer libs

Decision
--------
C — confirm for the engine path; constrain the server; defer transfer libs.

| Piece | Verdict | Notes |
|---|---|---|
| Go | Confirm | Shared protocol types client↔server; later gomobile/Wails embed |
| chi | Confirm | Coordination HTTP API; light, idiomatic |
| modernc.org/sqlite | Confirm | Device metadata *and* v1 coordination DB (see ADR 1) |
| fsnotify | Confirm, later | Device-only folder watch; not on the server |
| AES-256-GCM | Confirm, later | P2P transfer encryption; not coordination payloads in Phase 1 |
| go-bsdiff | Confirm with caution, later | Fine for delta patches; evaluate rolling-hash chunking (e.g. Rabin) before betting the whole transfer design on bsdiff |

Explicit rejects from the reference architecture doc:
- Server-side blob/object store (S3, local object backend as "source of truth")
- Treating `StorageBackend` Put/Get of file bytes on the server as sync
- Postgres+blob tables as a Phase 1 requirement

Reason
------
The stack fits a P2P design *if* file I/O, fsnotify, chunking, and encryption
live on devices, and the server only speaks coordination APIs. Existing
`internal/storage` (object-store style Put/Get/UploadSession) leans cloud-
backup; it is left untouched in Phase 1 and must not become the server's
data path. go-bsdiff is useful once we have full-file P2P transfer, but
chunking/content-addressing should be ADR'd in the transfer phase — bsdiff
alone is not a chunking strategy.
