## Decision 6
Question
--------
How should packages be laid out so Layer 2 (system design) stays separable
from the product semantics and from deferred Layer 3 details?

Options
-------
A. Follow the reference doc (`server/` with blobs/, `core/` for engine)
B. Coordination-first layout: `cmd/coordserver`, `internal/coord/*`, `internal/ids`, `docs/adr`
C. Monolithic `internal/` with storage as the center

Decision
--------
B for Phase 1. Introduce:

```
cmd/coordserver/          # process entrypoint
internal/coord/
  db/                     # schema, migrations, store
  auth/                   # password + bearer token helpers
  api/                    # chi routes + handlers
  model/                  # domain types (no file bytes)
internal/ids/             # DeviceID / FolderID / UserID generation
docs/adr/                 # decisions
```

Device engine packages (`internal/engine`, watchers, P2P transfer) arrive in
later phases. `internal/storage` is not used by the coordinator.

Reason
------
Phase 1 only needs a coordination server. Cloning the reference tree would
drag in blob stores and oplog packages we are not building yet. A thin
`internal/coord` boundary makes the "server never sees file bytes" rule
structurally obvious. Shared ID helpers live outside coord so the eventual
device agent can reuse the same ID rules.
