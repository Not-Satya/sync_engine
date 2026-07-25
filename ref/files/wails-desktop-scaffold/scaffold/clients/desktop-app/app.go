//go:build ignore

package main

import (
	"context"

	// This is core/api/public.go from the main sync-engine module — the
	// same small surface that the Android gomobile bindings call into.
	// Swap this import path for wherever your go.work / go.mod actually
	// resolves it.
	syncapi "sync-engine/core/api"
)

// App is the struct Wails binds to the frontend. Nothing outside this
// file should reach into core/engine, core/oplog, etc. directly — that's
// the whole point of keeping core/api/public.go small and stable.
type App struct {
	ctx    context.Context
	engine *syncapi.Engine
}

func NewApp() *App {
	return &App{}
}

// Startup runs once Wails has created the native window and the
// frontend is ready to receive events. This is where the sync engine
// actually starts — not in main(), so we have a valid ctx to emit
// events on (see app_events.go).
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	engine, err := syncapi.NewEngine(syncapi.Config{
		// Real code: resolve this to the OS app-data dir, e.g. via
		// os.UserConfigDir(), not a relative path.
		DBPath: "sync.db",
	})
	if err != nil {
		// Real code: surface this to the UI (emit a "sync:fatal" event)
		// instead of panicking a desktop app.
		panic(err)
	}
	a.engine = engine

	if err := a.engine.Start(); err != nil {
		panic(err)
	}

	a.watchEngineEvents()
}

// Shutdown is called by Wails when the window is closing — stop the
// background goroutines cleanly instead of letting the process die
// mid-write.
func (a *App) Shutdown(ctx context.Context) {
	if a.engine != nil {
		a.engine.Stop()
	}
}

// ---------------------------------------------------------------------
// Bound methods below this line are callable from React as plain async
// functions, e.g. `await CreateNote("buy milk")`. Wails marshals Go
// structs to JSON automatically, so the return types just work on the
// JS side as regular objects.
// ---------------------------------------------------------------------

// CreateNote writes locally first (SQLite + outbox, same transaction)
// and returns immediately — the network push happens later, on
// core/transport's own debounce timer. The UI never waits on it.
func (a *App) CreateNote(text string) (syncapi.Note, error) {
	return a.engine.ApplyLocalChange(syncapi.Mutation{
		Entity: "note",
		Field:  "text",
		Value:  text,
	})
}

// ListNotes always reads local SQLite state — it's correct whether
// you're online, offline, or mid-sync.
func (a *App) ListNotes() ([]syncapi.Note, error) {
	return a.engine.ListEntities("note")
}

// TriggerSync forces an immediate push/pull instead of waiting for the
// debounce timer. Wire this to a manual "sync now" button.
func (a *App) TriggerSync() error {
	return a.engine.SyncNow(a.ctx)
}

// GetStatus backs a small "synced / syncing / offline" indicator.
func (a *App) GetStatus() syncapi.Status {
	return a.engine.Status()
}
