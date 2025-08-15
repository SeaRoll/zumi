package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	springbootlike "github.com/SeaRoll/zumi/examples/spring-boot-like"
	"github.com/SeaRoll/zumi/examples/spring-boot-like/docs"
	"github.com/SeaRoll/zumi/queue"
	"github.com/SeaRoll/zumi/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	cfg, err := springbootlike.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		return
	}

	// Database initialization
	db, err := springbootlike.NewDatabase(ctx, cfg)
	if err != nil {
		slog.Error("Failed to create database", "error", err)
		return
	}
	defer db.Disconnect()

	// Connect to pubsub
	mq, err := queue.NewPubsubClient(cfg.Pubsub)
	if err != nil {
		slog.Error("Failed to connect to pubsub", "error", err)
		return
	}

	// Repository and service initialization
	repository := springbootlike.NewRepository()
	service := springbootlike.NewService(mq, db, repository)

	// API initialization
	api := springbootlike.NewAPI(service)
	api.InitAPI()
	docs.AddDocRoutes()

	// Start the server
	addr := ":8080"
	err = server.StartServer(ctx, addr)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
	}
}
