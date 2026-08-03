# Project phases

**Product:** Offline-first, peer-to-peer folder sync. The server coordinates; devices own file bytes.

**SDLC:** Incremental / iterative delivery with Architecture Decision Records (ADRs) before each major slice. Not waterfall; not “build everything then test.”

**MVP cutoff:** End of Phase 5 (P2P byte transfer working). Phase 6 completes the product story (delete vs remove-local-copy). Phase 7 is post-MVP.

| Phase | Status | Summary |
|---|---|---|
| **1** | Done | Coordination server: accounts, devices, tokens, folders, subscriptions, presence |
| **2** | Done | Auth hardening + deviceagent: revoke, rotate, pairing, keystore, register/login/pair/logout |
| **3** | In progress | Device folder bindings: local path ↔ FolderID; subscribe from agent; **no fsnotify yet** |
| **4** | Planned | Metadata sync loop (watch + sync names/hashes/versions via coordinator) |
| **5** | Planned | Peer-to-peer file byte transfer |
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
| P3.4 | Path validation + uniqueness | Pending |
| P3.5 | `folders status` (bindings only; no watch) | Pending |

Decisions live in `docs/adr/` and are tagged with `Phase` / `Subphase` in each file.
