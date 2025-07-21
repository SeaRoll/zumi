package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

// accesslog is a middleware that logs request and response details,
// including latency, method, path, query parameters, IP address, response status, and bytes sent.
func accesslog(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wr := responseRecorder{ResponseWriter: w}

		next.ServeHTTP(&wr, r)

		log.InfoContext(r.Context(), "accessed",
			slog.String("latency", time.Since(start).String()),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("query", r.URL.RawQuery),
			slog.String("ip", r.RemoteAddr),
			slog.Int("status", wr.status),
			slog.Int("bytes", wr.numBytes))
	})
}

// recovery is a middleware that recovers from panics during HTTP handler execution and logs the error details.
// It must be the last middleware in the chain to ensure it captures all panics.
func recovery(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := responseRecorder{ResponseWriter: w}
		defer func() {
			err := recover()
			if err == nil {
				return
			}

			if err, ok := err.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				// Handle the abort gracefully
				return
			}

			stack := make([]byte, 1024)
			n := runtime.Stack(stack, true)

			log.ErrorContext(r.Context(), "panic!",
				slog.Any("error", err),
				slog.String("stack", string(stack[:n])),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("query", r.URL.RawQuery),
				slog.String("ip", r.RemoteAddr))

			if wr.status > 0 {
				// response was already sent, nothing we can do
				return
			}

			// send error response
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		}()
		next.ServeHTTP(&wr, r)
	})
}

// responseRecorder is a wrapper around [http.ResponseWriter] that records the status and bytes written during the response.
// It implements the [http.ResponseWriter] interface by embedding the original ResponseWriter.
type responseRecorder struct {
	http.ResponseWriter
	status   int
	numBytes int
}

// Header implements the [http.ResponseWriter] interface.
func (re *responseRecorder) Header() http.Header {
	return re.ResponseWriter.Header()
}

// Write implements the [http.ResponseWriter] interface.
func (re *responseRecorder) Write(b []byte) (int, error) {
	re.numBytes += len(b)
	return re.ResponseWriter.Write(b)
}

// WriteHeader implements the [http.ResponseWriter] interface.
func (re *responseRecorder) WriteHeader(statusCode int) {
	re.status = statusCode
	re.ResponseWriter.WriteHeader(statusCode)
}
