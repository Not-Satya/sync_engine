package api

import (
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Not-Satya/sync_engine/internal/coord/auth"
	"github.com/Not-Satya/sync_engine/internal/coord/db"
	"github.com/Not-Satya/sync_engine/internal/coord/model"
	"github.com/Not-Satya/sync_engine/internal/ids"
)

type registerAccountRequest struct {
	Email    string            `json:"email"`
	Password string            `json:"password"`
	Device   deviceLinkRequest `json:"device"`
}

type deviceLinkRequest struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type loginRequest struct {
	Email    string            `json:"email"`
	Password string            `json:"password"`
	Device   deviceLinkRequest `json:"device"`
}

type heartbeatRequest struct {
	Endpoint string `json:"endpoint"`
}

type createFolderRequest struct {
	Name string `json:"name"`
}

type authResponse struct {
	UserID     string     `json:"user_id"`
	Device     deviceView `json:"device"`
	Token      string     `json:"token"` // plaintext, shown once
	PrivateKey string     `json:"device_private_key_hex"`
}

type deviceView struct {
	DeviceID  string     `json:"device_id"`
	UserID    string     `json:"user_id"`
	Name      string     `json:"name"`
	Platform  string     `json:"platform"`
	PublicKey string     `json:"public_key_hex"`
	CreatedAt time.Time  `json:"created_at"`
	LastSeen  time.Time  `json:"last_seen_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

func (s *Server) handleRegisterAccount(w http.ResponseWriter, r *http.Request) {
	var req registerAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || len(req.Password) < 8 || strings.TrimSpace(req.Device.Name) == "" {
		writeErr(w, http.StatusBadRequest, "email, password (>=8), and device.name required")
		return
	}

	userID, err := ids.NewUserID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "id generation failed")
		return
	}
	pwHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "password hash failed")
		return
	}
	now := time.Now().UTC()
	user := model.User{
		UserID:       userID,
		Email:        req.Email,
		PasswordHash: pwHash,
		CreatedAt:    now,
	}
	if err := s.store.CreateUser(r.Context(), user); err != nil {
		if errors.Is(err, db.ErrConflict) {
			writeErr(w, http.StatusConflict, "email already registered")
			return
		}
		writeErr(w, http.StatusInternalServerError, "create user failed")
		return
	}

	resp, status, errMsg := s.linkDevice(r, user, req.Device)
	if errMsg != "" {
		writeErr(w, status, errMsg)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" || strings.TrimSpace(req.Device.Name) == "" {
		writeErr(w, http.StatusBadRequest, "email, password, and device.name required")
		return
	}
	user, err := s.store.UserByEmail(r.Context(), req.Email)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := auth.VerifyPassword(user.PasswordHash, req.Password); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	resp, status, errMsg := s.linkDevice(r, user, req.Device)
	if errMsg != "" {
		writeErr(w, status, errMsg)
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) linkDevice(r *http.Request, user model.User, devReq deviceLinkRequest) (authResponse, int, string) {
	keys, err := ids.NewDeviceKeyMaterial()
	if err != nil {
		return authResponse{}, http.StatusInternalServerError, "device key generation failed"
	}
	plaintext, tokenHash, err := auth.IssueToken()
	if err != nil {
		return authResponse{}, http.StatusInternalServerError, "token issue failed"
	}
	now := time.Now().UTC()
	dev := model.Device{
		DeviceID:  keys.DeviceID,
		UserID:    user.UserID,
		Name:      strings.TrimSpace(devReq.Name),
		Platform:  strings.TrimSpace(devReq.Platform),
		PublicKey: append([]byte(nil), keys.PublicKey...),
		CreatedAt: now,
		LastSeen:  now,
	}
	tok := model.AuthToken{
		TokenHash: tokenHash,
		DeviceID:  dev.DeviceID,
		UserID:    user.UserID,
		CreatedAt: now,
	}
	if err := s.store.CreateDevice(r.Context(), dev, tok); err != nil {
		if errors.Is(err, db.ErrConflict) {
			return authResponse{}, http.StatusConflict, "device id conflict"
		}
		return authResponse{}, http.StatusInternalServerError, "create device failed"
	}
	return authResponse{
		UserID:     user.UserID,
		Device:     toDeviceView(dev),
		Token:      plaintext,
		PrivateKey: hex.EncodeToString(keys.PrivateKey),
	}, 0, ""
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	user, err := s.store.UserByID(r.Context(), dev.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "user lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": user.UserID,
		"email":   user.Email,
		"device":  toDeviceView(dev),
	})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	list, err := s.store.ListDevices(r.Context(), dev.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list devices failed")
		return
	}
	views := make([]deviceView, 0, len(list))
	for _, d := range list {
		views = append(views, toDeviceView(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": views})
}

func (s *Server) handleRevokeDevice(w http.ResponseWriter, r *http.Request) {
	caller := deviceFrom(r.Context())
	targetID := chi.URLParam(r, "deviceID")
	if targetID == "" {
		writeErr(w, http.StatusBadRequest, "device id requred")
		return
	}

	target, err := s.store.DeviceByID(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "device lookup failed")
		return
	}
	if target.UserID != caller.UserID {
		writeErr(w, http.StatusForbidden, "device not on this account")
		return
	}

	now := time.Now().UTC()
	if err := s.store.RevokeDevice(r.Context(), targetID, now); err != nil {
		if errors.Is(err, db.ErrRevoked) {
			writeErr(w, http.StatusConflict, "device already revoked")
			return
		}
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "revoke failed")
		return
	}

	updated, err := s.store.DeviceByID(r.Context(), targetID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_id":  targetID,
			"revoked_id": now,
		})
		return
	}
	writeJSON(w, http.StatusOK, toDeviceView(updated))
}

func (s *Server) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	var req createFolderRequest
	if err := decodeJSON(r, &req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	folderID, err := ids.NewFolderID()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "id generation failed")
		return
	}
	now := time.Now().UTC()
	folder := model.Folder{
		FolderID:  folderID,
		OwnerID:   dev.UserID,
		Name:      strings.TrimSpace(req.Name),
		CreatedAt: now,
	}
	if err := s.store.CreateFolder(r.Context(), folder); err != nil {
		writeErr(w, http.StatusInternalServerError, "create folder failed")
		return
	}
	// Creating device auto-subscribes; other devices opt in explicitly.
	_ = s.store.Subscribe(r.Context(), model.Subscription{
		FolderID:     folder.FolderID,
		DeviceID:     dev.DeviceID,
		SubscribedAt: now,
	})
	writeJSON(w, http.StatusCreated, folder)
}

func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	list, err := s.store.ListFolders(r.Context(), dev.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list folders failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"folders": list})
}

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	folderID := chi.URLParam(r, "folderID")
	folder, err := s.store.FolderByID(r.Context(), folderID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "folder not found")
		return
	}
	if folder.OwnerID != dev.UserID {
		writeErr(w, http.StatusForbidden, "folder not owned by this account")
		return
	}
	sub := model.Subscription{
		FolderID:     folderID,
		DeviceID:     dev.DeviceID,
		SubscribedAt: time.Now().UTC(),
	}
	if err := s.store.Subscribe(r.Context(), sub); err != nil {
		writeErr(w, http.StatusInternalServerError, "subscribe failed")
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	folderID := chi.URLParam(r, "folderID")
	if err := s.store.Unsubscribe(r.Context(), folderID, dev.DeviceID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "subscription not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "unsubscribe failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	list, err := s.store.ListSubscriptionsByDevice(r.Context(), dev.DeviceID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list subscriptions failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": list})
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	var req heartbeatRequest
	_ = decodeJSON(r, &req) // endpoint optional

	now := time.Now().UTC()
	p := model.Presence{
		DeviceID:  dev.DeviceID,
		Status:    model.PresenceOnline,
		Endpoint:  strings.TrimSpace(req.Endpoint),
		UpdatedAt: now,
	}
	if err := s.store.UpsertPresence(r.Context(), p); err != nil {
		writeErr(w, http.StatusInternalServerError, "presence update failed")
		return
	}
	_ = s.store.TouchDevice(r.Context(), dev.DeviceID, now)
	// Expire peers that missed their TTL so lists stay honest.
	_, _ = s.store.ExpireStalePresence(r.Context(), now.Add(-s.presenceTTL))
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleListPresence(w http.ResponseWriter, r *http.Request) {
	dev := deviceFrom(r.Context())
	now := time.Now().UTC()
	_, _ = s.store.ExpireStalePresence(r.Context(), now.Add(-s.presenceTTL))
	list, err := s.store.ListPresenceForUser(r.Context(), dev.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list presence failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"presence":    list,
		"ttl_seconds": int(s.presenceTTL.Seconds()),
	})
}

func toDeviceView(d model.Device) deviceView {
	return deviceView{
		DeviceID:  d.DeviceID,
		UserID:    d.UserID,
		Name:      d.Name,
		Platform:  d.Platform,
		PublicKey: hex.EncodeToString(d.PublicKey),
		CreatedAt: d.CreatedAt,
		LastSeen:  d.LastSeen,
		RevokedAt: d.RevokedAt,
	}
}
