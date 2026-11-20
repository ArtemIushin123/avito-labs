package create_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/trip/create"
	"github.com/ArtemIushin123/template/internal/model"
)

const (
	codeInvalidRequest = "invalid_request"
	detailMalformed    = "malformed JSON"
	fieldPrice         = "price"
	contentTypeProblem = "application/problem+json"
	secret             = "password=s3cret host=10.0.0.5"

	validBody = `{
		"user_id": "22222222-2222-2222-2222-222222222222",
		"driver_id": "33333333-3333-3333-3333-333333333333",
		"start_point": {"latitude": 59.9398, "longitude": 30.3146},
		"end_point":   {"latitude": 59.9290, "longitude": 30.3626},
		"price": 1450
	}`
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func wantInput() model.CreateTripInput {
	return model.CreateTripInput{
		UserID:     uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		DriverID:   uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		Price:      1450,
		StartPoint: trip.Coordinate{Latitude: 59.9398, Longitude: 30.3146},
		EndPoint:   trip.Coordinate{Latitude: 59.9290, Longitude: 30.3626},
	}
}

func createdTrip() *trip.Trip {
	in := wantInput()
	return &trip.Trip{
		ID:         tripID,
		UserID:     in.UserID,
		DriverID:   in.DriverID,
		Price:      in.Price,
		Status:     trip.StatusActive,
		StartPoint: in.StartPoint,
		EndPoint:   in.EndPoint,
		StartedAt:  time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	}
}

func doCreate(t *testing.T, uc *MocktripCreator, body string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	create.New(uc).CreateTrip(w, r, api.CreateTripParams{})
	return w
}

func TestHandler_CreateTrip_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripCreator(ctrl)
	uc.EXPECT().Create(gomock.Any(), wantInput()).Return(createdTrip(), nil)

	w := doCreate(t, uc, validBody)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if loc := w.Header().Get("Location"); loc != "/api/v1/trips/"+tripID.String() {
		t.Errorf("Location = %q, want /api/v1/trips/%s", loc, tripID)
	}

	var got api.Trip
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode trip: %v", err)
	}
	want := createdTrip()
	if got.Id != want.ID || got.UserId != want.UserID || got.DriverId != want.DriverID ||
		got.Price != want.Price || string(got.Status) != string(want.Status) ||
		got.StartPoint != api.Coordinates(want.StartPoint) || got.EndPoint != api.Coordinates(want.EndPoint) ||
		!got.StartedAt.Equal(want.StartedAt) || got.FinishedAt != nil {
		t.Errorf("trip = %+v, want %+v", got, want)
	}
}

func TestHandler_CreateTrip_InvalidBody(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantDetail string
	}{
		{name: "malformed JSON", body: `{"user_id":`, wantDetail: detailMalformed},
		{name: "not an object", body: `[1, 2]`, wantDetail: "JSON object"},
		{name: "unknown top-level field", body: strings.Replace(validBody, `"price": 1450`, `"price": 1450, "id": "x"`, 1), wantDetail: `unknown field "id"`},
		{name: "unknown nested field", body: strings.Replace(validBody, `"longitude": 30.3146}`, `"longitude": 30.3146, "alt": 1}`, 1), wantDetail: `start_point: unknown field "alt"`},
		{name: "missing driver_id", body: strings.Replace(validBody, `"driver_id": "33333333-3333-3333-3333-333333333333",`, "", 1), wantDetail: "driver_id is required"},
		{name: "missing end_point", body: strings.Replace(validBody, `"end_point":   {"latitude": 59.9290, "longitude": 30.3626},`, "", 1), wantDetail: "end_point is required"},
		{name: "missing end_point.longitude", body: strings.Replace(validBody, `, "longitude": 30.3626}`, `}`, 1), wantDetail: "end_point.longitude is required"},
		{name: "price is a string", body: strings.Replace(validBody, `"price": 1450`, `"price": "1450"`, 1), wantDetail: fieldPrice},
		{name: "price is fractional", body: strings.Replace(validBody, `"price": 1450`, `"price": 1450.5`, 1), wantDetail: fieldPrice},
		{name: "driver_id is not a UUID", body: strings.Replace(validBody, `33333333-3333-3333-3333-333333333333`, `not-a-uuid`, 1), wantDetail: "invalid"},
		{name: "empty body", body: ``, wantDetail: detailMalformed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMocktripCreator(ctrl)

			w := doCreate(t, uc, tt.body)

			p := assertProblem(t, w, http.StatusBadRequest, codeInvalidRequest)
			if p.Detail == nil || !strings.Contains(*p.Detail, tt.wantDetail) {
				t.Errorf("detail = %v, want it to mention %q", p.Detail, tt.wantDetail)
			}
		})
	}
}

func TestHandler_CreateTrip_BodyTooLarge(t *testing.T) {
	ctrl := gomock.NewController(t)
	uc := NewMocktripCreator(ctrl)

	huge := `{"user_id": "` + strings.Repeat("a", 2<<20) + `"}`
	w := doCreate(t, uc, huge)

	assertProblem(t, w, http.StatusBadRequest, codeInvalidRequest)
}

func TestHandler_CreateTrip_UsecaseErrors(t *testing.T) {
	tests := []struct {
		name       string
		ucErr      error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "driver busy",
			ucErr:      fmt.Errorf("create trip: %w", trip.ErrDriverBusy),
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			name:       "domain validation",
			ucErr:      fmt.Errorf("create trip: %w", &trip.ValidationError{Field: "price", Message: "must not be negative"}),
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "repository error",
			ucErr:      fmt.Errorf("create trip: %w", errors.New(secret)),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			uc := NewMocktripCreator(ctrl)
			uc.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, tt.ucErr)

			w := doCreate(t, uc, validBody)

			assertProblem(t, w, tt.wantStatus, tt.wantCode)
			if w.Header().Get("Location") != "" {
				t.Errorf("Location = %q, must be absent on error", w.Header().Get("Location"))
			}
			if strings.Contains(w.Body.String(), "s3cret") || strings.Contains(w.Body.String(), "10.0.0.5") {
				t.Errorf("response leaks internal error details: %s", w.Body.String())
			}
		})
	}
}

func assertProblem(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) api.Problem {
	t.Helper()

	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body: %s", w.Code, wantStatus, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != contentTypeProblem {
		t.Fatalf("Content-Type = %q, want %s", ct, contentTypeProblem)
	}
	var p api.Problem
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if p.Code != wantCode {
		t.Errorf("code = %q, want %q", p.Code, wantCode)
	}
	return p
}
