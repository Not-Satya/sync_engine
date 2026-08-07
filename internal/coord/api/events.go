package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Not-Satya/sync_engine/internal/coord/db"
	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

type pushEventsRequest struct {
	Events []pushEvent `json:"events"`
}

type pushEvent struct {
	EventID     string     `json:"event_id"`
	Op          string     `json:"op"`
	Path        string     `json:"path"`
	OldPath     string     `json:"old_path,omitempty"`
	Size        int64      `json:"size"`
	ContentHash string     `json:"content_hash"`
	ModTime     *time.Time `json:"mtime,omitempty"`
	HLC         model.HLC  `json:"hlc"`
}

func (s *Server) handlePushFolderEvents(w http.ResponseWriter, r *http.Request) {
	caller := deviceFrom(r.Context())
	folderID := chi.URLParam(r, "folderID")
	if err := s.authorizeFolderAccess(r, caller, folderID); err != nil {
		writeFolderAccessErr(w, err)
		return
	}

	var req pushEventsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Events) == 0 {
		writeErr(w, http.StatusBadRequest, "events required")
		return
	}
	if len(req.Events) > 500 {
		writeErr(w, http.StatusBadRequest, "too many events (max 500)")
		return
	}

	batch := make([]model.FolderEvent, 0, len(req.Events))
	for _, e := range req.Events {
		path := strings.TrimSpace(strings.ReplaceAll(e.Path, "\\", "/"))
		if e.EventID == "" || path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
			writeErr(w, http.StatusBadRequest, "each event needs event_id and a relative path without '..'")
			return
		}
		op := model.MetaOp(e.Op)
		ev := model.FolderEvent{
			EventID:     e.EventID,
			FolderID:    folderID,
			DeviceID:    caller.DeviceID,
			Op:          op,
			Path:        path,
			OldPath:     strings.TrimSpace(strings.ReplaceAll(e.OldPath, "\\", "/")),
			Size:        e.Size,
			ContentHash: e.ContentHash,
			HLC:         e.HLC,
		}
		if e.ModTime != nil {
			ev.ModTime = e.ModTime.UTC()
		}
		batch = append(batch, ev)
	}

	accepted, err := s.store.AppendFolderEvents(r.Context(), folderID, batch)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	maxSeq, _ := s.store.MaxFolderEventSeq(r.Context(), folderID)
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": accepted,
		"max_seq":  maxSeq,
	})
}

func (s *Server) handlePullFolderEvents(w http.ResponseWriter, r *http.Request) {
	caller := deviceFrom(r.Context())
	folderID := chi.URLParam(r, "folderID")
	if err := s.authorizeFolderAccess(r, caller, folderID); err != nil {
		writeFolderAccessErr(w, err)
		return
	}

	since := int64(0)
	if v := r.URL.Query().Get("since"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, "since must be a non-negative integer")
			return
		}
		since = n
	}
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}

	events, err := s.store.ListFolderEventsSince(r.Context(), folderID, since, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list events failed")
		return
	}
	maxSeq, _ := s.store.MaxFolderEventSeq(r.Context(), folderID)
	hasMore := false
	if len(events) == limit {
		hasMore = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"max_seq":  maxSeq,
		"has_more": hasMore,
	})
}

func (s *Server) authorizeFolderAccess(r *http.Request, caller model.Device, folderID string) error {
	folder, err := s.store.FolderByID(r.Context(), folderID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			return errFolderNotFound
		}
		return err
	}
	if folder.OwnerID != caller.UserID {
		return errFolderForbidden
	}
	ok, err := s.store.IsDeviceSubscribed(r.Context(), folderID, caller.DeviceID)
	if err != nil {
		return err
	}
	if !ok {
		return errFolderNotSubscribed
	}
	return nil
}

type folderAccessError string

func (e folderAccessError) Error() string { return string(e) }

const (
	errFolderNotFound       folderAccessError = "folder not found"
	errFolderForbidden      folderAccessError = "forbidden"
	errFolderNotSubscribed  folderAccessError = "not subscribed"
)

func writeFolderAccessErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errFolderNotFound), errors.Is(err, db.ErrNotFound):
		writeErr(w, http.StatusNotFound, "folder not found")
	case errors.Is(err, errFolderForbidden):
		writeErr(w, http.StatusForbidden, "folder not owned by this account")
	case errors.Is(err, errFolderNotSubscribed):
		writeErr(w, http.StatusForbidden, "device not subscribed to folder")
	default:
		writeErr(w, http.StatusInternalServerError, "folder access check failed")
	}
}
