//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}

package positions

import (
	"context"
	"time"

	"github.com/ArtemIushin123/template/internal/model"
)

type staleTripSource interface {
	ListStaleTrips(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error)
}

type ticker interface {
	Ticks() <-chan time.Time
	Stop()
}

type queueEvents interface {
	Enqueued(ctx context.Context, result string)
	QueueSize(ctx context.Context, size int64)
	Cancelled(ctx context.Context)
}

type workerEvents interface {
	WorkersActive(delta int64)
}

type pushGateway interface {
	RequestPosition(ctx context.Context, trip model.StaleTrip) error
}
