package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/Not-Satya/sync_engine/internal/coord/auth"
	"github.com/Not-Satya/sync_engine/internal/coord/model"
)

type ctxKey int

const (
	ctxToken ctxKey = iota
	ctxDevice
)

func (s *Server) requireDevice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		plaintext := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		tok, err := s.store.AuthByTokenHash(r.Context(), auth.HashToken(plaintext))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		dev, err := s.store.DeviceByID(r.Context(), tok.DeviceID)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unknown device")
			return
		}
		if dev.Revoked() {
			writeErr(w, http.StatusUnauthorized, "device revoked")
			return
		}
		ctx := context.WithValue(r.Context(), ctxToken, tok)
		ctx = context.WithValue(ctx, ctxDevice, dev)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func deviceFrom(ctx context.Context) model.Device {
	d, _ := ctx.Value(ctxDevice).(model.Device)
	return d
}
