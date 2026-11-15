package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/telemetry"
)

func counterValue(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) != 1 {
				t.Fatalf("%s: unexpected data %+v", name, m.Data)
			}
			return sum.DataPoints[0].Value
		}
	}
	return 0
}

func TestTripEvents(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")

	events, err := telemetry.NewTripEvents(logger, meter)
	if err != nil {
		t.Fatalf("NewTripEvents() error = %v", err)
	}

	tr := &trip.Trip{ID: uuid.MustParse("11111111-1111-1111-1111-111111111111"), UserID: uuid.New(), DriverID: uuid.New()}
	events.TripCreated(context.Background(), tr)
	events.TripCreated(context.Background(), tr)
	events.TripCompleted(context.Background(), tr)

	if got := counterValue(t, reader, "tripgo.trips.created"); got != 2 {
		t.Errorf("tripgo.trips.created = %d, want 2", got)
	}
	if got := counterValue(t, reader, "tripgo.trips.completed"); got != 1 {
		t.Errorf("tripgo.trips.completed = %d, want 1", got)
	}

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d log records, want 3", len(lines))
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[2]), &last); err != nil {
		t.Fatal(err)
	}
	if last["level"] != "INFO" || last["msg"] != "trip completed" || last["trip_id"] != tr.ID.String() {
		t.Errorf("record = %v, want INFO \"trip completed\" with trip_id", last)
	}
	// id пассажира и водителя — персональные данные, в лог не пишем
	if strings.Contains(logs.String(), tr.UserID.String()) || strings.Contains(logs.String(), tr.DriverID.String()) {
		t.Errorf("user or driver id leaked into the log: %s", logs.String())
	}
}
