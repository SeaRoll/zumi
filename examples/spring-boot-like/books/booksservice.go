package books

import (
	"context"
	"fmt"

	"github.com/SeaRoll/zumi/database"
	"github.com/google/uuid"
)

type Service interface {
	CreateBook(ctx context.Context, newBook NewBookDTO, tx ...database.DBTX) (BookDTO, error)
	GetBooks(ctx context.Context, pageRequest database.PageRequest, tx ...database.DBTX) (database.Page[BookDTO], error)
	GetBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (BookDTO, error)
	DeleteBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) error
}

type service struct {
	db         database.Database
	repository Repository
}

func NewService(db database.Database, repository Repository) Service {
	return &service{
		db:         db,
		repository: repository,
	}
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

		return err
	}, tx...)
	if err != nil {
		return BookDTO{}, err
	}

	return BookDTO{
		ID:          book.ID,
		Title:       book.Title,
		Description: book.Description,
	}, nil
}

// GetBooks implements Service.
func (s *service) GetBooks(ctx context.Context, pageRequest database.PageRequest, tx ...database.DBTX) (database.Page[BookDTO], error) {
	var books database.Page[Book]

	err := s.db.WithReadTX(ctx, func(tx database.DBTX) error {
		var err error

		books, err = s.repository.FindBooks(ctx, tx, pageRequest)

		return err
	}, tx...)
	if err != nil {
		return database.Page[BookDTO]{}, err
	}

	bookDTOs := make([]BookDTO, len(books.Content))
	for i, book := range books.Content {
		bookDTOs[i] = BookDTO{
			ID:          book.ID,
			Title:       book.Title,
			Description: book.Description,
		}
	}

	return database.MapContent(books, bookDTOs), nil
}

// GetBookByID implements Service.
func (s *service) GetBookByID(ctx context.Context, id uuid.UUID, tx ...database.DBTX) (BookDTO, error) {
	var book *Book

	err := s.db.WithReadTX(ctx, func(tx database.DBTX) error {
		var err error

		book, err = s.repository.FindOptionalBookByID(ctx, tx, id)

		return err
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
