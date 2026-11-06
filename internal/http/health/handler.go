package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ArtemIushin123/template/internal/generated"
)

type Handler struct {
	db               pinger
	dbConnectTimeout time.Duration
}

func New(db pinger, dbConnectTimeout time.Duration) *Handler {
	return &Handler{db: db, dbConnectTimeout: dbConnectTimeout}
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK, api.Ok)
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.dbConnectTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		writeStatus(w, http.StatusServiceUnavailable, api.Unavailable)
		return
	}
	writeStatus(w, http.StatusOK, api.Ok)
}

func writeStatus(w http.ResponseWriter, code int, status api.HealthResponseStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(api.HealthResponse{Status: status})
}
