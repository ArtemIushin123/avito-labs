package router

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/ArtemIushin123/template/internal/http/domainerror"
	"github.com/ArtemIushin123/template/internal/http/health"
	"github.com/ArtemIushin123/template/internal/http/middleware"
	"github.com/ArtemIushin123/template/internal/http/trip/create"
	"github.com/ArtemIushin123/template/internal/http/trip/finish"
	"github.com/ArtemIushin123/template/internal/http/trip/get"
)

type server struct {
	createTrip *create.Handler
	getTrip    *get.Handler
	finishTrip *finish.Handler
	health     *health.Handler
}

var _ api.ServerInterface = (*server)(nil)

func (s *server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.createTrip.CreateTrip(w, r, params)
}

func (s *server) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	s.getTrip.GetTrip(w, r, tripId)
}

func (s *server) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	s.finishTrip.FinishTrip(w, r, tripId)
}

func (s *server) Health(w http.ResponseWriter, r *http.Request) {
	s.health.Health(w, r)
}

func (s *server) Ready(w http.ResponseWriter, r *http.Request) {
	s.health.Ready(w, r)
}

func New(
	createH *create.Handler,
	getH *get.Handler,
	finishH *finish.Handler,
	healthH *health.Handler,
	logger *slog.Logger,
	httpMetrics func(http.Handler) http.Handler,
) http.Handler {
	s := &server{
		createTrip: createH,
		getTrip:    getH,
		finishTrip: finishH,
		health:     healthH,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recover(logger))
	r.Use(middleware.Tracing())
	r.Use(middleware.Logging(logger))

	return api.HandlerWithOptions(s, api.ChiServerOptions{
		BaseRouter:       r,
		Middlewares:      []api.MiddlewareFunc{httpMetrics},
		ErrorHandlerFunc: paramError,
	})
}

func paramError(w http.ResponseWriter, r *http.Request, err error) {
	detail := "invalid request parameters"

	var paramErr *api.InvalidParamFormatError
	if errors.As(err, &paramErr) {
		detail = paramErr.ParamName + " has invalid format"
	}

	domainerror.WriteInvalidRequest(w, r, detail)
}
