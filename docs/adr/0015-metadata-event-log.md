## Decision 15
Phase: 4
Subphase: P4.0 / P4.1
Question
--------
How does file metadata move between devices without the server storing file bytes?

Options
-------
A. Full folder snapshots uploaded to the server each sync
B. Append-only metadata event log per folder on the coordinator; devices
   push local changes and pull `since=cursor`
C. Devices gossip metadata peer-to-peer only (no server log)
D. CRDT document store on the server (Automerge-style)

Decision
--------
B. Per-folder append-only **metadata events** on the coordinator.

Event payload (illustrative):
- `event_id`, `folder_id`, `device_id`
- `op`: upsert | delete | rename
- `path` (relative to folder root), `size`, `content_hash`, `mtime`
- `hlc` (hybrid logical clock)
- server-assigned monotonic `seq` for pull cursors

The server stores **metadata only** — never file bytes. Peer introduction for
bytes remains Phase 5.

Reason
------
Snapshots explode bandwidth and hide history needed for offline catch-up.
Pure P2P metadata fails when only one device is online (the coordinator’s job
is availability of *coordination*, not storage of content). CRDTs are heavier
than needed for personal multi-device file trees with rare true concurrency.
An oplog + cursor matches Syncthing-ish / sync-engine practice and keeps the
“server never has your Movies” rule intact.
