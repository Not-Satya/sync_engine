## Decision 26
Phase: 5
Subphase: P5.1
Question
--------
What coordinator API exposes dialable peers for a folder, and what may it
return without becoming a content index or byte proxy?

Options
-------
A. Reuse `GET /v1/presence` only (account-wide; caller filters subscriptions)
B. `GET /v1/folders/{folderID}/peers` — online devices subscribed to that
   folder, with presence endpoint + public key material for handshake
C. `GET /v1/blobs/{hash}/providers` — server tracks which device has which hash
D. Push peer lists over WebSocket

Decision
--------
B for Phase 5.

- Route: `GET /v1/folders/{folderID}/peers`
- Authz: same as folder events — account owns folder and caller is subscribed
- Response rows: `device_id`, `name`, `platform`, `endpoint`, `public_key_hex`,
  `status`, `updated_at`
- Include only **online** (post-TTL expiry), **non-revoked** peers subscribed
  to the folder; **exclude the caller**
- `endpoint` may be empty (online but not yet listening) — fetch planner
  skips undialable peers
- Heartbeat `endpoint` must be `host:port` (IPv4/IPv6/hostname) when set;
  reject garbage so presence stays usable for dial

Explicitly **not** C: the coordinator does not learn or store which device
holds which content hash. Devices try online folder peers; a peer that
lacks the file returns an application-level "not found" on the P2P socket
(P5.2/P5.3). That keeps the server a notebook + switchboard, not a
replica map.

Reason
------
Account-wide presence (A) forces every client to re-join subscriptions and
leaks endpoints of devices not sharing the folder. A hash→provider index
(C) is useful later but is a new server-side content location database —
deferred. Folder-scoped online peers match ADR 21 with the least new
schema.
