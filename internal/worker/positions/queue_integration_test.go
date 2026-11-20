//go:build integration

package positions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric/noop"

	hcreate "github.com/ArtemIushin123/template/internal/http/position/create"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	"github.com/ArtemIushin123/template/internal/infra/telemetry"
	"github.com/ArtemIushin123/template/internal/model"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
	ucreate "github.com/ArtemIushin123/template/internal/usecase/position/create"
)

type brokenPositionUpdate struct{ *triprepo.Repository }

func (r brokenPositionUpdate) AdvanceLastPositionAt(context.Context, uuid.UUID, time.Time) error {
	return errors.New("position update failed")
}

func TestPositionHTTPCancelsOnlyCommittedTask(t *testing.T) {
	for _, status := range []int{http.StatusCreated, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			pool := pgtest.Pool(t)
			repo := triprepo.New(pool, pgtest.QueryTimeout)
			manager := transaction.New(pool, pgtest.QueryTimeout)
			tr := pgtest.NewTrip(t)
			pgtest.DeleteTrips(t, pool, tr.ID)
			if err := repo.Create(t.Context(), tr); err != nil {
				t.Fatal(err)
			}

			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			meter := noop.NewMeterProvider().Meter("test")
			events, err := telemetry.NewTripEvents(logger, meter)
			if err != nil {
				t.Fatal(err)
			}
			q, stats := testQueue(t, 1)
			q.Enqueue(t.Context(), model.StaleTrip{TripID: tr.ID, DriverID: tr.DriverID})
			clock := func() time.Time { return tr.StartedAt }
			uc := ucreate.New(repo, manager, events, q, clock)
			if status == http.StatusInternalServerError {
				uc = ucreate.New(brokenPositionUpdate{repo}, manager, events, q, clock)
			}

			body := fmt.Sprintf(`{"coordinates":{"latitude":1,"longitude":2},"recorded_at":%q}`, tr.StartedAt.Format(time.RFC3339Nano))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tr.ID.String()+"/positions", strings.NewReader(body))
			response := httptest.NewRecorder()
			hcreate.New(uc).CreateTripPosition(response, req, tr.ID)
			if response.Code != status {
				t.Fatalf("status=%d, want %d: %s", response.Code, status, response.Body)
			}
			q.close()
			item, ok := q.next(t.Context())
			if ok != (status == http.StatusInternalServerError) {
				t.Fatalf("queued request processable=%v after HTTP %d", ok, status)
			}
			if ok {
				q.complete(item)
			}
			wantCancelled := 0
			if status == http.StatusCreated {
				wantCancelled = 1
			}
			if stats.cancelled != wantCancelled {
				t.Fatalf("cancelled=%d, want %d", stats.cancelled, wantCancelled)
			}
		})
	}
}
