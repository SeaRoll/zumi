package database

import (
	"context"
	"embed"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

//go:embed migrations/*.sql
var testMigrations embed.FS

type Book struct {
	ID          int    `db:"id" json:"id"`
	Title       string `db:"title" json:"title"`
	Description string `db:"description" json:"description"`
}

func setupDatabase(ctx context.Context, t *testing.T) Database {
	t.Helper()

	db, err := NewDatabase(ctx, "postgres://postgres:mysecretpassword@localhost:5432/foodie?sslmode=disable", testMigrations)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	t.Cleanup(func() {
		db.Disconnect()
	})

	return db
}

func TestDatabaseExecSelectOneAndMultiple(t *testing.T) {
	ctx := context.Background()
	db := setupDatabase(ctx, t)

	t.Run("Insert and Select One", func(t *testing.T) {
		if err := db.WithTX(ctx, func(tx DBTX) error {
			bookTitle := uuid.NewString()
			if err := ExecQuery(
				ctx,
				tx,
				"INSERT INTO books (title, description) VALUES ($1, $2)",
				bookTitle,
				"This is a test book description",
			); err != nil {
				return err
			}

			book, err := SelectRow[Book](
				ctx,
				tx,
				"SELECT * FROM books WHERE title = $1",
				bookTitle,
			)
			if err != nil {
				return err
			}

			assert.Equal(t, bookTitle, book.Title)
			assert.Equal(t, "This is a test book description", book.Description)

			books, err := SelectRows[Book](ctx, tx, "SELECT * FROM books")
			if err != nil {
				return err
			}

			assert.NotEmpty(t, books)
			assert.Contains(t, books, book)

			return nil
		}); err != nil {
			t.Fatalf("Failed to insert and select one: %v", err)
		}
	})

}
