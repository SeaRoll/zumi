package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/SeaRoll/zumi/cache"
	"github.com/SeaRoll/zumi/database"
	"github.com/SeaRoll/zumi/queue"
	"github.com/SeaRoll/zumi/server"
	"github.com/google/uuid"
)

//go:generate go run ../../server/gen "-title=Zumi API" "-version=1.0.0" "-description=Zumi API for managing books and events" "-servers=http://localhost:8080,https://api.example.com"

type Book struct {
	TableName   string    `db:"-" table:"books b" json:"-"`
	ID          uuid.UUID `db:"id" json:"id" primary:"true"`
	Title       string    `db:"title" json:"title"`
	Description string    `db:"description" json:"description"`
}

var (
	//go:embed migrations/*.sql
	embedMigrations embed.FS
	//go:embed openapi.yaml
	embedOpenAPI string
	//go:embed index.html
	embedIndexHTML string
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

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
	defer cache.Disconnect()

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
		if _, err := w.Write([]byte(embedIndexHTML)); err != nil {
			http.Error(w, fmt.Sprintf("failed to write index.html: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// gen:ignore
	server.AddHandler("GET /openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(embedOpenAPI)); err != nil {
			http.Error(w, fmt.Sprintf("failed to write openapi.yaml: %v", err), http.StatusInternalServerError)
			return
		}
	})

	// Get all books handler
	server.AddHandler("GET /api/v1/books", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx context.Context `ctx:"context"`
		}
		if err := server.ParseRequest(r, &req); err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		var books []Book
		if err := db.WithTX(req.Ctx, func(tx database.DBTX) error {
			var err error
			books, err = database.Find[Book](req.Ctx, tx, "")
			if err != nil {
				return fmt.Errorf("failed to retrieve all books: %w", err)
			}
			return nil
		}); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve books: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusOK, books)
	})

	// Get a book by ID handler
	//
	// This handler retrieves a book by its ID from the path parameter.
	server.AddHandler("GET /api/v1/books/{id}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx context.Context `ctx:"context"`
			ID  uuid.UUID       `path:"id"`
		}
		if err := server.ParseRequest(r, &req); err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		var book Book
		if err := db.WithTX(req.Ctx, func(tx database.DBTX) error {
			var err error
			book, err = database.FindOne[Book](req.Ctx, tx, "WHERE id = $1", req.ID)
			if err != nil {
				return fmt.Errorf("failed to retrieve book: %w", err)
			}
			return nil
		}); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve book: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusOK, book)
	})

	// Add a book handler
	server.AddHandler("POST /api/v1/books", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Ctx  context.Context `ctx:"context"`
			Book struct {
				Title       string `json:"title"`
				Description string `json:"description"`
			} `body:"json"`
		}
		if err := server.ParseRequest(r, &req); err != nil {
			server.WriteError(w, http.StatusBadRequest, fmt.Sprintf("failed to parse request: %v", err))
			return
		}

		newBook := Book{
			ID:          uuid.New(),
			Title:       req.Book.Title,
			Description: req.Book.Description,
		}

		if err := db.WithTX(req.Ctx, func(tx database.DBTX) error {
			if err := database.Save(
				ctx,
				tx,
				newBook,
			); err != nil {
				return fmt.Errorf("failed to insert book: %w", err)
			}
			return nil
		}); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to insert book: %v", err))
			return
		}

		// Publish the book event to the queue
		bookEvent, err := json.Marshal(newBook)
		if err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal book event: %v", err))
			return
		}
		if err := mq.Publish("events.books", bookEvent); err != nil {
			server.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to publish book event: %v", err))
			return
		}

		server.WriteJSON(w, http.StatusCreated, nil)
	})

	// Start the server
	addr := ":8080"
	if err := server.StartServer(ctx, addr); err != nil {
		slog.Error("Failed to start server", "error", err)
		return
	}
}
