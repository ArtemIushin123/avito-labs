package domainerror_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
)

const (
	codeInvalidRequest = "invalid_request"
	codeInternalError  = "internal_error"
	secret             = "password=s3cret host=10.0.0.5"
)

func TestProblem(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantDetail string
	}{
		{
			name:       "validation error",
			err:        &trip.ValidationError{Field: "price", Message: "must not be negative"},
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
			wantDetail: "price: must not be negative",
		},
		{
			name:       "wrapped validation error",
			err:        fmt.Errorf("create trip: %w", &trip.ValidationError{Field: "start_point", Message: "out of range"}),
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
			wantDetail: "start_point: out of range",
		},
		{
			name:       "trip not found",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "wrapped trip not found",
			err:        fmt.Errorf("get trip: %w", trip.ErrTripNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name:       "trip completed",
			err:        fmt.Errorf("finish trip: %w", trip.ErrTripCompleted),
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name:       "driver busy",
			err:        fmt.Errorf("create trip: %w", trip.ErrDriverBusy),
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			name:       "unknown error",
			err:        errors.New(secret),
			wantStatus: http.StatusInternalServerError,
			wantCode:   codeInternalError,
		},
		{
			name:       "domain error wrapped with %v is not recognized",
			err:        fmt.Errorf("get trip: %v", trip.ErrTripNotFound),
			wantStatus: http.StatusInternalServerError,
			wantCode:   codeInternalError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := domainerror.Problem(tt.err)

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if int(body.Status) != status {
				t.Errorf("body.Status = %d, must duplicate HTTP status %d", body.Status, status)
			}
			if body.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tt.wantCode)
			}
			if body.Type == "" || body.Title == "" {
				t.Errorf("type = %q, title = %q, both must be set", body.Type, body.Title)
			}
			if tt.wantDetail != "" && (body.Detail == nil || *body.Detail != tt.wantDetail) {
				t.Errorf("detail = %v, want %q", body.Detail, tt.wantDetail)
			}

			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("marshal problem: %v", err)
			}
			if strings.Contains(string(raw), "s3cret") || strings.Contains(string(raw), "10.0.0.5") {
				t.Errorf("problem leaks internal error details: %s", raw)
			}
		})
	}
}

func TestWrite(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/trips/123", nil)
	domainerror.Write(w, r, errors.New(secret))

	got := decodeProblem(t, w, http.StatusInternalServerError)
	if got.Code != codeInternalError {
		t.Errorf("code = %q, want %q", got.Code, codeInternalError)
	}
	if got.Instance == nil || *got.Instance != "/api/v1/trips/123" {
		t.Errorf("instance = %v, want request path", got.Instance)
	}
	if strings.Contains(w.Body.String(), "s3cret") {
		t.Errorf("response leaks internal error details: %s", w.Body.String())
	}
}

func TestWriteInvalidRequest(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil)
	domainerror.WriteInvalidRequest(w, r, `unknown field "x"`)

	got := decodeProblem(t, w, http.StatusBadRequest)
	if got.Code != codeInvalidRequest {
		t.Errorf("code = %q, want %q", got.Code, codeInvalidRequest)
	}
	if got.Detail == nil || *got.Detail != `unknown field "x"` {
		t.Errorf("detail = %v, want the passed text", got.Detail)
	}
	if got.Instance == nil || *got.Instance != "/api/v1/trips" {
		t.Errorf("instance = %v, want request path", got.Instance)
	}

	_, fromDomain := domainerror.Problem(&trip.ValidationError{Field: "f", Message: "m"})
	if got.Type != fromDomain.Type || got.Title != fromDomain.Title {
		t.Errorf("format error = (%q, %q), domain validation = (%q, %q): must match",
			got.Type, got.Title, fromDomain.Type, fromDomain.Title)
	}
}

func TestWrite_ErrorSlot(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr error
	}{
		{name: "internal error is reported", err: errors.New(secret), wantErr: errors.New(secret)},
		{name: "client error is not reported", err: trip.ErrTripNotFound, wantErr: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, requestErr := domainerror.WithErrorSlot(context.Background())
			r := httptest.NewRequest(http.MethodGet, "/api/v1/trips/123", nil).WithContext(ctx)

			domainerror.Write(httptest.NewRecorder(), r, tt.err)

			got := requestErr()
			if fmt.Sprint(got) != fmt.Sprint(tt.wantErr) {
				t.Errorf("slot error = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestWrite_WithoutSlot(t *testing.T) {
	w := httptest.NewRecorder()
	domainerror.Write(w, httptest.NewRequest(http.MethodGet, "/", nil), errors.New(secret))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

func decodeProblem(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) api.Problem {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d", w.Code, wantStatus)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}

	var p api.Problem
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return p
}
