# Project phases

**Product:** Offline-first, peer-to-peer folder sync. The server coordinates; devices own file bytes.

**SDLC:** Incremental / iterative delivery with Architecture Decision Records (ADRs) before each major slice. Not waterfall; not “build everything then test.”

**MVP cutoff:** End of Phase 5 (P2P byte transfer working). Phase 6 completes the product story (delete vs remove-local-copy). Phase 7 is post-MVP.

| Phase | Status | Summary |
|---|---|---|
| **1** | Done | Coordination server: accounts, devices, tokens, folders, subscriptions, presence |
| **2** | Done | Auth hardening + deviceagent: revoke, rotate, pairing, keystore, register/login/pair/logout |
| **3** | Done | Device folder bindings: local path ↔ FolderID; subscribe from agent; **no fsnotify yet** |
| **4** | Done | Metadata sync loop (watch + sync names/hashes/versions via coordinator) |
| **5** | In progress | Peer-to-peer file byte transfer |
| **6** | Planned | Delete (all devices) vs remove-local-copy (this device) |
| **7** | Post-MVP | Selective sync / placeholders |

## Phase 2 subphases (done)

| ID | Slice |
|---|---|
| P2.0 | ADRs (keystore, revoke, pairing, agent layout) |
| P2.1 | Server soft-revoke |
| P2.2 | Richer device list |
| P2.3 | Token rotate |
| P2.4 | Pairing codes |
| P2.5 | Encrypted keystore |
| P2.6 | Agent status + heartbeat run |
| P2.7 | Register / login / pair CLI |
| P2.8 | Devices / revoke / logout |

## Phase 3 subphases

| ID | Slice |
|---|---|
| P3.0 | ADRs (binding model, path rules, local store) | Done |
| P3.1 | Local FolderID ↔ path binding store | Done |
| P3.2 | Coord HTTP client for folders + subscriptions | Done |
| P3.3 | Agent CLI: create / list / subscribe / bind | Done |
| P3.4 | Path validation + uniqueness | Done |
| P3.5 | `folders status` (bindings only; no watch) | Done |

## Phase 4 subphases

| ID | Slice | Status |
|---|---|---|
| P4.0 | ADRs (event log, HLC/LWW, hash, watch, HTTP transport) | Done |
| P4.1 | Server: metadata event schema + push/pull API | Done |
| P4.2 | Device: local file index (SQLite) for subscribed folders | Done |
| P4.3 | Device: fsnotify watcher + debounce | Done |
| P4.4 | Device: hash/scan + outbox | Done |
| P4.5 | Device: push/pull apply loop | Done |
| P4.6 | Wire into `deviceagent run` + status counts | Done |

## Phase 5 subphases

**Goal:** When metadata says a peer has content hash `H` and that peer is online, this device can pull the **file bytes** directly from that peer. The coordinator never sees those bytes — only introduces peers (presence + endpoint).

| ID | Slice | Status |
|---|---|---|
| P5.0 | ADRs (chunking, introduction, crypto, listen, layout) | Done |
| P5.1 | Advertise transfer endpoint via presence + peer discovery API | Done |
| P5.2 | Device transfer listener + mutual handshake (Ed25519) | Done |
| P5.3 | Whole-file pull by content hash over AES-256-GCM stream | Pending |
| P5.4 | Fetch planner: missing local blobs → pick online peer → pull | Pending |
| P5.5 | Wire into `deviceagent run` + status shows transfer activity | Pending |

**Explicitly out of Phase 5 v1:** NAT hole-punching / TURN byte relays, CDC/Rabin chunking, go-bsdiff as the primary path, server-side blob storage.

Decisions live in `docs/adr/` and are tagged with `Phase` / `Subphase` in each file.
