package positions

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	"github.com/ArtemIushin123/template/internal/model"
)

func TestQueueMetrics(t *testing.T) {
	var logs bytes.Buffer
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")
	events, err := telemetry.NewPositionRequests(slog.New(slog.NewJSONHandler(&logs, nil)), meter)
	if err != nil {
		t.Fatal(err)
	}
	q := NewQueue(1, events)
	trip := model.StaleTrip{TripID: uuid.New()}
	q.Enqueue(t.Context(), trip)
	q.Enqueue(t.Context(), trip)
	q.Enqueue(t.Context(), model.StaleTrip{TripID: uuid.New()})
	q.Cancel(trip.TripID)
	q.close()
	if _, ok := q.next(t.Context()); ok {
		t.Fatal("cancelled task was returned for processing")
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	results := make(map[string]int64)
	var sizeOK, cancelledOK bool
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "tripgo.stale_checks":
				for _, p := range m.Data.(metricdata.Sum[int64]).DataPoints {
					for _, a := range p.Attributes.ToSlice() {
						if a.Key == "result" {
							results[a.Value.AsString()] = p.Value
						}
					}
				}
			case "tripgo.position_requests":
				data := m.Data.(metricdata.Sum[int64]).DataPoints
				if len(data) == 1 {
					for _, a := range data[0].Attributes.ToSlice() {
						cancelledOK = a.Key == "result" && a.Value.AsString() == "cancelled" && data[0].Value == 1
					}
				}
			case "tripgo.position_requests.queue.size":
				data := m.Data.(metricdata.Gauge[int64]).DataPoints
				sizeOK = len(data) == 1 && data[0].Value == 0
			}
		}
	}
	if len(results) != 3 || results[string(Queued)] != 1 || results[string(Duplicate)] != 1 || results[string(Dropped)] != 1 || !sizeOK || !cancelledOK || bytes.Count(logs.Bytes(), []byte("\n")) != 1 {
		t.Fatalf("unexpected queue metrics: results=%v, size=%v, cancelled=%v, logs=%s", results, sizeOK, cancelledOK, &logs)
	}
}
