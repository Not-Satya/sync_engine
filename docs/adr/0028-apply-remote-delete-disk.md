## Decision 28
Phase: 6
Subphase: P6.0 / P6.1
Question
--------
When a remote `MetaOpDelete` wins LWW and tombstones the local index, what
happens to the file bytes on this device’s disk?

Options
-------
A. Index-only: leave the file on disk (status quo after Phase 4–5)
B. Unlink the absolute path under the bound folder root when the tombstone
   is applied (best-effort; ignore already-absent)
C. Move to a device-local trash / `.sync-trash` for a retention window
D. Quarantine until the user confirms

Decision
--------
**B** for Phase 6.

After `ApplyRemote` accepts a delete tombstone for path `P` in folder `F`:
1. Resolve binding for `F`; if path health is not OK, skip disk (log).
2. `abs = Join(root, FromSlash(P))` must stay under the binding root
   (same containment rules as bind/scan).
3. If `abs` is a regular file, `os.Remove`. Missing file is success.
4. Directories: only remove if empty after children are handled, or skip
   non-empty dirs in v1 (files are the unit; empty-dir cleanup can be later).

Do **not** enqueue a new outbox delete when applying a remote tombstone —
the oplog already carries the authoritative delete.

Local sync deletes (scanner) already unlink via the user’s action; applying
the same tombstone again is idempotent.

Reason
------
Without B, Device B keeps ghost files forever after Device A deletes — metadata
says gone, disk still has bytes, and `missing=0` looks “healthy.” Trash (C)
and confirm (D) are better UX later; MVP needs correct shared-folder semantics.
Containment checks prevent a malicious/corrupt path from escaping the root.
