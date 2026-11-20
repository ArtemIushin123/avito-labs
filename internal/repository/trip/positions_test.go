//go:build integration

package trip_test

import (
	"testing"
	"time"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/infra/postgres/pgtest"
	triprepo "github.com/ArtemIushin123/template/internal/repository/trip"
)

func TestListPositionsOrderAndTrip(t *testing.T) {
	pool := pgtest.Pool(t)
	repo := triprepo.New(pool, pgtest.QueryTimeout)
	ctx := t.Context()
	tr, other := pgtest.NewTrip(t), pgtest.NewTrip(t)
	pgtest.DeleteTrips(t, pool, tr.ID, other.ID)
	for _, item := range []*trip.Trip{tr, other} {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatal(err)
		}
	}
	at := tr.StartedAt
	points := []trip.Position{
		{TripID: tr.ID, Coordinates: trip.Coordinate{Latitude: 3, Longitude: 4}, RecordedAt: at.Add(time.Minute)},
		{TripID: tr.ID, Coordinates: trip.Coordinate{Latitude: 1, Longitude: 2}, RecordedAt: at},
		{TripID: other.ID, RecordedAt: at},
		{TripID: tr.ID, Coordinates: trip.Coordinate{Latitude: 5, Longitude: 6}, RecordedAt: at},
	}
	for i := range points {
		if err := repo.InsertPosition(ctx, &points[i]); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.ListPositions(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []trip.Position{points[1], points[3], points[0]}
	if len(got) != len(want) {
		t.Fatalf("got %d positions, want %d", len(got), len(want))
	}
	for i, p := range got {
		w := want[i]
		if p.ID != w.ID || p.TripID != tr.ID || p.Coordinates != w.Coordinates || !p.RecordedAt.Equal(w.RecordedAt) {
			t.Errorf("route[%d] = %+v, want %+v", i, p, w)
		}
	}
}
