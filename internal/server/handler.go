package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/repository"
	"github.com/ArtemIushin123/template/internal/transaction"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	api.Unimplemented
	pool             *pgxpool.Pool
	dbConnectTimeout time.Duration
	dbQueryTimeout   time.Duration
	repo             *repository.TripRepository
	manager          transaction.TxManager
}

func NewHandler(
	pool *pgxpool.Pool,
	dbConnectTimeout, dbQueryTimeout time.Duration,
	repo *repository.TripRepository,
	manager transaction.TxManager,
) *Handler {
	return &Handler{
		pool:             pool,
		dbConnectTimeout: dbConnectTimeout,
		dbQueryTimeout:   dbQueryTimeout,
		repo:             repo,
		manager:          manager,
	}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response := api.HealthResponse{Status: api.Ok}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.dbConnectTimeout)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		response := api.HealthResponse{Status: api.Unavailable}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	response := api.HealthResponse{Status: api.Ok}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	ctx, cancel := context.WithTimeout(r.Context(), h.dbQueryTimeout)
	defer cancel()

	trip, err := h.repo.GetByID(ctx, tripId)
	if err != nil {
		if errors.Is(err, repository.ErrTripNotFound) {
			SolveProblem(w, r, "trip_not_found", "trip not found", http.StatusNotFound,
				"Trip not found", "https://tripgo.example/problems/trip-not-found")
			return
		}

		fmt.Println("get trip:", err)
		SolveProblem(w, r, "internal_error", "", http.StatusInternalServerError,
			"Internal error", "https://tripgo.example/problems/internal-error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(trip)
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	ctx, cancel := context.WithTimeout(r.Context(), h.dbQueryTimeout)
	defer cancel()

	var trip *api.Trip
	err := h.manager.Do(ctx, func(txCtx context.Context) error {
		finished, err := h.repo.Finish(txCtx, tripId)
		if err != nil {
			return err
		}
		trip = finished

		fromStatus := string(api.Active)
		return h.repo.AddStatusHistory(txCtx, tripId, &fromStatus, string(api.Completed), nil)
	})

	if errors.Is(err, repository.ErrTripNotFound) {
		SolveProblem(w, r, "trip_not_found", "trip not found", http.StatusNotFound,
			"Trip not found", "https://tripgo.example/problems/trip-not-found")
		return
	}
	if errors.Is(err, repository.ErrTripCompleted) {
		SolveProblem(w, r, "trip_completed", "trip already completed", http.StatusConflict,
			"Trip completed", "https://tripgo.example/problems/trip-completed")
		return
	}
	if err != nil {
		fmt.Println("finish trip:", err)
		SolveProblem(w, r, "internal_error", "", http.StatusInternalServerError,
			"Internal error", "https://tripgo.example/problems/internal-error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(trip)
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		SolveProblem(w, r, "invalid_request", "", http.StatusBadRequest,
			"Invalid request", "https://tripgo.example/problems/invalid-request")
		return
	}

	req, err := parseCreateTripRequest(body)
	if err != nil {
		SolveProblem(w, r, "invalid_request", err.Error(), http.StatusBadRequest,
			"Invalid request", "https://tripgo.example/problems/invalid-request")
		return
	}

	tripData := toTripData(*req)

	if msg := validateTripData(tripData); msg != "" {
		SolveProblem(w, r, "invalid_request", msg, http.StatusBadRequest,
			"Invalid request", "https://tripgo.example/problems/invalid-request")
		return
	}

	trip := api.Trip{
		Id:         uuid.New(),
		UserId:     tripData.UserId,
		DriverId:   tripData.DriverId,
		StartPoint: tripData.StartPoint,
		EndPoint:   tripData.EndPoint,
		Price:      tripData.Price,
		Status:     api.Active,
		StartedAt:  time.Now().UTC(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.dbQueryTimeout)
	defer cancel()

	err = h.manager.Do(ctx, func(txCtx context.Context) error {
		if err := h.repo.Create(txCtx, trip); err != nil {
			return err
		}
		return h.repo.AddStatusHistory(txCtx, trip.Id, nil, string(api.Active), nil)
	})

	if errors.Is(err, repository.ErrDriverBusy) {
		SolveProblem(w, r, "driver_busy", "driver already has an active trip", http.StatusConflict,
			"Driver busy", "https://tripgo.example/problems/driver-busy")
		return
	}
	if err != nil {
		fmt.Println("create trip:", err)
		SolveProblem(w, r, "internal_error", "", http.StatusInternalServerError,
			"Internal error", "https://tripgo.example/problems/internal-error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/trips/"+trip.Id.String())
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(trip)
}

func validateTripData(d api.TripData) string {
	if d.UserId == uuid.Nil {
		return "user_id must not be empty"
	}
	if d.DriverId == uuid.Nil {
		return "driver_id must not be empty"
	}

	if !validCoordinates(d.StartPoint) {
		return "start_point is out of range"
	}
	if !validCoordinates(d.EndPoint) {
		return "end_point is out of range"
	}

	if d.Price < 0 {
		return "price must be >= 0"
	}

	return ""
}

func validCoordinates(c api.Coordinates) bool {
	return c.Latitude >= -90 && c.Latitude <= 90 &&
		c.Longitude >= -180 && c.Longitude <= 180
}

type coordinateRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type tripRequest struct {
	DriverID   *uuid.UUID         `json:"driver_id"`
	UserID     *uuid.UUID         `json:"user_id"`
	StartPoint *coordinateRequest `json:"start_point"`
	EndPoint   *coordinateRequest `json:"end_point"`
	Price      *int64             `json:"price"`
}

var coordinateKeys = map[string]bool{"latitude": true, "longitude": true}
var tripKeys = map[string]bool{"driver_id": true, "user_id": true, "start_point": true,
	"end_point": true, "price": true}

func checkKeys(raw []byte, allowed map[string]bool) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage

	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	for k := range m {
		if !allowed[k] {
			return nil, fmt.Errorf("unknown field %q", k)
		}
	}
	return m, nil
}

func parseCreateTripRequest(body []byte) (*tripRequest, error) {
	top, err := checkKeys(body, tripKeys)
	if err != nil {
		return nil, err
	}

	if raw, ok := top["start_point"]; ok {
		if _, err := checkKeys(raw, coordinateKeys); err != nil {
			return nil, fmt.Errorf("start_point: %w", err)
		}
	}

	if raw, ok := top["end_point"]; ok {
		if _, err := checkKeys(raw, coordinateKeys); err != nil {
			return nil, fmt.Errorf("end_point: %w", err)
		}
	}

	var req tripRequest
	if err := json.Unmarshal(body, &req); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return nil, fmt.Errorf("%s has invalid value", typeErr.Field)
		}
		return nil, errors.New("invalid field value")
	}

	if req.DriverID == nil {
		return nil, errors.New("driver_id is required")
	}
	if req.UserID == nil {
		return nil, errors.New("user_id is required")
	}
	if req.Price == nil {
		return nil, errors.New("price is required")
	}
	if req.StartPoint == nil {
		return nil, errors.New("start_point is required")
	}
	if req.StartPoint.Latitude == nil {
		return nil, errors.New("start_point.latitude is required")
	}
	if req.StartPoint.Longitude == nil {
		return nil, errors.New("start_point.longitude is required")
	}
	if req.EndPoint == nil {
		return nil, errors.New("end_point is required")
	}
	if req.EndPoint.Latitude == nil {
		return nil, errors.New("end_point.latitude is required")
	}
	if req.EndPoint.Longitude == nil {
		return nil, errors.New("end_point.longitude is required")
	}
	return &req, nil
}

func toTripData(req tripRequest) api.TripData {
	tripData := api.TripData{
		DriverId:   *req.DriverID,
		EndPoint:   api.Coordinates{Latitude: *req.EndPoint.Latitude, Longitude: *req.EndPoint.Longitude},
		Price:      *req.Price,
		StartPoint: api.Coordinates{Latitude: *req.StartPoint.Latitude, Longitude: *req.StartPoint.Longitude},
		UserId:     *req.UserID,
	}
	return tripData
}
