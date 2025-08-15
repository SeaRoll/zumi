package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

//go:embed banner.txt
var banner string

type MiddlewareFunc func(http.Handler) http.Handler

type server struct {
	mux         *http.ServeMux
	middlewares []MiddlewareFunc
}

func newServer() *server {
	mux := http.NewServeMux()
	return &server{mux: mux}
}

// Starts a running server by the given address.
// Stops the server when it receives ctx.Done().
func (s *server) start(ctx context.Context, addr string) error {
	fmt.Println(banner)
	slog.Info("starting server", "address", addr)

	var handler http.Handler = s.mux
	// wrap by decorating the mux with all middlewares, the first one is the outermost
	// and the last one is the innermost, due to that, we need to reverse the order
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		handler = s.middlewares[i](handler)
	}

	// finally add recovery and accesslog middlewares
	handler = accesslog(recovery(handler))

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
	}

	// Create a channel to listen for errors from the server
	errChan := make(chan error, 1)

	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for either an error or the context to be done
	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}

		return nil
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		err := server.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
	}

	return nil
}

// AddMiddleware adds a middleware function to the server.
func (s *server) addMiddleware(middleware MiddlewareFunc) {
	s.middlewares = append(s.middlewares, middleware)
}

// AddHandler registers a new handler for the specified path.
func (s *server) addHandler(path string, handler http.HandlerFunc) {
	s.mux.HandleFunc(path, handler)
}

// WriteJSON writes a JSON response to the http.ResponseWriter.
// This should be the last step in your handler function, since
// it sets the Content-Type header to application/json, and can transform writer to send errors
// if the encoding fails.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to write JSON response: %v", err))
	}
}

// WriteError writes an error message to the http.ResponseWriter with the specified status code.
// This is a utility function to handle errors in a consistent way across your handlers.
func WriteError(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(map[string]string{"error": message})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to write error response: %v", err), http.StatusInternalServerError)
	}
}
