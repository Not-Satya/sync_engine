# Offline-First Sync Engine — Phase 1–4 Systems Briefing

**Product:** Offline-first, peer-to-peer folder sync. The server coordinates; devices own file bytes.

**Status at end of Phase 4:** Metadata sync loop complete. File bytes do not move between devices yet (Phase 5).

**Scale:** ~5,600 lines production Go, ~1,800 lines tests, 19 Architecture Decision Records (ADRs).

---

## How to use this document

Read in this order, out loud, without the code:

1. Product invariants (Section 1)
2. What you believed on day 1 vs now (Section 2)
3. The running architecture (Section 3)
4. ADRs in clusters, not 1→19 (Section 5)
5. Open the ADR file only when you cannot answer "why not the other option?"

If you start at `runtime.go`, you will memorize trees and miss the forest.

---

## 1. The requirements — Layer 1, frozen

These came from the original product brief before Phase 1. They are **product law**.

### What it is NOT

Not Dropbox, Google Drive, or "upload to the server and download later." If a design makes the server the home of file bytes, it is wrong for this project even if it is a fine cloud product.

### The split

| Path | Travels via | Contains |
|------|-------------|----------|
| Metadata | Coordinator | relative path, size, hash, mtime, HLC, delete/rename |
| Bytes | Device → device | the file itself |

### Core rules

- **Sync unit = folder.** Subscribe per device. Phone can skip Movies. Unsubscribe = stop receiving updates for that folder.
- **Delete ≠ remove local copy.** Delete is a tombstone for all subscribers. Remove local copy frees disk on one device; metadata stays; another device still has bytes (Phase 6).
- **Offline waits.** If the only device with bytes is asleep, transfer waits. v1 has no server-side spare copy.
- **Storage awareness.** Devices have different disks. Design must not assume every replica has every blob.

### How the project is built

Incremental phases. ADR before each fork. Three layers:

- **Layer 1 (Product):** folder semantics, delete vs remove-local-copy — decided, do not relitigate.
- **Layer 2 (System design):** polling vs WebSocket, metadata storage, package layout — ADR these.
- **Layer 3 (Implementation):** SQLite, HLC, goroutines — only when building that component.

**SDLC:** Iterative / incremental delivery with ADRs. Not waterfall.

**MVP cutoff:** End of Phase 5 (P2P byte transfer working).

---

## 2. What you thought at the start — and what the work taught you

| Day-1 picture | Picture after Phase 4 | Why your brain must change |
|---------------|----------------------|----------------------------|
| "Sync" means files flying | Phase 4 sync = replicas agree on the tree | You can be "in sync" on names/hashes without copying bytes |
| Server is a small REST API | Server is identity + presence + append-only metadata log | Coordination availability ≠ content availability |
| Need CRDTs for conflicts | One human, many devices → HLC + LWW | Conflict theory ≠ conflict product |
| fsnotify watches the folder | fsnotify is a hint; periodic scan is truth | OS APIs miss events under load |
| Auth is login | Actor is a device; password is bootstrap only | Lost phone ≠ reset the account |
| Paths live "with the folder" | FolderID is shared; absolute path never leaves device | D:\Movies is not a network fact |
| WebSockets when we sync metadata | ADR 4 planned WS; ADR 19 took it back for Phase 4 | Prove oplog + LWW first |
| Wails/reference tree is the architecture | internal/storage is a rejected ancestor | That design is cloud backup |
| One database | Four stores on purpose | Secrets, paths, index, coord log differ |
| Stack includes AES-GCM + bsdiff | First four pieces live; transfer crypto unused | Phase 5 questions, not Phase 1 trophies |

**Interview line:** "Dropbox's server is the replica. Ours is a notebook. That is the entire architecture."

---

## 3. Architecture as it exists (end of Phase 4)

### Two processes

```
deviceagent  ──HTTP metadata──►  coordserver  ◄──HTTP metadata──  deviceagent
     │                                                                    │
     └── file bytes on local disk ── (Phase 5: P2P) ── file bytes ───────┘
```

### Coordinator (`cmd/coordserver`)

Stores: users, devices, hashed tokens, pairing codes, folders, subscriptions, presence, folder_events.

- SQLite via modernc.org/sqlite
- Assigns monotonic `seq` on append
- Never stores absolute paths or file bytes

### Device (`cmd/deviceagent`)

| Store | Holds | Never holds |
|-------|-------|-------------|
| keystore.json | Ed25519 private key + bearer token (wrapped) | Folder paths |
| folder_bindings.json | FolderID ↔ absolute local path | Secrets |
| file_index.db | Per-path hash/size/HLC/tombstone + cursor + outbox | File bytes |

### Metadata pipeline (built)

1. Bound directory changes
2. fsnotify bursts → debounce (~400ms) → relative paths
3. Scanner stats + SHA-256. Unchanged hash = no event
4. Local HLC stamp → upsert/tombstone in index → enqueue outbox (event_id)
5. POST /v1/folders/{id}/events (idempotent on event_id)
6. Other device GET ...?since={cursor}
7. Clock.Observe so future local stamps stay causally ahead
8. ApplyRemote = LWW on (hlc_wall, hlc_counter, device_id)
9. Cursor advances to max seq even if LWW rejected the payload

