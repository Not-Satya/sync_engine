## Decision 16
Phase: 4
Subphase: P4.0 / P4.5
Question
--------
How do we order concurrent metadata updates from different devices?

Options
-------
A. Wall-clock `mtime` only (trust OS clocks)
B. Hybrid Logical Clock (HLC) + last-write-wins (LWW) per file path
C. Vector clocks with explicit conflict files
D. Full CRDT (e.g. per-field or tree CRDT)

Decision
--------
B. HLC stamped on each local mutation; LWW by `(hlc, device_id)` tie-break
per relative path within a folder. Deletes are tombstones with an HLC so
late upserts do not resurrect casually.

Conflict UX in Phase 4: automatic LWW; optionally record a conflict log entry
for later UI. No “file (conflicted copy)” generation yet unless we hit a real
need in testing.

Reason
------
Wall clocks lie across laptops/phones. Vector clocks + manual merge are
correct but noisy for a single-user sync product. CRDTs overkill for “same
person, two devices.” HLC+LWW was the Phase 1 tentative lean (ADR 7) and fits
personal file sync. Tie-break on `device_id` makes ordering deterministic.
