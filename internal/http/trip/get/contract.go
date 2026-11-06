//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type tripGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}
