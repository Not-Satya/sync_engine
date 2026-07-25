# Full-Fledged Sync Engine in Go — Architecture Guide
### (Laptop ⇄ Android, offline-first, multi-device, desktop via Wails)

---

## 1. Why Go works well for this

Go is a genuinely good fit for a sync engine, not just a "good enough" one:

- **Goroutines + channels** map naturally onto sync's real shape: background workers, debounced batchers, retry loops, fan-out listeners — all things you'd otherwise fight threads/callbacks for.
- **Cross-compiles natively** to Android via `gomobile bind` (produces a `.aar` with auto-generated Kotlin/Java bindings) and to desktop via **Wails**, which wraps a Go binary with the OS's native webview (WebKit2GTK on Linux, WebView2 on Windows, WKWebView on macOS) — no Electron, no bundled Chromium, no separate runtime to ship.
- **Precedent**: Syncthing's entire sync core is Go, and `syncthing-android` embeds that *exact same* Go engine via `gomobile`. This is not a hypothetical pattern — it's shipped and battle-tested at scale.
- Good SQLite options exist in pure Go (`modernc.org/sqlite`), which matters because pure-Go avoids needing the Android NDK C toolchain when cross-compiling — cgo-based drivers (`mattn/go-sqlite3`) work too but add NDK setup pain for Android builds.

The core ideas (oplog, HLC clocks, conflict resolution, tombstones, cursors) don't change based on language — only the packaging changes. Quick recap since it matters for the folder structure below:

- **Oplog, not snapshots**: sync operations (`{op_id, entity_id, field, value, hlc, device_id}`), not "current state," so devices can replay and merge deterministically.
- **HLC (Hybrid Logical Clock)**: orders events across devices without trusting wall-clock time.
- **LWW (Last-Write-Wins) resolver**: simplest correct strategy for personal data (notes/tasks/files) where the same person edits from two devices, rarely truly concurrently. CRDTs (via a library like Yjs-equivalent, if you ever need real-time collaborative text) are a heavier upgrade path, not a starting point.
- **Tombstones**: deletes are soft-deletes with a `deleted_at`, kept for N days, so late-arriving devices don't resurrect deleted data.
- **Cursors**: each device remembers "last event id I've applied," so pulls are incremental (`since=<cursor>`), not full re-downloads.

**One more thing Wails buys you specifically**: because the desktop UI shell and the sync engine are both Go, in the same process, the desktop side of this project has *one fewer integration pattern to build and maintain* than an Electron/Tauri setup would. Section 3 spells out exactly what that removes.

---

## 2. High-level architecture

```
┌───────────────────────────────┐              ┌──────────────────────┐
│  LAPTOP — Wails app            │              │  ANDROID              │
│  (single OS process)           │              │                       │
│                                 │              │  Kotlin/Compose UI    │
│  React UI (native webview)     │              │    │                  │
│    │  bound Go methods ────┐   │              │  Go Sync Engine       │
│    │  runtime events ◄─────┤   │              │  (.aar via gomobile,  │
│    ▼                       │   │              │   embedded in-proc)   │
│  Go Sync Engine (embedded, │   │              │    │                  │
│   same process, no FFI) ───┘   │              │  SQLite (local DB)    │
│    │                            │             └───────────┬───────────┘
│  SQLite (local DB)              │                         │
└──────────────┬──────────────────┘                         │
               │                                             │
               └───────────────────┐         ┌───────────────┘
                                    ▼         ▼
                          ┌────────────────────────┐
                          │   SYNC SERVER (Go)      │
                          │                         │
                          │  - Auth / devices       │
                          │  - Append-only events   │
                          │  - Per-device cursors   │
                          │  - Fan-out (WS / FCM)   │
                          │  - Blob storage         │
                          └─────────────────────────┘
```

The server is written in Go too — meaning you can literally share Go structs/protocol code between client and server via a shared internal package. That's one of the biggest practical wins of picking one language end to end.

---

## 3. How the two platforms actually embed the same Go code

### Android — `gomobile bind` (unchanged)
```bash
gomobile bind -target=android -o synccore.aar ./core
```
This inspects your exported Go package and auto-generates a Kotlin-callable `.aar`. Constraints to design around:
- Exported functions can only use gomobile-supported types: basic types, strings, byte slices, and exported structs/interfaces — no generics, no raw channels, no arbitrary interfaces across the boundary.
- Long-running background work (the sync loop) should be started from Kotlin via `WorkManager` calling into a Go `StartSync()` / `StopSync()` pair, not left running unmanaged — Android will kill it otherwise.
- The Go engine runs **in-process** inside the Android app (not a separate process), so it shares the app's lifecycle.

### Desktop — Wails, direct embed (this is the committed path now)

