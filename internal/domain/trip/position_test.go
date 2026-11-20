package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ArtemIushin123/template/internal/domain/trip"
)

func TestNewPosition(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		lat, lon float64
		offset   time.Duration
		badField string
	}{
		{"exact minute and boundaries", -90, 180, time.Minute, ""},
		{"late point", 90, -180, -time.Hour, ""},
		{"future beyond minute", 0, 0, time.Minute + time.Nanosecond, "recorded_at"},
		{"invalid latitude", 91, 0, 0, "coordinates"},
		{"invalid longitude", 0, -181, 0, "coordinates"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := trip.NewPosition(uuid.New(), trip.Coordinate{Latitude: tc.lat, Longitude: tc.lon}, now.Add(tc.offset), now)
			if tc.badField == "" {
				if err != nil || p == nil {
					t.Fatalf("NewPosition: %v", err)
				}
				return
			}
			var v *trip.ValidationError
			if p != nil || !errors.As(err, &v) || v.Field != tc.badField {
				t.Fatalf("error = %v, want invalid %s", err, tc.badField)
			}
		})
	}
}
