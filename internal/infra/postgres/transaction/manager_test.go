//go:build integration

package transaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
)

func TestManager_RollbackOnSecondInsertError(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	txm := transaction.New(pool)
	tr := pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, tr.ID)

	reason := "test rollback"

	err := txm.Do(context.Background(), func(ctx context.Context) error {
		if err := repo.Create(ctx, tr); err != nil {
			return err
		}

		return repo.AddStatusHistory(ctx, tr.ID, nil, trip.Status("bogus"), &reason)
	})

	if err == nil {
		t.Fatal("Do() error = nil, want error from the second insert")
	}

	_, err = repo.GetByID(context.Background(), tr.ID)
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("GetByID() after rollback error = %v, want %v: trip stayed in the database, rollback did not work",
			err, trip.ErrTripNotFound)
	}
}
