package list_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/usecase/position/list"
)

func TestListPositions(t *testing.T) {
	id := uuid.New()
	ctx := t.Context()
	t.Run("completed trip", func(t *testing.T) {
		repo := NewMocktripRepository(gomock.NewController(t))
		repo.EXPECT().GetByID(ctx, id).Return(&trip.Trip{ID: id, Status: trip.StatusCompleted}, nil)
		repo.EXPECT().ListPositions(ctx, id).Return([]trip.Position{{ID: 42, TripID: id}}, nil)
		got, err := list.New(repo).ListPositions(ctx, id)
		if err != nil || len(got) != 1 || got[0].ID != 42 {
			t.Fatalf("positions = %+v, err = %v", got, err)
		}
	})
	t.Run("missing trip", func(t *testing.T) {
		repo := NewMocktripRepository(gomock.NewController(t))
		repo.EXPECT().GetByID(ctx, id).Return(nil, trip.ErrTripNotFound)
		_, err := list.New(repo).ListPositions(ctx, id)
		if !errors.Is(err, trip.ErrTripNotFound) {
			t.Fatalf("got %v, want ErrTripNotFound", err)
		}
	})
	t.Run("database read failure", func(t *testing.T) {
		repo := NewMocktripRepository(gomock.NewController(t))
		readErr := errors.New("read failed")
		repo.EXPECT().GetByID(ctx, id).Return(&trip.Trip{ID: id}, nil)
		repo.EXPECT().ListPositions(ctx, id).Return(nil, readErr)
		_, err := list.New(repo).ListPositions(ctx, id)
		if !errors.Is(err, readErr) {
			t.Fatalf("got %v, want %v", err, readErr)
		}
	})
}
