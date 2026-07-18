package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/Not-Satya/sync_engine/internal/coord/db"
)

// PresenceTTL is how long a device stays "online" without a heartbeat.
const PresenceTTL = 45 * time.Second

// Server is the Phase 1 coordination HTTP API.
// It exposes account/device/folder/subscription/presence only — never file bytes.
type Server struct {
	store       *db.Store
	presenceTTL time.Duration
}

func New(store *db.Store) *Server {
	return &Server{store: store, presenceTTL: PresenceTTL}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Post("/accounts", s.handleRegisterAccount)
		r.Post("/accounts/login", s.handleLogin) // links a new device to an existing account

		r.Group(func(r chi.Router) {
			r.Use(s.requireDevice)
			r.Get("/me", s.handleMe)
			r.Get("/devices", s.handleListDevices)

			r.Post("/folders", s.handleCreateFolder)
			r.Get("/folders", s.handleListFolders)

			r.Post("/folders/{folderID}/subscriptions", s.handleSubscribe)
			r.Delete("/folders/{folderID}/subscriptions", s.handleUnsubscribe)
			r.Get("/subscriptions", s.handleListSubscriptions)

			r.Post("/presence/heartbeat", s.handleHeartbeat)
			r.Get("/presence", s.handleListPresence)
		})
	})

	return r
}