### What each hop is allowed to forget

- **Watcher:** does not classify create/modify/delete — disk does
- **Server:** does not interpret LWW — dumb ordered log
- **Index:** may ignore older event and still advance cursor
- **Coordinator:** does not know C:\Users\...\Movies

### Presence

HTTP heartbeat every ~20s, TTL ~45s. Answers "who is online?" for future peer introduction. Not how metadata moves today.

### Locked stack vs deferred

| Piece | Status | Role |
|-------|--------|------|
| Go + chi | Locked | Shared types; coordination HTTP |
| modernc.org/sqlite | Locked | Coord DB and device index |
| fsnotify | Locked (P4) | Device watch + reconcile |
| argon2id + SHA-256 | Locked | Account bootstrap + bearer + file hash |
| Ed25519 DeviceID | Locked | Stable ID; future P2P auth |
| AES-256-GCM | Stacked, unused | P2P transfer encryption |
| go-bsdiff | Caution, unused | Deltas — not chunking |
| WebSocket | Deferred | Metadata fan-out after oplog proven |
| Postgres / S3 | Rejected v1 | Would turn server into storage |

---

## 4. Technologies — what, why, what we skipped

### Go

**Why:** One language for coordinator and agent; embed later (Wails, gomobile). Strong stdlib crypto and HTTP.

**Skipped:** Node/TS, Rust (slower iteration for this scope), Java (heavy for CLI agent).

### chi

**Why:** Thin router, idiomatic Go, middleware for bearer auth.

**Skipped:** gin (more magic), gRPC (overkill before stable event schema).

### modernc.org/sqlite

**Why:** Pure Go, one binary, same driver on server and device. Coordination data is small. Portable SQL if coord moves to Postgres later.

**Skipped:** Postgres day one (ops tax for personal MVP). Server-side object store.

### argon2id + SHA-256

**Why:** Password is rare. Tokens are high-entropy and hashed. File identity = "did bytes change?" — SHA-256 in stdlib.

**Skipped:** JWT as session (ADR 2), BLAKE3 (extra dep), size+mtime only (copy tools lie).

### Ed25519

**Why:** DeviceID = fingerprint of public key. Same key authenticates P2P in Phase 5.

**Skipped:** Server-assigned UUID, hardware IDs.

### DPAPI + AES-GCM keystore

**Why:** Agent must survive reboot. Plaintext token on disk is unacceptable. Windows DPAPI default; passphrase for shared PCs.

**Skipped:** OS keychain-only on all platforms in Phase 2, plaintext JSON.

### fsnotify

**Why:** Correct as a hint. Debounce collapses editor save storms. Periodic reconcile is the Syncthing pattern safety net.

**Skipped:** Scan-only, USN/FSEvents first, manual "Sync now."

### internal/storage (rejected ancestor)

StorageBackend with Put/Get/UploadSession — S3-shaped object store. Coordinator does not import it. Cloud-backup design deliberately not used.

---

## 5. The 19 ADRs — five clusters

Read ADR files in `docs/adr/` for full Option/Decision/Reason blocks.

### Paper A — "The server is not a warehouse" (ADR 1, 5, 6)

| ADR | Decision | Rejected |
|-----|----------|----------|
| 1 | SQLite coordination store | Postgres first |
| 5 | Confirm engine stack; constrain server | Server blob backend |
| 6 | cmd/coordserver + internal/coord | Wails core/engine + blobs tree |

### Paper B — "The actor is a device" (ADR 2, 3, 8, 9, 10, 11)

| ADR | Decision | Rejected |
|-----|----------|----------|
| 2 | Per-device opaque bearer token | JWT, mTLS, OIDC |
| 3 | Ed25519-derived DeviceID | UUID, MAC/Android ID |
| 8 | DPAPI wrap + passphrase fallback keystore | Plaintext JSON |
| 9 | Soft-revoke + wipe tokens | Hard delete |
| 10 | Pairing codes + password backup | Email OTP / QR first |
| 11 | cmd/deviceagent binary | Client inside coordserver |

### Paper C — "FolderID is global; path is local" (ADR 12, 13, 14)

| ADR | Decision | Rejected |
|-----|----------|----------|
| 12 | Local bindings file | Send paths to server |
| 13 | Abs dir, exists, unique, no .. | Lazy validate at watch |
| 14 | Explicit create/sub/bind + add sugar | Path implies subscription |

### Paper D — "Metadata is an oplog" (ADR 15, 16, 17, 7)

| ADR | Decision | Rejected |
|-----|----------|----------|
| 15 | Per-folder append-only event log | Snapshots, P2P-only meta, CRDT store |
| 16 | HLC + LWW per path | mtime-only, vector clocks, CRDT |
| 17 | SHA-256 whole file | size+mtime, BLAKE3, CDC in P4 |
| 7 | P4 locks clocks+transport; chunk deferred | Decide everything in Phase 1 |

