//go:build ignore

package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// watchEngineEvents is the piece that replaces polling. The engine
// already knows *when* something changed — a remote op got applied, a
// sync pass finished — because that's what core/resolver and
// core/cursor do internally. This just forwards that as a Wails
// runtime event so the frontend can react instead of asking "anything
// new?" on a timer.
//
// core/api.Engine exposes a Changes() channel for exactly this purpose;
// it's fed by the same code path whether the change came from a local
// write (CreateNote) or a remote one arriving over ws_listener.
func (a *App) watchEngineEvents() {
	go func() {
		for change := range a.engine.Changes() {
			runtime.EventsEmit(a.ctx, "sync:changed", change.EntityIDs)
		}
	}()
}
