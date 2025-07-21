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

func AddMiddleware(middleware MiddlewareFunc) {
	globalServer.addMiddleware(middleware)
}

func AddHandler(path string, handler http.HandlerFunc) {
	globalServer.addHandler(path, handler)
}

func StartServer(ctx context.Context, addr string) error {
	// print banner
	fmt.Println(banner)
	slog.Info("starting server", "address", addr)
	return globalServer.start(ctx, addr)
}
