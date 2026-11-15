package trip

import (
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type tripRow struct {
	id             uuid.UUID
	userID         uuid.UUID
	driverID       uuid.UUID
	startLatitude  float64
	startLongitude float64
	endLatitude    float64
	endLongitude   float64
	price          int64
	status         string
	startedAt      time.Time
	finishedAt     *time.Time
}

var tripColumns = []string{
	"id",
	"user_id",
	"driver_id",
	"start_latitude",
	"start_longitude",
	"end_latitude",
	"end_longitude",
	"price",
	"status",
	"started_at",
	"finished_at",
}

func (r *tripRow) scanTargets() []any {
	return []any{
		&r.id,
		&r.userID,
		&r.driverID,
		&r.startLatitude,
		&r.startLongitude,
		&r.endLatitude,
		&r.endLongitude,
		&r.price,
		&r.status,
		&r.startedAt,
		&r.finishedAt,
	}
}

func (r tripRow) values() []any {
	return []any{
		r.id,
		r.userID,
		r.driverID,
		r.startLatitude,
		r.startLongitude,
		r.endLatitude,
		r.endLongitude,
		r.price,
		r.status,
		r.startedAt,
		r.finishedAt,
	}
}

func toDomain(row tripRow) *trip.Trip {
	return &trip.Trip{
		ID:       row.id,
		DriverID: row.driverID,
		UserID:   row.userID,
		Price:    row.price,
		Status:   trip.Status(row.status),
		StartPoint: trip.Coordinate{
			Latitude:  row.startLatitude,
			Longitude: row.startLongitude,
		},
		EndPoint: trip.Coordinate{
			Latitude:  row.endLatitude,
			Longitude: row.endLongitude,
		},
		StartedAt:  row.startedAt,
		FinishedAt: row.finishedAt,
	}
}

func fromDomain(tr *trip.Trip) *tripRow {
	return &tripRow{
		id:             tr.ID,
		userID:         tr.UserID,
		driverID:       tr.DriverID,
		startLatitude:  tr.StartPoint.Latitude,
		startLongitude: tr.StartPoint.Longitude,
		endLatitude:    tr.EndPoint.Latitude,
		endLongitude:   tr.EndPoint.Longitude,
		price:          tr.Price,
		status:         string(tr.Status),
		startedAt:      tr.StartedAt,
		finishedAt:     tr.FinishedAt,
	}
}
