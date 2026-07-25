# Wails desktop-app scaffold

This is the `clients/desktop-app/` piece from the architecture doc, fleshed
out into real files. It's meant to be dropped into the `sync-engine/`
repo layout described there — it assumes `core/api/public.go` already
exists and exports roughly the shape used below (`Engine`, `NewEngine`,
`Config`, `Mutation`, `Note`, `Status`, and a `Changes()` channel).

## What's hand-written vs generated

| File | Status |
|---|---|
| `main.go` | hand-written — you edit this |
| `app.go` | hand-written — you edit this, it's where bound methods live |
| `app_events.go` | hand-written — bridges engine changes to frontend events |
| `wails.json` | hand-written — project config |
| `frontend/src/**` | hand-written — your actual React app |
| `frontend/wailsjs/**` | **not included here** — Wails generates this the first time you run `wails dev`, by inspecting the exported methods on the struct passed to `Bind` in `main.go`. Don't hand-write it and don't commit fights over it; regenerate instead of merging. |
| `frontend/dist/**` | build output, gitignore it |

## Getting it running

```bash
# one-time
go install github.com/wailsapp/wails/v2/cmd/wails@latest

cd clients/desktop-app
wails dev        # generates wailsjs/, starts Go + Vite with hot reload
```

`wails dev` is the thing to reach for while building — it rebuilds the Go
side and hot-reloads the React side on save, and it's what generates
`frontend/wailsjs/go/main/App.js` from your `app.go` exports. Run it once
before your editor complains about `App.jsx`'s imports not resolving.

```bash
wails build       # produces a single native binary with the frontend embedded
```

## The one thing to keep straight

`app.go` should stay a thin translation layer — Wails plumbing in, calls
into `core/api` out. Business logic (conflict resolution, batching,
retry) lives in `core/`, same as the Android side. If you find yourself
writing sync logic inside `app.go`, it's in the wrong file.
