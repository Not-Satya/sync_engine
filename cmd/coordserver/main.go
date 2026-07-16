package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Not-Satya/sync_engine/internal/coord/api"
	"github.com/Not-Satya/sync_engine/internal/coord/db"
)

func main() {
	addr := flag.String("addr", ":8000", "HTTP listen address")
	dbPath := flag.String("db", filepath.Join("data", "coord.db"), "coordination sqlite path")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	store, err := db.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	srv := api.New(store)
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("coordination server listening on %s (db = %s)", *addr, *dbPath)
	log.Printf("schema: users, devices, auth_tokens, folders, subscriptions, presence - no file bytes")
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("listen: %v", err)
	}
}
