## Decision 29
Phase: 6
Subphase: P6.0 / P6.2–P6.3
Question
--------
How does this device remember “I know content hash H for path P, but I choose
not to keep the blob locally” without lying to the coordinator?

Options
-------
A. Delete the index row (looks like never-synced; other devices’ upserts
   re-fetch forever)
B. Tombstone locally without pushing (diverges from oplog; next pull fights)
C. Device-local **`omit_local`** flag on the live index entry; metadata
   (hash/size/HLC) unchanged; no oplog event
D. Separate “exclusion list” file outside the index

Decision
--------
**C.** Add `omit_local INTEGER NOT NULL DEFAULT 0` on `file_entries`
(device-local only; never serialized into coordinator events).

Rules:
1. **Remove-local-copy:** require a live, non-deleted entry; set
   `omit_local=1`; unlink local file if present; **do not** enqueue
   `MetaOpDelete`.
2. **Scanner:** if the file is absent and `omit_local=1`, do **not**
   tombstone. If the user recreates the file on disk with matching or new
   bytes, clear `omit_local` and upsert as today (sync that change).
3. **Fetch planner:** skip candidates with `omit_local=1` (not “missing”).
4. **Remote upsert** that wins LWW for the same path: apply metadata; keep
   `omit_local` unless the new `content_hash` differs — then clear omit so
   the device can pull the new version (user opted out of *old* bytes, not
   forever).
5. **Remote delete** that wins: tombstone + unlink (ADR 28); clear omit.
6. **Materialize:** clear `omit_local` and run fetch for that path/hash.

Coordinator schema and event JSON stay unchanged — omit is not shared state.

Reason
------
A and B break the “server notebook is truth for membership” model. A separate
exclusion file (D) duplicates path keys and drifts from the index. A column on
the existing row keeps one source of truth for “what should exist in this
folder” vs “what this device stores.” Clearing omit on content_hash change
avoids permanently ignoring updates after a peer edits the file.
