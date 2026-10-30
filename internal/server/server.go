package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/ArtemIushin123/template/internal/config"
	api "github.com/ArtemIushin123/template/internal/generated"
	"github.com/go-chi/chi/v5"
)

func NewServer(h *Handler, c *config.Config) *http.Server {
	router := chi.NewRouter()
	router.Use(recoverer)
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
	detail := "invalid request parameters"

	var paramErr *api.InvalidParamFormatError
	if errors.As(err, &paramErr) {
		detail = paramErr.ParamName + " has invalid format"
	}

	SolveProblem(w, r, "invalid_request", detail,
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

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()

			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}
			fmt.Println("panic recovered:", rec, "\n", string(debug.Stack()))
			SolveProblem(w, r, "internal_error", "", http.StatusInternalServerError,
				"Internal error", "https://tripgo.example/problems/internal-error")
		}()

		next.ServeHTTP(w, r)
	})
}
