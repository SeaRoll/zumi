package server

import (
	"context"
	"net/http"
)

var globalServer Server = NewServer()

func AddMiddleware(middleware MiddlewareFunc) {
	globalServer.AddMiddleware(middleware)
}

func AddHandler(path string, handler http.HandlerFunc) {
	globalServer.AddHandler(path, handler)
}

func StartServer(ctx context.Context, addr string) error {
	return globalServer.Start(ctx, addr)
}
