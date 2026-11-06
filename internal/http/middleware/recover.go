package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/ArtemIushin123/template/internal/http/domainerror"
)

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				logger.ErrorContext(r.Context(), "panic recovered",
					"panic", fmt.Sprint(rec),
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				domainerror.Write(w, r, fmt.Errorf("panic: %v", rec))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
