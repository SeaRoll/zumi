// This example demonstrates how pubsub can also be achieved through using pubsub with cache.

package main

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"

	"github.com/SeaRoll/zumi/cache"
	"github.com/SeaRoll/zumi/server"
)

//go:generate go run ../../server/gen "-title=Zumi Message API" "-version=1.0.0" "-description=Zumi API for events with pubsub\n\nWe can also add spacing here" "-servers=http://localhost:8080"

const exampleChannel = "example_channel"

var (
	//go:embed openapi.yaml
	embedOpenAPI string
	//go:embed index.html
	embedIndexHTML string
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// setup cache
	cache, err := cache.NewCache(cache.CacheConfig{
		Host: "localhost",
		Port: "6379",
	})
	if err != nil {
		panic(fmt.Errorf("failed to create cache: %w", err))
	}

	// subscribe to a channel
	go func() {
		err := cache.Subscribe(exampleChannel, func(msg string) error {
			fmt.Printf("Received message: %s\n", msg)
			return nil
		})
		if err != nil {
			panic(fmt.Errorf("failed to subscribe to channel: %w", err))
		}
	}()

	// gen:ignore
	server.AddHandler("GET /health", func(w http.ResponseWriter, r *http.Request) {
		type healthResponse struct {
			Status string `json:"status"`
		}
		server.WriteJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})

	// gen:ignore
	server.AddHandler("GET /docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)

		_, err := w.Write([]byte(embedIndexHTML))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to write index.html: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// gen:ignore
	server.AddHandler("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)

		_, err := w.Write([]byte(embedOpenAPI))
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to write openapi.yaml: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// Endpoint to publish messages to the channel
	server.AddHandler("POST /publish", func(w http.ResponseWriter, r *http.Request) {
		type publishResponse struct {
			Status string `json:"status"`
		}

		var req struct {
			Ctx     context.Context `ctx:"context"`
			Message struct {
				Content string `json:"content" validate:"required"` // ensure content is provided
			} `body:"json"`
		}

		err := server.ParseRequest(r, &req)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
			return
		}

		err = cache.Publish(req.Ctx, exampleChannel, req.Message.Content)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to publish message: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusOK, publishResponse{Status: "message published"})
	})

	// start the server
	addr := ":8080"

	err = server.StartServer(ctx, addr)
	if err != nil {
		panic(fmt.Errorf("failed to start server: %w", err))
	}
}
