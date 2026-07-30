## Decision 12
Phase: 3
Subphase: P3.0 / P3.1
Question
--------
Where is the mapping between a sync FolderID and a local filesystem path stored?

Options
-------
A. On the coordination server (path sent to server)
B. Only in the device keystore JSON (extend keystore file)
C. Separate device-local store next to the keystore (SQLite or JSON)
D. Implicit: folder name equals directory name under a fixed root

Decision
--------
C. Device-local bindings file/DB under the same app config dir
(e.g. `%AppData%/sync_engine/folder_bindings.json` or `bindings.db`).

The coordinator continues to store only FolderID, name, owner, and which
DeviceIDs are subscribed. It never stores absolute paths.

A binding row is approximately:
- `folder_id`
- `local_path` (absolute, normalized)
- `subscribed` (bool — mirrors intent; server subscription is source of truth for sync eligibility)
- `bound_at`

Reason
------
Absolute paths are device-specific and often private (`D:\Movies` vs
`/storage/emulated/0/Movies`). Putting them on the server violates the
“coordinator does not own device layout” rule and breaks multi-device
semantics. Extending the keystore mixes secrets with non-secret config.
A separate bindings store keeps keystore small and makes Phase 4’s watcher
able to enumerate paths without decrypting credentials.
