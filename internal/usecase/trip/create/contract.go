//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type (
	tripRepository interface {
		Create(ctx context.Context, tr *trip.Trip) error
		AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus *trip.Status, toStatus trip.Status, reason *string) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	tripEvents interface {
		TripCreated(ctx context.Context, tr *trip.Trip)
	}
)
