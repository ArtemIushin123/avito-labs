package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/middleware"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestRecover_Panic(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	h := middleware.Recover(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("db password is s3cret")
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/trips", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	var p api.Problem
	if err := json.NewDecoder(strings.NewReader(w.Body.String())).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Code != "internal_error" {
		t.Errorf("code = %q, want internal_error", p.Code)
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		t.Errorf("panic message leaked to the client: %s", w.Body.String())
	}

	var rec map[string]any
	if err := json.Unmarshal(logs.Bytes(), &rec); err != nil {
		t.Fatalf("panic was not logged as one JSON record: %q", logs.String())
	}
	if rec["level"] != "ERROR" || rec["msg"] != "panic recovered" {
		t.Errorf("log = %v, want level ERROR and msg \"panic recovered\"", rec)
	}
	if stack, _ := rec["stack"].(string); !strings.Contains(stack, "goroutine") {
		t.Errorf("log has no stack trace: %v", rec["stack"])
	}
}

func TestRecover_NoPanic(t *testing.T) {
	h := middleware.Recover(discardLogger())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusTeapot {
		t.Errorf("status = %d, want the handler's own status %d", w.Code, http.StatusTeapot)
	}
}

func TestRecover_ErrAbortHandlerIsRepanicked(t *testing.T) {
	h := middleware.Recover(discardLogger())(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if rec := recover(); rec != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler to be re-panicked", rec)
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Error("ServeHTTP returned normally, want a re-panic")
}
