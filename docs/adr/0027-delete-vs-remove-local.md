## Decision 27
Phase: 6
Subphase: P6.0
Question
--------
What is the product difference between “delete this file” and “free disk on
this device only,” and which action does an ordinary filesystem delete mean?

Options
-------
A. Filesystem delete = sync delete (tombstone) to all subscribed devices;
   remove-local-copy is a separate explicit agent command
B. Filesystem delete = remove-local-copy; sync delete requires an explicit
   agent command
C. Prompt / flag on every disappearance (ambiguous for headless `run`)
D. Collapse both into one operation (always tombstone)

Decision
--------
**A.**

| Action | Local disk | Index row | Metadata oplog | Other devices |
|--------|------------|-----------|----------------|---------------|
| **Sync delete** | Unlink when tombstone applies | Tombstone (`deleted=1`) with HLC | `MetaOpDelete` | Receive tombstone; unlink their copies |
| **Remove local copy** | Unlink on this device only | Stay **alive**; mark **omit_local** | *No* delete event | Unchanged; they keep bytes |

Ordinary watcher/scan disappearance (file gone under the bound root) is a
**sync delete**. That matches user expectation for “I deleted the movie” and
matches what the Phase 4 scanner already enqueues today.

Remove-local-copy is intentional disk reclaim: “I still want this file in the
folder’s shared metadata; I choose not to store the blob here.” Peers remain
the source of bytes (Phase 5). Materialize / fetch-again clears `omit_local`
and pulls.

Reason
------
Product rule from the systems briefing: **delete ≠ remove local copy.** Option
B would silently stop propagating deletes and surprise multi-device users.
Option C cannot run unattended. Option D makes selective disk use impossible
without leaving the subscription. Scanner already implements A for the sync
path; Phase 6 adds the omit path and finishes applying remote tombstones to
disk.
