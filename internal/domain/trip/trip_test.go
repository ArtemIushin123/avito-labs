package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

type newTripInput struct {
	id, driverID, userID uuid.UUID
	price                int64
	start, end           trip.Coordinate
	startedAt            time.Time
}

func validInput() newTripInput {
	return newTripInput{
		id:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		driverID:  uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		userID:    uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		price:     1450,
		start:     trip.Coordinate{Latitude: 59.9398, Longitude: 30.3146},
		end:       trip.Coordinate{Latitude: 59.9290, Longitude: 30.3626},
		startedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	}
}

const fieldStartPoint = "start_point"

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(in *newTripInput)
		wantField string
	}{
		{name: "valid input"},

		{name: "empty driver id", modify: func(in *newTripInput) { in.driverID = uuid.Nil }, wantField: "driver_id"},
		{name: "empty user id", modify: func(in *newTripInput) { in.userID = uuid.Nil }, wantField: "user_id"},
		{name: "negative price", modify: func(in *newTripInput) { in.price = -1 }, wantField: "price"},
		{name: "start latitude above 90", modify: func(in *newTripInput) { in.start.Latitude = 90.0001 }, wantField: fieldStartPoint},
		{name: "start latitude below -90", modify: func(in *newTripInput) { in.start.Latitude = -90.0001 }, wantField: fieldStartPoint},
		{name: "start longitude above 180", modify: func(in *newTripInput) { in.start.Longitude = 180.0001 }, wantField: fieldStartPoint},
		{name: "start longitude below -180", modify: func(in *newTripInput) { in.start.Longitude = -180.0001 }, wantField: fieldStartPoint},
		{name: "end latitude above 90", modify: func(in *newTripInput) { in.end.Latitude = 90.0001 }, wantField: "end_point"},
		{name: "end longitude below -180", modify: func(in *newTripInput) { in.end.Longitude = -180.0001 }, wantField: "end_point"},

		{name: "zero price is valid", modify: func(in *newTripInput) { in.price = 0 }},
		{name: "latitude 90 is valid", modify: func(in *newTripInput) { in.start.Latitude = 90 }},
		{name: "latitude -90 is valid", modify: func(in *newTripInput) { in.end.Latitude = -90 }},
		{name: "longitude 180 is valid", modify: func(in *newTripInput) { in.start.Longitude = 180 }},
		{name: "longitude -180 is valid", modify: func(in *newTripInput) { in.end.Longitude = -180 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			if tt.modify != nil {
				tt.modify(&in)
			}

			got, err := trip.New(in.id, in.driverID, in.userID, in.price, in.start, in.end, in.startedAt)

			if tt.wantField != "" {
				var vErr *trip.ValidationError
				if !errors.As(err, &vErr) {
					t.Fatalf("New() error = %v, want *trip.ValidationError", err)
				}
				if vErr.Field != tt.wantField {
					t.Errorf("ValidationError.Field = %q, want %q", vErr.Field, tt.wantField)
				}
				if got != nil {
					t.Errorf("New() returned trip %+v, want nil on error", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("New() unexpected error: %v", err)
			}
			want := trip.Trip{
				ID:         in.id,
				DriverID:   in.driverID,
				UserID:     in.userID,
				Price:      in.price,
				Status:     trip.StatusActive,
				StartPoint: in.start,
				EndPoint:   in.end,
				StartedAt:  in.startedAt,
				FinishedAt: nil,
			}
			if *got != want {
				t.Errorf("New() =\n  %+v\nwant\n  %+v", *got, want)
			}
		})
	}
}

func TestTrip_Finish(t *testing.T) {
	earlier := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	at := time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)

	tests := []struct {
		name           string
		status         trip.Status
		finishedAt     *time.Time
		wantErr        error
		wantStatus     trip.Status
		wantFinishedAt time.Time
	}{
		{
			name:           "active trip becomes completed",
			status:         trip.StatusActive,
			finishedAt:     nil,
			wantErr:        nil,
			wantStatus:     trip.StatusCompleted,
			wantFinishedAt: at,
		},
		{
			name:           "completed trip is not changed",
			status:         trip.StatusCompleted,
			finishedAt:     &earlier,
			wantErr:        trip.ErrTripCompleted,
			wantStatus:     trip.StatusCompleted,
			wantFinishedAt: earlier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := &trip.Trip{Status: tt.status, FinishedAt: tt.finishedAt}

			err := tr.Finish(at)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Finish() error = %v, want %v", err, tt.wantErr)
			}
			if tr.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", tr.Status, tt.wantStatus)
			}
			if tr.FinishedAt == nil {
				t.Fatalf("FinishedAt = nil, want %v", tt.wantFinishedAt)
			}
			if !tr.FinishedAt.Equal(tt.wantFinishedAt) {
				t.Errorf("FinishedAt = %v, want %v", *tr.FinishedAt, tt.wantFinishedAt)
			}
		})
	}
}
