## Decision 13
Phase: 3
Subphase: P3.0 / P3.4
Question
--------
What rules apply when binding a local path to a FolderID?

Options
-------
A. Accept any string; fail later when watching
B. Must exist, must be a directory, must be absolute after resolution;
   reject `..` traversal; one path → at most one FolderID; one FolderID →
   at most one path on this device
C. Allow relative paths resolved against a configured sync root
D. Allow files as sync “folders” (single-file sync units)

Decision
--------
B for Phase 3.

- Resolve with `filepath.Abs` / `EvalSymlinks` where practical; store absolute path
- Require the path to exist and be a directory at bind time
- Reject paths that escape after cleaning (basic traversal defense)
- Uniqueness: no two bindings share the same path or the same folder_id
- Sync unit remains a folder (not a single file) — product Layer 1

Symlink policy: resolve to the real path when possible; if resolution fails,
reject the bind rather than store an ambiguous path.

Reason
------
Phase 4’s fsnotify watcher needs a real directory. Lazy validation produces
confusing runtime errors. Unique bindings prevent double-watching and
ambiguous “which folder is this path?”. Relative paths without a sync-root
product decision are footguns on Windows drive letters. Single-file sync
units contradict the folder-centric product model.
