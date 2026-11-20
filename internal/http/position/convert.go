package position

import (
	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
)

func ToAPI(p trip.Position) api.TripPosition {
	return api.TripPosition{
		Id:          p.ID,
		TripId:      p.TripID,
		Coordinates: api.Coordinates(p.Coordinates),
		RecordedAt:  p.RecordedAt.UTC(),
	}
}