**HLC in one breath:** Physical time plus a counter when time does not advance. Observe(remote) keeps local stamps causally ahead after applying remote events.

### Paper E — "Hints vs truth; transport vs correctness" (ADR 4, 18, 19)

| ADR | Decision | Rejected |
|-----|----------|----------|
| 4 | HTTP heartbeat + TTL; WS later | WS-only presence |
| 18 | fsnotify + debounce + reconcile | Scan-only, USN first, manual sync |
| 19 | HTTP push/pull + poll in Phase 4 | WebSocket on day one of P4 |

**Important evolution:** ADR 4 planned WebSocket when Phase 4 started. ADR 19 reversed that: prove oplog + cursor + LWW over HTTP first. WebSocket is optimization, not correctness requirement.

---

## 6. What we decided, then scratched or narrowed

1. **WebSocket in Phase 4** (ADR 4 → ADR 19) — biggest explicit scratch
2. **Reference architecture as skeleton** — blob store, Postgres, Wails UI declined
3. **Keys in memory** (ADR 3) → durable keystore (ADR 8); ID format unchanged
4. **ADR 7 deferral** → clocks and transport locked in P4; chunking still open
5. **Bindings SQLite or JSON** → JSON bindings, SQLite index
6. **MetaOpRename on wire** exists; watcher often emits delete+create
7. **Agent heartbeat** grew into RunLoop (P4.6), not desktop UI yet
8. **go-bsdiff** — stacked, unused; not a chunking strategy (ADR 5)

**What did NOT change:** server never stores file bytes; folder is the unit; delete ≠ remove local; devices own secrets.

---

## 7. Future decisions (Phase 5–7)

### Phase 5 — MVP bytes (must decide)

| Decision | Leading options | Trap |
|----------|-----------------|------|
| Chunking | Fixed-size, CDC/Rabin, whole-file + bsdiff | Betting on bsdiff alone |
| Peer introduction | Presence.endpoint + coord | Relaying bytes through server |
| NAT | LAN first; hole-punch/relay later | TURN before LAN works |
| Transfer crypto | AES-GCM under Ed25519 session | Encrypting coord JSON only |
| Resume | Chunk-level ack | Restart 4GB from byte 0 |

Defensible P5 v1: whole-file P2P first, then CDC if large files hurt.

### Phase 6 — Delete vs remove-local-copy

Tombstones exist. Need local policy: "I know this hash; I choose not to store the blob." Do not delete the index row.

### Phase 7 — Selective sync / placeholders

UX + policy on existing index. No second metadata system.

### Later (not MVP)

WebSocket fan-out. Postgres for coord if multi-writer. Real rename events. Conflict copies if LWW feels brutal. macOS Keychain / Android Keystore.

---

## 8. Explaining it two ways

### Five-year-old

Two backpacks. A teacher with a notebook, not a warehouse. The notebook says which drawing and how big, not the drawing. Kids only write new lines at the end. If two kids color the same page, whoever wrote later wins. If your friend is asleep, you wait — you do not give the drawing to the teacher to hold.

### Professional

Each subscribed folder has an append-only metadata oplog on the coordinator. Devices stamp mutations with an HLC, push idempotently, pull by seq cursor, and apply LWW. Content identity is SHA-256. Watchers are best-effort; reconcile is truth. Bytes are a separate, still-unbuilt path. The coordinator provides ordering and introduction, not content storage.

### The one sentence

**Devices keep the files. The server only keeps a notebook of what changed. When two edits collide, the later HLC wins. File bytes never enter that notebook.**

---

## 9. Study path

| Pass | Read | You should then say |
|------|------|---------------------|
| 1 | Product rules + PHASES.md | What this is not and where MVP ends |
| 2 | ADR 5, 1, 6, 11 | Why two binaries; why SQLite is not a cop-out |
| 3 | ADR 2, 3, 8, 9, 10 | Device is the actor; password is bootstrap |
| 4 | ADR 12, 13, 14 | FolderID shared; path local; subscribe per-device |
| 5 | ADR 15–19, 7, 4 | Oplog + HLC + hash + watch + why not WS yet |
| 6 | Trace hello.txt in code | What each hop is allowed to forget |

---

## 10. Phase completion summary

| Phase | Status | Summary |
|-------|--------|---------|
| 1 | Done | Coordination server: accounts, devices, tokens, folders, subscriptions, presence |
| 2 | Done | Auth hardening + deviceagent: revoke, rotate, pairing, keystore |
| 3 | Done | Device folder bindings: local path ↔ FolderID |
| 4 | Done | Metadata sync loop: watch, hash, outbox, push/pull, LWW apply |
| 5 | Planned | Peer-to-peer file byte transfer |
| 6 | Planned | Delete vs remove-local-copy |
| 7 | Post-MVP | Selective sync / placeholders |

---

*Generated from project ADRs and Phase 1–4 implementation. See docs/adr/ and docs/PHASES.md for authoritative decision records.*