With Wails, `clients/desktop-app` imports `core/engine` as an ordinary Go package. There's no HTTP API, no localhost daemon, no "is the sidecar still alive" health check — that whole class of problem doesn't exist here. Concretely:

- **One binary.** `go:embed` bakes the built React app (a static Vite bundle) directly into the Go executable. `wails build` produces a single native file per OS — no bundled Chromium, no `node` runtime shipped alongside it.
- **Go → JS calls, auto-generated.** You write a plain Go struct (`App`, in `app.go`) and pass it to `wails.Run(&options.App{Bind: []interface{}{app}})`. Wails inspects its exported methods and generates a matching JS function for each one under `frontend/wailsjs/go/main/App.js`. Calling `CreateNote("buy milk")` from React is a same-process call into your Go method — Wails marshals the Go return struct to JSON automatically, so it lands in JS as a plain object.
- **JS → Go events, no polling.** The reverse direction (server pushed you a change, update the UI) goes through `runtime.EventsEmit(ctx, "sync:changed", ids)` on the Go side and `runtime.EventsOn('sync:changed', handler)` on the React side. This replaces the "poll a `/status` endpoint" pattern you'd need with a sidecar.
- **`core/api/public.go` is still the only door.** Both integration paths — gomobile's generated bindings and `app.go`'s direct calls — should only ever call through this file, never into `engine`, `oplog`, etc. directly. That's what keeps the Android and desktop integrations honest with each other as the internals evolve.

