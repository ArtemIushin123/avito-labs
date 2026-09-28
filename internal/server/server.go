package server

import (
	"encoding/json"
	"net/http"

	"github.com/ArtemIushin123/template/internal/config"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewServer(h *Handler, c *config.Config) *http.Server {
	router := chi.NewRouter()
	router.Use(middleware.Recoverer)
	handler := api.HandlerWithOptions(h, api.ChiServerOptions{
		BaseRouter:       router,
		ErrorHandlerFunc: ErrorHandlerFunc,
	})
	server := &http.Server{
		Addr:              c.HTTPAddr,
		Handler:           handler,
		ReadTimeout:       c.HTTPReadTimeout,
		ReadHeaderTimeout: c.HTTPReadHeaderTimeout,
		WriteTimeout:      c.HTTPWriteTimeout,
		IdleTimeout:       c.HTTPIdleTimeout,
	}
	return server
}

func ErrorHandlerFunc(w http.ResponseWriter, r *http.Request, err error) {
	SolveProblem(w, r, "invalid_request", err.Error(),
		http.StatusBadRequest, "Invalid request", "https://tripgo.example/problems/invalid-request",
	)
}

func SolveProblem(w http.ResponseWriter, r *http.Request, code string, detail string, status int, title string, errorType string) {
	instance := r.URL.Path
	response := api.Problem{
		Code:     code,
		Instance: &instance,
		Status:   int32(status),
		Title:    title,
		Type:     errorType,
	}
	if detail != "" {
		response.Detail = &detail
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
