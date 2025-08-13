package books

import (
	"context"
	"errors"
	"fmt"

	"github.com/SeaRoll/zumi/database"
	"github.com/google/uuid"
)

type Repository interface {
	SaveBook(ctx context.Context, tx database.DBTX, book Book) (Book, error)
	FindBooks(ctx context.Context, tx database.DBTX, pageRequest database.PageRequest) (database.Page[Book], error)
	FindOptionalBookByID(ctx context.Context, tx database.DBTX, id uuid.UUID) (*Book, error)
	DeleteBookByID(ctx context.Context, tx database.DBTX, id uuid.UUID) error
}

type repository struct{}

func NewRepository() Repository {
	return &repository{}
}

// DeleteBookByID implements Repository.
func (r *repository) DeleteBookByID(ctx context.Context, tx database.DBTX, id uuid.UUID) error {
	_, err := tx.Exec(ctx, "DELETE FROM books WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete book with id %d: %w", id, err)
	}
	return nil
}

// FindBooks implements Repository.
func (r *repository) FindBooks(ctx context.Context, tx database.DBTX, pageRequest database.PageRequest) (database.Page[Book], error) {
	books, err := database.SelectRowsPageable[Book](ctx, tx, pageRequest, "SELECT * FROM books")
	if err != nil {
		return books, fmt.Errorf("failed to retrieve all books: %w", err)
	}
	return books, nil
}

// FindOptionalBookByID implements Repository.
func (r *repository) FindOptionalBookByID(ctx context.Context, tx database.DBTX, id uuid.UUID) (*Book, error) {
	book, err := database.SelectRow[Book](ctx, tx, "SELECT * FROM books WHERE id = $1", id)
	if err != nil {
		if errors.Is(err, database.ErrNoRows) {
			return nil, nil // No book found with the given ID
		}
		return nil, fmt.Errorf("failed to retrieve book: %w", err)
	}
	return &book, nil
}

// SaveBook implements Repository.
func (r *repository) SaveBook(ctx context.Context, tx database.DBTX, book Book) (Book, error) {
	book, err := database.SelectRow[Book](ctx, tx, `
INSERT INTO books (id, title, description) 
VALUES ($1, $2, $3)
ON CONFLICT (id) DO UPDATE SET
	title = EXCLUDED.title,
	description = EXCLUDED.description
RETURNING *
`, book.ID, book.Title, book.Description)
	if err != nil {
		return Book{}, fmt.Errorf("failed to insert book: %w", err)
	}

	return book, nil
}
