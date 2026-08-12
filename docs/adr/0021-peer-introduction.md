## Decision 21
Phase: 5
Subphase: P5.0 / P5.1
Question
--------
How does a device learn *where* to fetch file bytes for a missing hash
without the coordinator storing or proxying those bytes?

Options
-------
A. Coordinator stores blobs and serves downloads (rejected by product law)
B. Coordinator relays byte streams between devices (TURN-like)
C. Devices advertise a transfer `endpoint` on presence heartbeat; peers
   discover online subscribers for a folder and dial them directly
D. mDNS / LAN gossip only (no coordinator involvement)

Decision
--------
C. Reuse presence:

1. `deviceagent run` listens for transfers and includes
   `endpoint` (e.g. `host:port`) on each heartbeat.
2. Coordinator stores that endpoint with presence (already in schema) —
   never file bytes.
3. A device missing hash `H` asks the coordinator for **online peers**
   subscribed to the same folder (and optionally filters by who likely
   has the blob via local index later).
4. Device dials peer endpoint and requests the file by content hash.

No byte relay through the server (B). No server object store (A).
mDNS (D) may supplement later for same-LAN discovery but is not the v1
path — account-scoped introduction via the coordinator matches multi-
network phones/laptops.

Reason
------
The coordinator's job is introduction: "Laptop is online at X and shares
folder Movies." Relaying bytes would recreate Dropbox under another name
and blow the "server never sees file bytes" rule. Presence already has an
`endpoint` field unused for P2P — Phase 5 fills it.
