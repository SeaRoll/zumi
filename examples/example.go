package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/SeaRoll/zumi/cache"
	"github.com/SeaRoll/zumi/database"
	"github.com/SeaRoll/zumi/queue"
	"github.com/SeaRoll/zumi/server"
)

type Book struct {
	ID          int    `db:"id" json:"id"`
	Title       string `db:"title" json:"title"`
	Description string `db:"description" json:"description"`
}

//go:embed migrations/*.sql
var embedMigrations embed.FS

func main() {
	ctx := context.Background()

	// Creating the database
	db, err := database.NewDatabase(
		ctx,
		"postgres://postgres:mysecretpassword@localhost:5432/foodie?sslmode=disable",
		embedMigrations,
	)
	if err != nil {
		panic(fmt.Errorf("failed to create database: %w", err))
	}
	defer db.Disconnect()

	// Creating the cache
	cache, err := cache.NewCache(cache.CacheConfig{
		Host:     "localhost",
		Port:     "6379",
		Password: "",
	})
	if err != nil {
		panic(fmt.Errorf("failed to connect to redis: %w", err))
	}

	// Connect to pubsub
	mq, err := queue.NewPubsubClient(queue.NewPubsubClientParams{
		ConnectionUrl: "nats://localhost:4222",
		Name:          "default",
		TopicPrefix:   "events",
		MaxAge:        24 * time.Hour,
	})
	if err != nil {
		panic(fmt.Errorf("failed to connect to pubsub: %w", err))
	}

	go func() {
		if err := mq.Consume(queue.ConsumerConfig{
			ConsumerName: "api",
			Subject:      "events.books",
			FetchLimit:   1,
			Callback: func(ctx context.Context, events []queue.Event) []int {
				successMsgs := []int{}
				for _, event := range events {
					var book Book
					if err := json.Unmarshal(event.Payload, &book); err != nil {
						slog.Error("Failed to unmarshal book event", "error", err)
						continue
					}
					slog.Info("Book event received", "book", book)
					successMsgs = append(successMsgs, event.Index)
				}
				return successMsgs
			},
		}); err != nil {
			slog.Error("Failed to consume messages", "error", err)
		}
	}()

	// Do some operations on cache
	if err := cache.Set(ctx, "something", "hello", 5*time.Minute); err != nil {
		panic(fmt.Errorf("failed to insert to cache: %w", err))
	}

	var cacheValue string
	if err := cache.Get(ctx, "something", &cacheValue); err != nil {
		panic(fmt.Errorf("failed to get to cache: %w", err))
	}

	slog.Info("received cache value", "key", "something", "value", cacheValue)

	srv := server.NewServer()

	// Get all books handler
	srv.AddHandler("GET /api/v1/books", func(w http.ResponseWriter, r *http.Request) {
		var books []Book
		if err := db.WithTX(ctx, func(tx database.DBTX) error {
			var err error
			books, err = database.SelectRows[Book](ctx, tx, "SELECT * FROM books")
			if err != nil {
				return fmt.Errorf("failed to retrieve all books: %w", err)
			}
			return nil
		}); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve books: %v", err))
			return
		}

		server.WriteJSON(w, books)
	})

	// Get a book by ID handler
	srv.AddHandler("GET /api/v1/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		idInt, err := strconv.Atoi(id)
		if err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid book ID: %v", id))
			return
		}

		var book Book
		if err := db.WithTX(ctx, func(tx database.DBTX) error {
			var err error
			book, err = database.SelectRow[Book](ctx, tx, "SELECT * FROM books WHERE id = $1", idInt)
			if err != nil {
				return fmt.Errorf("failed to retrieve book: %w", err)
			}
			return nil
		}); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve book: %v", err))
			return
		}

		server.WriteJSON(w, book)
	})

	// Add a book handler
	srv.AddHandler("POST /api/v1/books", func(w http.ResponseWriter, r *http.Request) {
		var book Book
		if err := json.NewDecoder(r.Body).Decode(&book); err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to decode request body: %v", err))
			return
		}

		if err := db.WithTX(ctx, func(tx database.DBTX) error {
			if err := database.ExecQuery(
				ctx,
				tx,
				"INSERT INTO books (title, description) VALUES ($1, $2)",
				book.Title,
				book.Description,
			); err != nil {
				return fmt.Errorf("failed to insert book: %w", err)
			}
			return nil
		}); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to insert book: %v", err))
			return
		}

		// Publish the book event to the queue
		bookEvent, err := json.Marshal(book)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal book event: %v", err))
			return
		}
		if err := mq.Publish("events.books", bookEvent); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to publish book event: %v", err))
			return
		}

		server.WriteJSON(w, map[string]string{"status": "book added"})
	})

	// Start the server
	addr := ":8080"
	slog.Info("Starting server", "address", addr)
	if err := srv.Start(ctx, addr); err != nil {
		slog.Error("Failed to start server", "error", err)
		return
	}
}
