//go:build integration

package trip_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	"github.com/ArtemIushin123/template/internal/model"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
)

func TestListStaleTrips(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	manager := transaction.New(pool, pgtest.QueryTimeout)
	const threshold = 75 * time.Second
	err := manager.Do(t.Context(), func(ctx context.Context) error {
		tx, _ := transaction.CheckTransaction(ctx)
		var now time.Time
		if err := tx.QueryRow(ctx, "SELECT now()").Scan(&now); err != nil {
			return err
		}
		fresh := now.Add(-time.Second)
		stale := now.Add(-2 * time.Minute)
		boundary := now.Add(-threshold)
		owned := make(map[uuid.UUID]bool)
		addTrip := func(age time.Duration, last *time.Time, status trip.Status) model.StaleTrip {
			t.Helper()
			tr := pgtest.NewTrip(t)
			tr.StartedAt = now.Add(-age)
			tr.LastPositionAt = last
			if status == trip.StatusCompleted {
				if err := tr.Finish(now); err != nil {
					t.Fatal(err)
				}
			}
			pgtest.DeleteTrips(t, pool, tr.ID)
			if err := repo.Create(ctx, tr); err != nil {
				t.Fatal(err)
			}
			owned[tr.ID] = true
			return model.StaleTrip{TripID: tr.ID, DriverID: tr.DriverID}
		}

		oldPosition := addTrip(10*time.Minute, &stale, trip.StatusActive)
		addTrip(10*time.Minute, &fresh, trip.StatusActive)
		neverReported := addTrip(5*time.Minute, nil, trip.StatusActive)
		addTrip(time.Second, nil, trip.StatusActive)
		addTrip(12*time.Minute, nil, trip.StatusCompleted)
		addTrip(10*time.Minute, &boundary, trip.StatusActive)
		want := []model.StaleTrip{neverReported, oldPosition}

		trips, err := repo.ListStaleTrips(ctx, threshold)
		if err != nil {
			return err
		}
		var got []model.StaleTrip
		for _, candidate := range trips {
			if owned[candidate.TripID] {
				got = append(got, candidate)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("stale trips in polling order = %v, want %v", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
