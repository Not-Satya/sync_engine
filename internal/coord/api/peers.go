package api

import (
	"encoding/hex"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListFolderPeers(w http.ResponseWriter, r *http.Request) {
	caller := deviceFrom(r.Context())
	folderID := chi.URLParam(r, "folderID")
	if err := s.authorizeFolderAccess(r, caller, folderID); err != nil {
		writeFolderAccessErr(w, err)
		return
	}

	now := time.Now().UTC()
	_, _ = s.store.ExpireStalePresence(r.Context(), now.Add(-s.presenceTTL))

	peers, err := s.store.ListOnlineFolderPeers(r.Context(), folderID, caller.DeviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list peers failed")
		return
	}
	out := make([]map[string]any, 0, len(peers))
	for _, p := range peers {
		out = append(out, map[string]any{
			"device_id":       p.DeviceID,
			"name":            p.Name,
			"platform":        p.Platform,
			"endpoint":        p.Endpoint,
			"public_key_hex":  hex.EncodeToString(p.PublicKey),
			"status":          p.Status,
			"updated_at":      p.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"peers":       out,
		"ttl_seconds": int(s.presenceTTL.Seconds()),
	})
}
