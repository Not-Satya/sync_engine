## Decision 14
Phase: 3
Subphase: P3.0 / P3.3
Question
--------
How should create-folder, subscribe, and bind-path relate in the agent UX?

Options
-------
A. Three separate commands; user wires them manually
B. `folder add -name Movies -path D:\Movies` creates on server (if needed),
   subscribes this device, and binds the path in one step
C. Auto-create a folder on the server whenever a path is watched
D. Bindings only; subscriptions implied by having a local path

Decision
--------
A + convenience B.

- Explicit primitives: `folders create`, `folders subscribe`, `folders bind`,
  `folders unbind`, `folders list`
- Convenience: `folders add -name X -path P` = create (or reuse by name later)
  + subscribe + bind, for the common first-device case
- Unsubscribe without unbind (and vice versa) remains possible so Phase 6/7
  can support “still remember path but stop syncing” / “remove local copy”
  without deleting the FolderID on the account

Server subscription remains authoritative for “will this device receive
metadata updates?” Local binding answers “where on disk?”

Reason
------
Students and interviewers need a clear mental model: **FolderID is shared;
path is local; subscription is per-device.** A single magic command is fine
as sugar, but collapsing concepts into “path implies subscription” makes
delete vs remove-local-copy harder later. Phase 3 does not watch files yet —
binding only records intent for Phase 4.
