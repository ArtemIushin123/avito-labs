package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/ArtemIushin123/template/internal/http/domainerror"
)

type panicError struct {
	message string
	stack   string
}

func (e *panicError) Error() string {
	return e.message
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec)
			}

			domainerror.Write(w, r, &panicError{
				message: fmt.Sprintf("panic: %v", rec),
				stack:   string(debug.Stack()),
			})
		}()

		next.ServeHTTP(w, r)
	})
}
