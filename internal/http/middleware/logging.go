package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ArtemIushin123/template/internal/http/domainerror"
)

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ctx, requestErr := domainerror.WithErrorSlot(r.Context())
			rec := &statusRecorder{ResponseWriter: w}

			next.ServeHTTP(rec, r.WithContext(ctx))

			status := rec.code()
			level := slog.LevelInfo
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("route", routePattern(r)),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			}
			if status >= http.StatusInternalServerError {
				level = slog.LevelError
				if err := requestErr(); err != nil {
					attrs = append(attrs, slog.String("error", err.Error()))
				}
			}

			logger.LogAttrs(ctx, level, "request handled", attrs...)
		})
	}
}
