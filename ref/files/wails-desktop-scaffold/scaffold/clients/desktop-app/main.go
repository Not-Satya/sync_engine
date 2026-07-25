//go:build ignore

package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

// The React app is built by Vite into frontend/dist, then embedded
// straight into this Go binary. No separate process, no localhost
// server for the UI — `go build` produces one native executable.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "Sync Engine Desktop",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.Startup,
		OnShutdown: app.Shutdown,
		// Every exported method on `app` becomes a callable JS function
		// in wailsjs/go/main/App the next time you run `wails dev` or
		// `wails build`. That generated file is what frontend/src/lib/
		// syncClient.js imports from.
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error starting app:", err.Error())
	}
}
