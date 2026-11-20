//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type (
	tripRepository interface {
		GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
		Save(ctx context.Context, trip *trip.Trip) error
		AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *trip.Status, toStatus trip.Status, reason *string) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	tripEvents interface {
		TripCompleted(ctx context.Context, tr *trip.Trip)
	}
)
