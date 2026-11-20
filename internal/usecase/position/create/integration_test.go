//go:build integration

package create_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	"github.com/ArtemIushin123/template/internal/infra/postgres/transaction"
	"github.com/ArtemIushin123/template/internal/model"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
	"github.com/ArtemIushin123/template/internal/usecase/position/create"
)

type missingUpdateTarget struct{ *triprepo.Repository }

func (r missingUpdateTarget) AdvanceLastPositionAt(ctx context.Context, _ uuid.UUID, at time.Time) error {
	return r.Repository.AdvanceLastPositionAt(ctx, uuid.Nil, at)
}

func TestSavePositionTransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	tx := transaction.New(pool, pgtest.QueryTimeout)
	tr := pgtest.NewTrip(t)
	ctx := t.Context()
	pgtest.DeleteTrips(t, pool, tr.ID)
	if err := repo.Create(ctx, tr); err != nil {
		t.Fatal(err)
	}
	now := tr.StartedAt.Add(time.Hour)
	clock := func() time.Time { return now }
	events := NewMockpositionEvents(gomock.NewController(t))
	queue := NewMockpositionCanceller(gomock.NewController(t))
	queue.EXPECT().Cancel(tr.ID).Times(2)
	events.EXPECT().PositionSaved(ctx).Times(2)
	uc := create.New(repo, tx, events, queue, clock)
	fresh := now.Add(-time.Minute)
	in := model.CreatePositionInput{TripID: tr.ID, Coordinates: trip.Coordinate{Latitude: 20, Longitude: -3}, RecordedAt: fresh}
	first, err := uc.CreatePosition(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	in.RecordedAt = fresh.Add(-time.Minute)
	late, err := uc.CreatePosition(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == late.ID || !late.RecordedAt.Equal(in.RecordedAt) {
		t.Fatalf("unexpected positions: %+v, %+v", first, late)
	}
	in.RecordedAt = now
	broken := create.New(missingUpdateTarget{repo}, tx, events, queue, clock)
	_, err = broken.CreatePosition(ctx, in)
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("second write error = %v, want missing update target", err)
	}
	got, err := repo.GetByID(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastPositionAt == nil || !got.LastPositionAt.Equal(fresh) {
		t.Fatalf("last_position_at = %v, want %v", got.LastPositionAt, fresh)
	}
	var count, matching int
	err = pool.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE id IN ($2, $3)) FROM trip_positions WHERE trip_id=$1", tr.ID, first.ID, late.ID).Scan(&count, &matching)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 || matching != 2 {
		t.Fatalf("got %d points, %d expected IDs; want both committed points only", count, matching)
	}
}
