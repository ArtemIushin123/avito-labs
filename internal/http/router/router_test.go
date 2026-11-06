package router_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/health"
	"github.com/ArtemIushin123/template/internal/http/middleware"
	"github.com/ArtemIushin123/template/internal/http/router"
	"github.com/ArtemIushin123/template/internal/http/trip/create"
	"github.com/ArtemIushin123/template/internal/http/trip/finish"
	"github.com/ArtemIushin123/template/internal/http/trip/get"
	"github.com/ArtemIushin123/template/internal/model"
)

const (
	tripsPath          = "/api/v1/trips"
	codeInvalidRequest = "invalid_request"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type fakeUsecases struct {
	called string
	gotID  uuid.UUID
	panic  bool
}

func (f *fakeUsecases) tripFor(method string, id uuid.UUID) *trip.Trip {
	if f.panic {
		panic("boom")
	}
	f.called, f.gotID = method, id
	return &trip.Trip{ID: id, Status: trip.StatusActive, StartedAt: time.Now()}
}

func (f *fakeUsecases) GetByID(_ context.Context, id uuid.UUID) (*trip.Trip, error) {
	return f.tripFor("GetByID", id), nil
}

func (f *fakeUsecases) Finish(_ context.Context, id uuid.UUID) (*trip.Trip, error) {
	return f.tripFor("Finish", id), nil
}

func (f *fakeUsecases) Create(_ context.Context, _ model.CreateTripInput) (*trip.Trip, error) {
	return f.tripFor("Create", tripID), nil
}

type fakePinger struct{}

func (fakePinger) Ping(context.Context) error { return nil }

func newRouter(uc *fakeUsecases) http.Handler {
	httpMetrics, err := middleware.NewMetrics(noop.NewMeterProvider().Meter("test"))
	if err != nil {
		panic(err)
	}
	return router.New(
		create.New(uc),
		get.New(uc),
		finish.New(uc),
		health.New(fakePinger{}, time.Second),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		httpMetrics,
	)
}

func serve(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w
}

func TestRouter_Routes(t *testing.T) {
	validBody := `{"user_id":"22222222-2222-2222-2222-222222222222","driver_id":"33333333-3333-3333-3333-333333333333",` +
		`"start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1}`

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantCall   string
	}{
		{name: "health", method: http.MethodGet, path: "/health", wantStatus: http.StatusOK},
		{name: "ready", method: http.MethodGet, path: "/ready", wantStatus: http.StatusOK},
		{name: "create", method: http.MethodPost, path: tripsPath, body: validBody, wantStatus: http.StatusCreated, wantCall: "Create"},
		{name: "get", method: http.MethodGet, path: tripsPath + "/" + tripID.String(), wantStatus: http.StatusOK, wantCall: "GetByID"},
		{name: "finish", method: http.MethodPost, path: tripsPath + "/" + tripID.String() + "/finish", wantStatus: http.StatusOK, wantCall: "Finish"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &fakeUsecases{}

			w := serve(newRouter(uc), tt.method, tt.path, tt.body)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", w.Code, tt.wantStatus, w.Body.String())
			}
			if uc.called != tt.wantCall {
				t.Errorf("called usecase %q, want %q", uc.called, tt.wantCall)
			}
			if tt.wantCall != "" && uc.gotID != tripID {
				t.Errorf("usecase got id %v, want %v from the path", uc.gotID, tripID)
			}
		})
	}
}

func TestRouter_InvalidTripID(t *testing.T) {
	for _, path := range []string{tripsPath + "/not-a-uuid", tripsPath + "/not-a-uuid/finish"} {
		method := http.MethodGet
		if strings.HasSuffix(path, "/finish") {
			method = http.MethodPost
		}

		t.Run(method+" "+path, func(t *testing.T) {
			w := serve(newRouter(&fakeUsecases{}), method, path, "")

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", w.Code)
			}
			if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", ct)
			}
			var p api.Problem
			if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
				t.Fatalf("decode problem: %v", err)
			}
			if p.Code != codeInvalidRequest {
				t.Errorf("code = %q, want %q", p.Code, codeInvalidRequest)
			}
			if p.Detail == nil || !strings.Contains(*p.Detail, "tripId") {
				t.Errorf("detail = %v, want it to name the tripId parameter", p.Detail)
			}
		})
	}
}

func TestRouter_PanicIsRecovered(t *testing.T) {
	w := serve(newRouter(&fakeUsecases{panic: true}), http.MethodGet, tripsPath+"/"+tripID.String(), "")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}
