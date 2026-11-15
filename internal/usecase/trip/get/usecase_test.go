package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/usecase/trip/get"
)

func TestUsecase_GetByID(t *testing.T) {
	tripID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	found := &trip.Trip{ID: tripID, Status: trip.StatusActive}
	errDB := errors.New("db is down")

	tests := []struct {
		name     string
		setup    func(repo *MocktripRepository)
		wantTrip *trip.Trip
		wantErr  error
	}{
		{
			name: "trip found",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().GetByID(gomock.Any(), tripID).Return(found, nil)
			},
			wantTrip: found,
			wantErr:  nil,
		},
		{
			name: "trip not found",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().GetByID(gomock.Any(), tripID).Return(nil, trip.ErrTripNotFound)
			},
			wantTrip: nil,
			wantErr:  trip.ErrTripNotFound,
		},
		{
			name: "repository error",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().GetByID(gomock.Any(), tripID).Return(nil, errDB)
			},
			wantTrip: nil,
			wantErr:  errDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tt.setup(repo)

			uc := get.New(repo)

			got, err := uc.GetByID(context.Background(), tripID)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("GetByID() error = %v, want %v", err, tt.wantErr)
			}
			if got != tt.wantTrip {
				t.Errorf("GetByID() trip = %+v, want %+v", got, tt.wantTrip)
			}
		})
	}
}
