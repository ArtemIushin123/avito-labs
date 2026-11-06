package domainerror

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/ArtemIushin123/template/internal/domain/trip"
	api "github.com/ArtemIushin123/template/internal/generated"
)

const (
	typeInvalidRequest = "https://tripgo.example/problems/invalid-request"
	typeTripNotFound   = "https://tripgo.example/problems/trip-not-found"
	typeTripCompleted  = "https://tripgo.example/problems/trip-completed"
	typeDriverBusy     = "https://tripgo.example/problems/driver-busy"
	typeInternalError  = "https://tripgo.example/problems/internal-error"
)

func Problem(err error) (status int, body api.Problem) {
	var validationError *trip.ValidationError

	switch {
	case errors.As(err, &validationError):
		return invalidRequest(validationError.Field + ": " + validationError.Message)
	case errors.Is(err, trip.ErrTripNotFound):
		return problem(http.StatusNotFound, "trip_not_found", "Trip not found", typeTripNotFound, "trip not found")
	case errors.Is(err, trip.ErrTripCompleted):
		return problem(http.StatusConflict, "trip_completed", "Trip completed", typeTripCompleted, "trip already completed")
	case errors.Is(err, trip.ErrDriverBusy):
		return problem(http.StatusConflict, "driver_busy", "Driver busy", typeDriverBusy, "driver already has an active trip")
	default:
		return problem(http.StatusInternalServerError, "internal_error", "Internal error", typeInternalError, "")
	}
}

func Write(w http.ResponseWriter, r *http.Request, err error) {
	status, body := Problem(err)
	if status >= http.StatusInternalServerError {
		reportInternal(r.Context(), err, body.Code)
	}
	write(w, r, status, body)
}

type errorSlot struct{ err error }

type errorSlotKey struct{}

func WithErrorSlot(ctx context.Context) (context.Context, func() error) {
	slot := &errorSlot{}
	return context.WithValue(ctx, errorSlotKey{}, slot), func() error { return slot.err }
}

func reportInternal(ctx context.Context, err error, code string) {
	if slot, ok := ctx.Value(errorSlotKey{}).(*errorSlot); ok {
		slot.err = err
	}

	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, code)
	span.SetAttributes(attribute.String("error.type", code))
}

func WriteInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	status, body := invalidRequest(detail)
	write(w, r, status, body)
}

func invalidRequest(detail string) (int, api.Problem) {
	return problem(http.StatusBadRequest, "invalid_request", "Invalid request", typeInvalidRequest, detail)
}

func problem(status int32, code, title, problemType, detail string) (int, api.Problem) {
	body := api.Problem{
		Code:   code,
		Status: status,
		Title:  title,
		Type:   problemType,
	}
	if detail != "" {
		body.Detail = &detail
	}
	return int(status), body
}

func write(w http.ResponseWriter, r *http.Request, status int, body api.Problem) {
	instance := r.URL.Path
	body.Instance = &instance

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(body)
}
