package springbootlike

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/SeaRoll/zumi/database"
	"github.com/SeaRoll/zumi/queue"
	"github.com/google/uuid"
)

type Service interface {
	CreateBook(ctx context.Context, newBook NewBookDTO, tx ...database.DBTX) (BookDTO, error)
	GetBooks(ctx context.Context, pageRequest database.PageRequest, tx ...database.DBTX) (database.Page[BookDTO], error)
	GetBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (BookDTO, error)
	DeleteBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error
}

type service struct {
	mq         queue.Queue
	db         database.Database
	repository Repository
}

func NewService(mq queue.Queue, db database.Database, repository Repository) Service {
	s := &service{
		mq:         mq,
		db:         db,
		repository: repository,
	}

	go s.mq.Consume(queue.ConsumerConfig{
		ConsumerName: "api",
		Topic:        "events.books",
		FetchLimit:   1,
		Callback: func(ctx context.Context, events []queue.Event) []int {
			processed := []int{}

			slog.Info("Received events", "count", len(events))
			for _, event := range events {
				slog.Info("Processing event", "event", string(event.Payload))
				processed = append(processed, event.Index)
			}
			return processed
		},
	})

	return s
}

// CreateBook implements Service.
func (s *service) CreateBook(ctx context.Context, newBook NewBookDTO, tx ...database.DBTX) (BookDTO, error) {
	var book Book

	err := s.db.WithTX(ctx, func(tx database.DBTX) error {
		var err error

		book, err = s.repository.SaveBook(ctx, tx, Book{
			ID:          uuid.New(),
			Title:       newBook.Title,
			Description: newBook.Description,
		})
		if err != nil {
			return fmt.Errorf("failed to save book: %w", err)
		}

		return nil
	}, tx...)
	if err != nil {
		return BookDTO{}, err
	}

	b, err := json.Marshal(book)
	if err != nil {
		return BookDTO{}, fmt.Errorf("failed to marshal book: %w", err)
	}

	err = s.mq.Publish("events.books", b)
	if err != nil {
		return BookDTO{}, fmt.Errorf("failed to publish book event: %w", err)
	}

	return BookDTO(book), nil
}

// GetBooks implements Service.
func (s *service) GetBooks(ctx context.Context, pageRequest database.PageRequest, tx ...database.DBTX) (database.Page[BookDTO], error) {
	var books database.Page[Book]

	err := s.db.WithReadTX(ctx, func(tx database.DBTX) error {
		var err error

		books, err = s.repository.FindBooks(ctx, tx, pageRequest)
		if err != nil {
			return fmt.Errorf("failed to find books: %w", err)
		}

		return nil
	}, tx...)
	if err != nil {
		return database.Page[BookDTO]{}, err
	}

	bookDTOs := make([]BookDTO, len(books.Content))
	for i, book := range books.Content {
		bookDTOs[i] = BookDTO(book)
	}

	return database.MapContent(books, bookDTOs), nil
}

// GetBookByID implements Service.
func (s *service) GetBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (BookDTO, error) {
	var book *Book

	err := s.db.WithReadTX(ctx, func(tx database.DBTX) error {
		var err error

		book, err = s.repository.FindOptionalBookByID(ctx, tx, id)
		if err != nil {
			return fmt.Errorf("failed to find book by id %s: %w", id, err)
		}

		return nil
	}, tx...)
	if err != nil {
		return BookDTO{}, err
	}

	if book == nil {
		return BookDTO{}, fmt.Errorf("book with id %s not found", id)
	}

	return BookDTO{
		ID:          book.ID,
		Title:       book.Title,
		Description: book.Description,
	}, nil
}

// DeleteBookByID implements Service.
func (s *service) DeleteBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error {
	return s.db.WithTX(ctx, func(tx database.DBTX) error {
		return s.repository.DeleteBookByID(ctx, tx, id)
	}, tx...)
}
