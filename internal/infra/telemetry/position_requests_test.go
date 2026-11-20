package telemetry_test

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/ArtemIushin123/template/internal/infra/telemetry"
)

func TestPositionRequestMetrics(t *testing.T) {
	for _, tc := range []struct {
		result string
		err    error
	}{{"success", nil}, {"error", errors.New("unavailable")}} {
		t.Run(tc.result, func(t *testing.T) {
			var logs bytes.Buffer
			reader := sdkmetric.NewManualReader()
			meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test")
			events, err := telemetry.NewPositionRequests(slog.New(slog.NewJSONHandler(&logs, nil)), meter)
			if err != nil {
				t.Fatal(err)
			}
			events.Finished(t.Context(), "request-test", 250*time.Millisecond, tc.err)
			if (logs.Len() > 0) != (tc.err != nil) {
				t.Fatalf("unexpected logs %q", logs.String())
			}

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(t.Context(), &rm); err != nil {
				t.Fatal(err)
			}
			want := attribute.NewSet(attribute.String("result", tc.result))
			var counterOK, histogramOK bool
			for _, scope := range rm.ScopeMetrics {
				for _, m := range scope.Metrics {
					switch data := m.Data.(type) {
					case metricdata.Sum[int64]:
						counterOK = m.Name == "tripgo.position_requests" && len(data.DataPoints) == 1 && data.DataPoints[0].Value == 1 && data.DataPoints[0].Attributes.Equals(&want)
					case metricdata.Histogram[float64]:
						histogramOK = m.Name == "tripgo.position_requests.duration" && m.Unit == "s" && len(data.DataPoints) == 1 && data.DataPoints[0].Count == 1 && data.DataPoints[0].Sum == 0.25 && data.DataPoints[0].Attributes.Equals(&want)
					}
				}
			}
			if !counterOK || !histogramOK {
				t.Fatalf("unexpected measurements: %+v", rm.ScopeMetrics)
			}
		})
	}
}
