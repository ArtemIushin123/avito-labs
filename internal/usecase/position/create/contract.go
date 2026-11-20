//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type tripRepository interface {
	GetByIDForUpdate(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
	InsertPosition(ctx context.Context, p *trip.Position) error
	AdvanceLastPositionAt(ctx context.Context, id uuid.UUID, at time.Time) error
}
type txManager interface {
	Do(ctx context.Context, fn func(context.Context) error) error
}
type positionCanceller interface {
	Cancel(tripID uuid.UUID)
}

type positionEvents interface {
	PositionSaved(ctx context.Context)
}
