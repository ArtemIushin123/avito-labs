package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	"github.com/ArtemIushin123/template/internal/model"
)

type Client struct {
	endpoint   string
	httpClient *http.Client
	transport  *http.Transport
	events     *telemetry.PositionRequests
}

type StatusError struct {
	StatusCode int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("push service returned HTTP %d", e.StatusCode)
}

func New(baseURL string, timeout time.Duration, events *telemetry.PositionRequests) (*Client, error) {
	endpoint, err := url.JoinPath(baseURL, "api/v1/push")
	if err != nil {
		return nil, fmt.Errorf("build push endpoint: %w", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	client := &http.Client{
		Transport:     otelhttp.NewTransport(transport),
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Client{endpoint: endpoint, httpClient: client, transport: transport, events: events}, nil
}

func (c *Client) CloseIdleConnections() {
	c.transport.CloseIdleConnections()
}

type pushRequest struct {
	RequestID   string            `json:"request_id"`
	RecipientID uuid.UUID         `json:"recipient_id"`
	Kind        string            `json:"kind"`
	Data        map[string]string `json:"data"`
}

func (c *Client) RequestPosition(ctx context.Context, trip model.StaleTrip) error {
	requestID := uuid.NewString()
	started := time.Now()
	err := c.send(ctx, pushRequest{
		RequestID:   requestID,
		RecipientID: trip.DriverID,
		Kind:        "REQUEST_POSITION",
		Data:        map[string]string{"trip_id": trip.TripID.String()},
	})
	c.events.Finished(ctx, requestID, time.Since(started), err)
	return err
}

func (c *Client) send(ctx context.Context, payload pushRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode push request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send push request: %w", err)
	}
	defer resp.Body.Close()
	_, readErr := io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusAccepted {
		return &StatusError{StatusCode: resp.StatusCode}
	}
	if readErr != nil {
		return fmt.Errorf("read push response: %w", readErr)
	}
	return nil
}
