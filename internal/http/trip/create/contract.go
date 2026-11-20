//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/model"
)

type tripCreator interface {
	Create(ctx context.Context, in model.CreateTripInput) (*trip.Trip, error)
}
