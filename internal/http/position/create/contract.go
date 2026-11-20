//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/model"
)

type positionCreator interface {
	CreatePosition(ctx context.Context, in model.CreatePositionInput) (*trip.Position, error)
}
