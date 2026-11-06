package finish

import (
	"time"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	"github.com/ArtemIushin123/template/internal/generated"
)

func toAPI(tr *trip.Trip) api.Trip {
	return api.Trip{
		Id:             tr.ID,
		UserId:         tr.UserID,
		DriverId:       tr.DriverID,
		StartPoint:     api.Coordinates(tr.StartPoint),
		EndPoint:       api.Coordinates(tr.EndPoint),
		Price:          tr.Price,
		Status:         api.TripStatus(tr.Status),
		StartedAt:      tr.StartedAt.UTC(),
		FinishedAt:     utcOrNil(tr.FinishedAt),
		LastPositionAt: nil,
	}
}

func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
