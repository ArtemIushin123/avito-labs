package push

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	"github.com/ArtemIushin123/template/internal/model"
)

func testClient(t *testing.T, url string, timeout time.Duration) *Client {
	t.Helper()
	events, err := telemetry.NewPositionRequests(slog.New(slog.NewTextHandler(io.Discard, nil)), noop.NewMeterProvider().Meter("test"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(url, timeout, events)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.CloseIdleConnections)
	return c
}

func TestPushStatus(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusTemporaryRedirect, http.StatusOK} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == "/redirected" {
					w.WriteHeader(http.StatusAccepted)
					return
				}
				w.Header().Set("Location", "/redirected")
				w.WriteHeader(status)
			}))
			t.Cleanup(server.Close)
			err := testClient(t, server.URL, time.Second).RequestPosition(t.Context(), model.StaleTrip{})
			var statusErr *StatusError
			if !errors.As(err, &statusErr) || statusErr.StatusCode != status || calls.Load() != 1 {
				t.Fatalf("status %d: err=%v, calls=%d", status, err, calls.Load())
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type observedBody struct {
	io.Reader
	closed bool
}

func (b *observedBody) Close() error { b.closed = true; return nil }

func TestPushClosesResponse(t *testing.T) {
	for _, status := range []int{http.StatusAccepted, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			body := &observedBody{Reader: strings.NewReader("{}")}
			client := testClient(t, "http://example.test", time.Second)
			client.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body}, nil
			})
			err := client.RequestPosition(t.Context(), model.StaleTrip{})
			if !body.closed || (err == nil) != (status == http.StatusAccepted) {
				t.Fatalf("closed=%v, err=%v", body.closed, err)
			}
		})
	}
}

func TestPushTransportTimeout(t *testing.T) {
	client := testClient(t, "http://example.test", 20*time.Millisecond)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	err := client.RequestPosition(ctx, model.StaleTrip{})
	var statusErr *StatusError
	if !errors.Is(err, context.DeadlineExceeded) || errors.As(err, &statusErr) || ctx.Err() != nil {
		t.Fatalf("timeout error=%v, parent context error=%v", err, ctx.Err())
	}
}

func TestPushHonorsCancellation(t *testing.T) {
	client := testClient(t, "http://example.test", time.Second)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := client.RequestPosition(ctx, model.StaleTrip{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancelled", err)
	}
}
