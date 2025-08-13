package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	springbootlike "github.com/SeaRoll/zumi/examples/spring-boot-like"
	"github.com/SeaRoll/zumi/examples/spring-boot-like/books"
	"github.com/SeaRoll/zumi/examples/spring-boot-like/docs"
	"github.com/SeaRoll/zumi/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Database initialization
	db, err := springbootlike.NewDatabase(
		ctx,
		"postgres://postgres:mysecretpassword@localhost:5432/foodie?sslmode=disable",
	)
	if err != nil {
		slog.Error("Failed to create database", "error", err)
		return
	}
	defer db.Disconnect()

	// Repository and service initialization
	repository := books.NewRepository()
	service := books.NewService(db, repository)

	// API initialization
	api := books.NewAPI(service)
	api.InitAPI()
	docs.AddDocRoutes()

	// Start the server
	addr := ":8080"
	err = server.StartServer(ctx, addr)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
	}
}
