package server

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"net/http"
)

var (
	//go:embed banner.txt
	banner       string
	globalServer *server = newServer()
)

// MiddlewareFunc defines the type for middleware functions.
// The first middleware added is the most nested one.
func AddMiddleware(middleware MiddlewareFunc) {
	globalServer.addMiddleware(middleware)
}

// AddHandler registers a new HTTP handler for the specified path.
// Usage: AddHandler("GET /path", handlerFunction)
// Basically the same as http.HandleFunc but with a custom server instance.
func AddHandler(path string, handler http.HandlerFunc) {
	globalServer.addHandler(path, handler)
}

// StartServer initializes and starts the server with the given address.
// It will return once the server is stopped or an error occurs.
func StartServer(ctx context.Context, addr string) error {
	// print banner
	fmt.Println(banner)
	slog.Info("starting server", "address", addr)
	return globalServer.start(ctx, addr)
}

// ClearServer resets the global server instance.
// This is useful for testing purposes to ensure a clean state.
func ClearServer() {
	globalServer = newServer()
}
