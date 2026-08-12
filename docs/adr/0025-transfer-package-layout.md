## Decision 25
Phase: 5
Subphase: P5.0
Question
--------
Where does Phase 5 P2P transfer code live relative to the existing agent?

Options
-------
A. Stuff TCP + crypto into `internal/device/agent` and `internal/coord/api`
B. New packages under `internal/device/transfer/` (listen, protocol, fetch)
   plus thin coord APIs for peer listing; keep `internal/storage` unused
C. Adopt the reference `core/engine` + server blob tree now
D. Separate `cmd/xferd` binary

Decision
--------
B:

```
internal/device/transfer/
  listen.go      # TCP accept loop
  handshake.go   # Ed25519 mutual auth
  protocol.go    # request hash / stream frames
  fetch.go       # dial peer, pull, verify, place file
  planner.go     # missing blobs → peer pick → fetch (P5.4)
```

Coordinator gains only discovery helpers (e.g. list online folder peers
with endpoints) — still no Put/Get of file bytes. `internal/storage`
remains unused. One `deviceagent` process still runs heartbeat + watch +
metadata sync + transfer listen/fetch (wired in P5.5).

Reason
------
Keeps the "bytes never touch the server" boundary obvious in the package
graph. A second binary (D) splits keystore/bindings for little gain.
Absorbing everything into `agent` (A) repeats the Phase 2 mistake of a
god package. The reference blob tree (C) is the design we rejected in
ADR 5/6.
