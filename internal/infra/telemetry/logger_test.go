package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/ArtemIushin123/template/internal/infra/telemetry"
)

const (
	testTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	testSpanID  = "00f067aa0ba902b7"
)

func captureStdout(t *testing.T, fn func()) []map[string]any {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = orig
	_ = w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		records = append(records, rec)
	}
	return records
}

func ctxWithSpan(t *testing.T) context.Context {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(testTraceID)
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex(testSpanID)
	if err != nil {
		t.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestNewLogger_RequiredFields(t *testing.T) {
	records := captureStdout(t, func() {
		logger, err := telemetry.NewLogger("info", "trip-service")
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		logger.InfoContext(ctxWithSpan(t), "trip created", "trip_id", "123")
	})

	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	rec := records[0]
	want := map[string]string{
		"level":    "INFO",
		"msg":      "trip created",
		"service":  "trip-service",
		"trace_id": testTraceID,
		"span_id":  testSpanID,
		"trip_id":  "123",
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, want %q", k, rec[k], v)
		}
	}
	if _, ok := rec["time"]; !ok {
		t.Error("record has no time field")
	}
}

func TestNewLogger_TraceIDSurvivesWith(t *testing.T) {
	records := captureStdout(t, func() {
		logger, err := telemetry.NewLogger("info", "trip-service")
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		logger.With("component", "http").InfoContext(ctxWithSpan(t), "request handled")
	})

	if len(records) != 1 || records[0]["trace_id"] != testTraceID {
		t.Fatalf("trace_id lost after With: %v", records)
	}
}

func TestNewLogger_NoSpan(t *testing.T) {
	records := captureStdout(t, func() {
		logger, err := telemetry.NewLogger("info", "trip-service")
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		logger.InfoContext(context.Background(), "service started")
	})

	if _, ok := records[0]["trace_id"]; ok {
		t.Errorf("trace_id present without a span: %v", records[0])
	}
}

func TestNewLogger_Level(t *testing.T) {
	records := captureStdout(t, func() {
		logger, err := telemetry.NewLogger("WARN", "trip-service") // регистр не важен
		if err != nil {
			t.Fatalf("NewLogger() error = %v", err)
		}
		logger.Info("dropped")
		logger.Warn("kept")
	})

	if len(records) != 1 || records[0]["msg"] != "kept" {
		t.Fatalf("LOG_LEVEL=warn must drop info records, got %v", records)
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	if _, err := telemetry.NewLogger("verbose", "trip-service"); err == nil {
		t.Fatal("NewLogger(\"verbose\") error = nil, want an error: the service must fail on a bad LOG_LEVEL")
	}
}
