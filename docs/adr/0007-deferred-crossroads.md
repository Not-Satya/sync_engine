## Decision 7 (deferred — partially resolved in Phase 4)
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
**Phase 4 locks:**
- Conflict / ordering → HLC + LWW (ADR 16)
- Metadata transport → HTTP push/pull + poll first (ADR 19); WebSocket later
- Content identity for metadata → SHA-256 whole file (ADR 17)

**Still deferred to Phase 5:**
- Chunking strategy (CDC vs fixed vs bsdiff)
- P2P byte transfer encryption details (AES-256-GCM already stacked)

Reason
------
Phase 4 needs clocks and a metadata wire path; it does not need chunking.
Keeping ADR 7 as the index avoids orphaning the original deferral note.
