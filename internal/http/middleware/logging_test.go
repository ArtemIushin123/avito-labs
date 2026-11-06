package middleware_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
	"github.com/ArtemIushin123/template/internal/http/middleware"
)

const tripRoute = "/api/v1/trips/{tripId}"

func newLoggedRouter(logs *bytes.Buffer, respond func(w http.ResponseWriter, r *http.Request)) http.Handler {
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	r := chi.NewRouter()
	r.Use(middleware.Logging(logger))
	r.Get(tripRoute, respond)
	return r
}

func readRecords(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	out := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		out = append(out, rec)
	}
	return out
}

func TestLogging(t *testing.T) {
	tests := []struct {
		name      string
		respond   func(w http.ResponseWriter, r *http.Request)
		wantCode  float64
		wantLevel string
		wantError string
	}{
		{
			name:      "2xx is Info",
			respond:   func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
			wantCode:  http.StatusOK,
			wantLevel: levelInfo,
		},
		{
			name:      "status 200 when handler writes body without WriteHeader",
			respond:   func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) },
			wantCode:  http.StatusOK,
			wantLevel: levelInfo,
		},
		{
			name: "4xx is Info and has no error field",
			respond: func(w http.ResponseWriter, r *http.Request) {
				domainerror.Write(w, r, fmt.Errorf("get trip: %w", trip.ErrTripNotFound))
			},
			wantCode:  http.StatusNotFound,
			wantLevel: levelInfo,
		},
		{
			name: "5xx is Error and carries the internal error",
			respond: func(w http.ResponseWriter, r *http.Request) {
				domainerror.Write(w, r, fmt.Errorf("get trip: %w", errors.New("connection refused")))
			},
			wantCode:  http.StatusInternalServerError,
			wantLevel: "ERROR",
			wantError: "get trip: connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			h := newLoggedRouter(&logs, tt.respond)

			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/trips/123", nil))

			records := readRecords(t, &logs)
			if len(records) != 1 {
				t.Fatalf("got %d log records, want exactly one per request: %v", len(records), records)
			}
			rec := records[0]

			if rec["msg"] != "request handled" || rec["method"] != http.MethodGet {
				t.Errorf("msg, method = %v, %v", rec["msg"], rec["method"])
			}
			if rec["route"] != tripRoute {
				t.Errorf("route = %v, want template %q", rec["route"], tripRoute)
			}
			if rec["status"] != tt.wantCode {
				t.Errorf("status = %v, want %v", rec["status"], tt.wantCode)
			}
			if rec["level"] != tt.wantLevel {
				t.Errorf("level = %v, want %v", rec["level"], tt.wantLevel)
			}
			if _, ok := rec["duration_ms"]; !ok {
				t.Error("record has no duration_ms")
			}
			gotErr, hasErr := rec["error"]
			switch {
			case tt.wantError == "" && hasErr:
				t.Errorf("error = %v, want no error field", gotErr)
			case tt.wantError != "" && gotErr != tt.wantError:
				t.Errorf("error = %v, want %q", gotErr, tt.wantError)
			}
		})
	}
}

func TestLogging_NoBodies(t *testing.T) {
	var logs bytes.Buffer
	h := newLoggedRouter(&logs, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"user_id":"secret-user"}`))
	})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/trips/123", strings.NewReader(`{"driver_id":"secret-driver"}`)))

	if strings.Contains(logs.String(), "secret-") {
		t.Errorf("request or response body leaked into the log: %s", logs.String())
	}
}

func TestLogging_UnmatchedRoute(t *testing.T) {
	var logs bytes.Buffer
	h := newLoggedRouter(&logs, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/no/such/path", nil))

	records := readRecords(t, &logs)
	if len(records) != 1 || records[0]["route"] == "/no/such/path" {
		t.Fatalf("unmatched request must be logged once without the raw path as route: %v", records)
	}
}

const levelInfo = "INFO"