*(The previous version of this doc also described a "sidecar" pattern — a small local HTTP daemon — for the case where the desktop UI wasn't Go, e.g. Electron or Tauri. That's not needed here: it existed specifically to avoid FFI when the UI is a separate process in a separate language. Wails doesn't have that problem, so `sidecar/` is gone from the folder structure below.)*

### What actually happens, end to end, with this setup

Walking through one real interaction, because this is the part that's easy to leave vague:

1. **You type a note in React.** `NotesList.jsx`'s submit handler calls `onAddNote(text)`, which calls `syncClient.createNote(text)` — a thin wrapper around the Wails-generated `CreateNote` binding.
2. **That call lands in `App.CreateNote` in `app.go`**, which calls `core/api.Engine.ApplyLocalChange(...)`. This writes the note to SQLite *and* appends the mutation to the `outbox` table, in the same transaction.
3. **The function returns synchronously** with the created `Note` struct, Wails marshals it to JSON, and React updates state immediately. No network round trip — the UI never waits on the server.
4. **Meanwhile, in the background**, the `transport.Batcher` goroutine (already running since `Startup`) picks up that outbox row on its next debounce tick and pushes it to the sync server.
5. **On your phone**, the server's fan-out (WebSocket or FCM wake) delivers the event; Android's `ws_listener` hands it to `resolver.LWW`, which applies it to Android's own SQLite and advances its cursor.
6. **Back on the laptop**, when *your phone's* changes come back the same way, `core/engine` applies them and pushes onto its internal `Changes()` channel.
7. **`app_events.go` is listening on that channel** and calls `runtime.EventsEmit(ctx, "sync:changed", entityIDs)`.
8. **React's `useSyncEvents` hook** (subscribed via `runtime.EventsOn` in a `useEffect`) fires, and the component re-fetches the changed notes from Go. No polling loop anywhere in this path — everything downstream of step 4 is event-driven.

Steps 2 and 8 are the two places that are genuinely different from a sidecar/Electron setup: step 2 is an in-process function call instead of an HTTP POST, and step 8 is a native event subscription instead of a WebSocket/SSE connection the frontend has to manage itself.

---

## 4. Folder / file structure

```
sync-engine/
├── go.mod
├── go.work                          # workspace file tying client/server modules together
├── README.md
│
├── core/                            # the sync engine itself — platform-agnostic Go package
│   ├── go.mod
│   ├── db/
│   │   ├── schema.go                 # table definitions (entities, oplog, cursors, tombstones)
│   │   ├── migrations/               # versioned .sql files, run via golang-migrate or embed+manual
│   │   └── conn.go                   # modernc.org/sqlite connection setup, WAL mode config
│   │
│   ├── oplog/
│   │   ├── operation.go              # Operation struct: OpID, EntityID, Field, Value, HLC, DeviceID
│   │   ├── outbox.go                 # local mutations pending push
│   │   ├── inbox.go                  # remote mutations pending apply
│   │   └── tombstone.go              # delete tracking + expiry sweep
│   │
│   ├── clock/
│   │   └── hlc.go                    # Hybrid Logical Clock: Now(), Compare(), Merge()
│   │
│   ├── resolver/
│   │   ├── lww.go                    # last-write-wins per-field resolver
│   │   ├── crdt.go                   # optional pluggable CRDT resolver for rich text later
│   │   └── conflictlog.go            # records conflicts for user-visible surfacing
│   │
│   ├── transport/
│   │   ├── client.go                 # push/pull calls to sync server (HTTP/gRPC)
│   │   ├── batcher.go                # debounce + batch outgoing ops (uses time.Timer/channels)
│   │   ├── retry.go                  # exponential backoff, jitter, offline queue
│   │   └── ws_listener.go            # realtime push channel (nhooyr.io/websocket)
│   │
│   ├── blobs/
│   │   ├── chunker.go                # splits large files into content-addressed chunks
│   │   ├── hasher.go                 # BLAKE3/SHA-256 chunk hashing
│   │   └── blobstore.go              # local blob cache + upload/download queue
│   │
│   ├── crypto/
│   │   ├── e2ee.go                   # payload encryption (golang.org/x/crypto/nacl or age)
│   │   └── keys.go                   # per-device keypair management
│   │
│   ├── cursor/
│   │   └── cursor.go                 # per-device "last applied event" checkpoint
│   │
│   ├── engine/
│   │   └── engine.go                 # orchestrator: wires db+oplog+transport+resolver together;
│   │                                 #   exposes Start(), Stop(), ApplyLocalChange(), Status(), Changes()
│   │
│   └── api/
│       └── public.go                 # the ONLY file gomobile bindings and app.go should import from —
│                                      #   keep this a small, stable, intentional surface
│
├── mobile/
│   └── androidbind/
│       └── build.sh                  # runs `gomobile bind` against core/api, outputs .aar
│
├── server/                           # sync relay/coordinator
│   ├── main.go
│   ├── auth/
│   │   └── device_auth.go            # device registration + token issuance
│   ├── events/
│   │   ├── store.go                  # append-only event store (Postgres)
│   │   └── fanout.go                 # pushes new events to other online devices (WS / FCM)
│   ├── blobs/
│   │   └── objectstore.go            # S3-compatible storage for file chunks
│   ├── api/
│   │   ├── push.go                   # POST /events
│   │   ├── pull.go                   # GET  /events?since=cursor
│   │   └── ws.go                     # realtime fan-out channel
│   └── db/
│       └── migrations/               # Postgres schema: events, devices, users, blobs
│
├── clients/
│   ├── desktop-app/                  # the Wails app — this IS the desktop app, no separate daemon
│   │   ├── wails.json                # Wails project config
│   │   ├── go.mod
│   │   ├── main.go                   # wails.Run(...) — window, embedded frontend, binds App
│   │   ├── app.go                    # App struct: wraps core/api, exposes bound methods
│   │   │                             #   (CreateNote, ListNotes, TriggerSync, GetStatus, ...)
│   │   ├── app_events.go             # forwards core/engine's Changes() to runtime.EventsEmit
│   │   └── frontend/                 # React app, built by Vite, embedded into the Go binary
│   │       ├── package.json
│   │       ├── src/
│   │       │   ├── main.jsx
│   │       │   ├── App.jsx
│   │       │   ├── hooks/
│   │       │   │   └── useSyncEvents.js  # runtime.EventsOn wrapper
│   │       │   ├── lib/
│   │       │   │   └── syncClient.js     # thin wrapper around wailsjs/go bindings
│   │       │   └── components/
│   │       │       └── NotesList.jsx     # example UI consuming synced data
│   │       └── wailsjs/                  # AUTO-GENERATED by `wails dev` — do not hand-edit
│   │           ├── go/main/App.js
│   │           └── runtime/runtime.js
│   │
│   └── android-app/                  # Kotlin/Jetpack Compose app
│       ├── app/libs/synccore.aar     # the gomobile-built engine, dropped in as a library
│       └── app/src/main/java/.../
│           ├── sync/
│           │   ├── SyncBridge.kt      # thin Kotlin wrapper calling into synccore.aar
│           │   ├── SyncWorker.kt      # WorkManager job — periodic + on-network-change triggers
│           │   └── FcmListener.kt     # wakes sync on push notification
│           └── ui/                    # screens
│
└── shared/
    ├── protocol/
    │   └── event.go                  # wire format shared by client + server (single source of truth)
    └── docs/
        ├── conflict-resolution.md
        └── sync-protocol-spec.md
```

Notice there's no `sidecar/` directory anymore. That entire integration layer — HTTP routes, JSON marshaling over loopback, a health-check endpoint — existed only to solve the "UI is a different process/language than the engine" problem. Wails doesn't have that problem, so it doesn't need the workaround.

### Why `core/api/public.go` still matters

Both integration paths (gomobile's generated Kotlin bindings, and `app.go`'s direct Go calls) should only ever call through this one small, deliberate file — not reach into `engine`, `oplog`, etc. directly. This keeps your Android bindings and your Wails `app.go` in sync with each other by construction, and means you can change internals freely without breaking either platform's integration.

---

## 5. Recommended Go libraries

| Need | Library |
|---|---|
| Desktop app framework (Go backend + native webview + React frontend) | `github.com/wailsapp/wails/v2` |
| SQLite (pure Go, no cgo — easiest cross-compile to Android) | `modernc.org/sqlite` |
| SQLite (cgo, faster, needs NDK for Android builds) | `mattn/go-sqlite3` |
| WebSocket | `nhooyr.io/websocket` (simpler, context-based) or `gorilla/websocket` |
| UUIDs | `google/uuid` |
| Migrations | `golang-migrate/migrate` |
| Encryption | `golang.org/x/crypto/nacl/box` or `filippo.io/age` |
| gRPC (if you want typed client↔server instead of raw HTTP) | `google.golang.org/grpc` + protobuf |
| Push notifications to wake Android sync | Firebase Cloud Messaging (via server-side Go SDK) |

---

## 6. Data flow, end to end

**Write path:**
1. React calls a bound Go method directly (e.g. `CreateNote(text)` via the generated `wailsjs/go/main/App` binding) — an in-process function call, not a serialized network/IPC hop the way an Electron+sidecar setup would need.
2. `App.CreateNote` calls `core/api.ApplyLocalChange`, which writes to local SQLite *and* appends the mutation to the `outbox` table in the same transaction.
3. A background goroutine (`transport.Batcher`) wakes on a debounce timer (e.g. every 500ms) or on network-reconnect signal, batches pending outbox rows, and calls `transport.Client.Push()`.
4. Server assigns a global sequence number, stores the event, and fans it out over WebSocket (or FCM push, to wake a backgrounded Android app) to the user's other devices.
5. Receiving device's `ws_listener` goroutine gets the event, hands it to `resolver.LWW`, applies it to local SQLite, updates its `cursor`. On the Wails app specifically, `core/engine` then pushes onto its `Changes()` channel, `app_events.go` forwards that via `runtime.EventsEmit`, and React's `useSyncEvents` hook re-fetches — no polling.

**Catch-up path (device reconnects after being offline):**
1. `transport.Client.Pull(since: cursor)` — one HTTP call.
2. Server streams everything after that cursor.
3. Applied in order through the same resolver path as realtime events — one code path for both cases, which is a nice property Go's straightforward control flow makes easy to keep clean. On desktop this still ends in the same `Changes()` → `EventsEmit` → `useSyncEvents` path as a single realtime update, so the UI doesn't need a separate "bulk catch-up" rendering mode.

---

## 7. Realistic build order

1. **SQLite schema + migrations** in `core/db` — get this right first, both platforms depend on it.
2. **HLC clock** (`core/clock`) — small, easy to unit test in isolation, foundational to everything else.
3. **Oplog + outbox/inbox** — prove you can write a mutation, log it, and replay it back into the same state.
4. **LWW resolver** — simplest conflict strategy; ship this before touching CRDTs.
5. **`core/api/public.go`** — define the tiny stable surface early, even before transport exists, so both integration paths can start being built against it.
6. **Wails app shell (desktop)** — wire `app.go` to call directly into `core/engine`, bind it in `main.go`. Easiest platform to iterate on: `wails dev` hot-reloads both the Go and React sides on save, and there's no separate process to keep alive.
7. **gomobile bind (Android)** — once the API surface is stable, generate the `.aar` and wire `SyncWorker.kt` to it.
8. **Server: event store + cursor endpoint** — a Postgres table (`events(id, user_id, device_id, payload, seq)`) plus `/push` and `/pull` is enough to start.
9. **Realtime fan-out** (WebSocket + FCM for Android wake-ups) — upgrade from polling once the basic loop is proven.
10. **Blob sync** — separate pipeline, only once structured sync is solid.
11. **E2E encryption** — retrofit onto a stable wire format; much easier than encrypting a protocol still in flux.

---

## 8. Worth studying directly

- **Syncthing** (`github.com/syncthing/syncthing`) — the single best reference: full Go sync core, and `syncthing-android` shows exactly how `gomobile` embeds it in a real shipped Android app.
- **Wails' own guides** (`wails.io/docs/guides`) — specifically the "Bind" and application lifecycle (`OnStartup`/`OnShutdown`) sections; that's the exact pattern `app.go` uses here.
- **rqlite** — Go + SQLite + Raft consensus; good reading for how Go people structure a replicated SQLite system, even though your consistency model will be simpler (single-user, multi-device, not multi-writer consensus).
- **PowerSync / ElectricSQL** — not Go, but their public architecture docs are excellent for the oplog/cursor/conflict-resolution design regardless of implementation language.

---

## What's attached

Real starter files for `clients/desktop-app/` — `main.go`, `app.go`, `app_events.go`, `wails.json`, and the React side (`App.jsx`, `NotesList.jsx`, `syncClient.js`, `useSyncEvents.js`) — following the structure above, plus a README on what's hand-written vs. what Wails auto-generates and how to actually run it.
