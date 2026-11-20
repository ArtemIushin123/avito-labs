//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type positionLister interface {
	ListPositions(ctx context.Context, id uuid.UUID) ([]trip.Position, error)
}
