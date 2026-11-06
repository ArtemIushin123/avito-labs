package health_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/health"
)

const pingTimeout = time.Second

func TestHandler_Health(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := NewMockpinger(ctrl)

	w := httptest.NewRecorder()
	health.New(db, pingTimeout).Health(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assertHealth(t, w, http.StatusOK, api.Ok)
}

func TestHandler_Ready(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantCode   int
		wantStatus api.HealthResponseStatus
	}{
		{name: "database available", pingErr: nil, wantCode: http.StatusOK, wantStatus: api.Ok},
		{name: "database unavailable", pingErr: errors.New("connection refused"), wantCode: http.StatusServiceUnavailable, wantStatus: api.Unavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			db := NewMockpinger(ctrl)
			db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
				if _, ok := ctx.Deadline(); !ok {
					t.Error("Ping called without a deadline")
				}
				return tt.pingErr
			})

			w := httptest.NewRecorder()
			health.New(db, pingTimeout).Ready(w, httptest.NewRequest(http.MethodGet, "/ready", nil))

			assertHealth(t, w, tt.wantCode, tt.wantStatus)
		})
	}
}

func assertHealth(t *testing.T, w *httptest.ResponseRecorder, wantCode int, wantStatus api.HealthResponseStatus) {
	t.Helper()

	if w.Code != wantCode {
		t.Fatalf("status code = %d, want %d", w.Code, wantCode)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var got api.HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if got.Status != wantStatus {
		t.Errorf("status = %q, want %q", got.Status, wantStatus)
	}
}
