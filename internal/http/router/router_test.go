package router_test

import (
	"bytes"
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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/health"
	"github.com/ArtemIushin123/template/internal/http/middleware"
	positioncreate "github.com/ArtemIushin123/template/internal/http/position/create"
	positionlist "github.com/ArtemIushin123/template/internal/http/position/list"
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

func (f *fakeUsecases) CreatePosition(_ context.Context, in model.CreatePositionInput) (*trip.Position, error) {
	f.called, f.gotID = "CreatePosition", in.TripID
	return &trip.Position{ID: 1, TripID: in.TripID, Coordinates: in.Coordinates, RecordedAt: in.RecordedAt}, nil
}

func (f *fakeUsecases) ListPositions(_ context.Context, id uuid.UUID) ([]trip.Position, error) {
	f.called, f.gotID = "ListPositions", id
	return nil, nil
}

type fakePinger struct{}

func (fakePinger) Ping(context.Context) error { return nil }

func newRouter(uc *fakeUsecases) http.Handler {
	return newRouterWithLogger(uc, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newRouterWithLogger(uc *fakeUsecases, logger *slog.Logger) http.Handler {
	return newRouterWithMeter(uc, logger, noop.NewMeterProvider().Meter("test"))
}

func newRouterWithMeter(uc *fakeUsecases, logger *slog.Logger, meter metric.Meter) http.Handler {
	httpMetrics, err := middleware.NewMetrics(meter)
	if err != nil {
		panic(err)
	}
	return router.New(
		create.New(uc),
		get.New(uc),
		finish.New(uc),
		positioncreate.New(uc),
		positionlist.New(uc),
		health.New(fakePinger{}, time.Second),
		logger,
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
		{name: "route", method: http.MethodGet, path: tripsPath + "/" + tripID.String() + "/positions", wantStatus: http.StatusOK, wantCall: "ListPositions"},
		{name: "position", method: http.MethodPost, path: tripsPath + "/" + tripID.String() + "/positions", body: `{"coordinates":{"latitude":0,"longitude":0},"recorded_at":"2026-08-24T12:00:05Z"}`, wantStatus: http.StatusCreated, wantCall: "CreatePosition"},
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
	for _, path := range []string{tripsPath + "/not-a-uuid", tripsPath + "/not-a-uuid/finish", tripsPath + "/not-a-uuid/positions"} {
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

func TestRouter_MeasuresInvalidTripID(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	uc := &fakeUsecases{}
	h := newRouterWithMeter(uc, slog.New(slog.NewTextHandler(io.Discard, nil)), provider.Meter("test"))
	w := serve(h, http.MethodGet, tripsPath+"/not-a-uuid", "")
	if w.Code != http.StatusBadRequest || uc.called != "" {
		t.Fatalf("status = %d, usecase = %q, want 400 before usecase", w.Code, uc.called)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if measurement.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := measurement.Data.(metricdata.Histogram[float64])
			if !ok || len(hist.DataPoints) != 1 || hist.DataPoints[0].Count != 1 {
				t.Fatalf("want one duration measurement, got %+v", measurement.Data)
			}
			attrs := hist.DataPoints[0].Attributes
			route, _ := attrs.Value("http.route")
			status, _ := attrs.Value("http.response.status_code")
			if route.AsString() != tripsPath+"/{tripId}" || status.AsInt64() != http.StatusBadRequest {
				t.Errorf("http.route = %v, http.response.status_code = %v, want trip template and 400", route, status)
			}
			return
		}
	}
	t.Fatal("missing HTTP duration for invalid trip ID")
}

type requestLogHandler struct {
	slog.Handler
	spanContext trace.SpanContext
}

func (h *requestLogHandler) Handle(ctx context.Context, record slog.Record) error {
	h.spanContext = trace.SpanContextFromContext(ctx)
	return h.Handler.Handle(ctx, record)
}

func TestRouter_PanicIsRecovered(t *testing.T) {
	spans := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	originalProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(originalProvider) })

	var logs bytes.Buffer
	logHandler := &requestLogHandler{Handler: slog.NewJSONHandler(&logs, nil)}
	h := newRouterWithLogger(&fakeUsecases{panic: true}, slog.New(logHandler))
	w := serve(h, http.MethodGet, tripsPath+"/"+tripID.String(), "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}

	var record struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil || record.Status != http.StatusInternalServerError {
		t.Fatalf("want one request log with status 500, got %q", logs.String())
	}

	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	span := ended[0]
	if span.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", span.Status())
	}
	attrs := attribute.NewSet(span.Attributes()...)
	if status, ok := attrs.Value("http.response.status_code"); !ok || status.AsInt64() != http.StatusInternalServerError {
		t.Errorf("span response status = %v, want 500", status)
	}
	if !logHandler.spanContext.IsValid() || !logHandler.spanContext.Equal(span.SpanContext()) {
		t.Errorf("log span context = %v, want HTTP span context %v", logHandler.spanContext, span.SpanContext())
	}
}
