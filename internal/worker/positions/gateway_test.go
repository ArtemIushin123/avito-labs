package positions

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/ArtemIushin123/template/internal/gateway/push"
	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	"github.com/ArtemIushin123/template/internal/model"
)

func TestPushGatewayContract(t *testing.T) {
	tr := model.StaleTrip{TripID: uuid.New(), DriverID: uuid.New()}
	propagator := propagation.TraceContext{}
	old := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagator)
	t.Cleanup(func() { otel.SetTextMapPropagator(old) })

	spans := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	ctx, parent := provider.Tracer("test").Start(t.Context(), "process position")
	defer parent.End()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/push" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected push: %s %s %v", r.Method, r.URL, r.Header)
		}
		sc := trace.SpanContextFromContext(propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header)))
		if !sc.IsValid() || sc.TraceID() != parent.SpanContext().TraceID() || sc.SpanID() == parent.SpanContext().SpanID() {
			t.Error("push is missing its own span in the worker trace")
		}
		var payload struct {
			RequestID   string            `json:"request_id"`
			RecipientID uuid.UUID         `json:"recipient_id"`
			Kind        string            `json:"kind"`
			Data        map[string]string `json:"data"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil || payload.RequestID == "" || payload.RecipientID != tr.DriverID || payload.Kind != "REQUEST_POSITION" || payload.Data["trip_id"] != tr.TripID.String() {
			t.Errorf("unexpected push body: %+v, %v", payload, err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	events, err := telemetry.NewPositionRequests(slog.New(slog.NewTextHandler(io.Discard, nil)), noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := push.New(server.URL+"/", time.Second, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	var gateway pushGateway = client
	if err := gateway.RequestPosition(ctx, tr); err != nil {
		t.Fatal(err)
	}
	ended := spans.Ended()
	if len(ended) != 1 || ended[0].SpanKind() != trace.SpanKindClient || ended[0].Parent().SpanID() != parent.SpanContext().SpanID() {
		t.Fatalf("unexpected outbound spans: %v", ended)
	}
}
