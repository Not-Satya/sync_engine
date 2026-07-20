## Decision 11
Question
--------
Where does the Phase 2 device-side agent live in the repo, relative to the
coordinator?

Options
-------
A. Extend `cmd/coordserver` with client subcommands
B. New `cmd/deviceagent` + `internal/device/*` packages
C. Full `core/engine` tree from the Wails reference doc now
D. Put client code under `clients/desktop-app` (Wails) immediately

Decision
--------
B. Thin CLI agent first:

```
cmd/deviceagent/              # CLI: register, login, pair, status, heartbeat, logout
internal/device/
  keystore/                   # encrypted persist + load (ADR 8)
  client/                     # HTTP client for /v1 coordinator APIs
  agent/                      # long-running loop (heartbeat); no fsnotify yet
```

Coordinator stays under `cmd/coordserver` + `internal/coord/*`.
Shared ID helpers remain in `internal/ids`.
Wails / gomobile packaging stays deferred until the agent API is stable.

Reason
------
Phase 2 needs a real client process to exercise keystore, pairing, and revoke —
not a desktop UI. A separate `deviceagent` binary keeps the "server coordinates,
device owns secrets" boundary obvious. Folding client commands into
`coordserver` would blur that. Jumping to Wails or full `core/engine` now
front-loads Layer 3 and UI before auth hardening is done.
 