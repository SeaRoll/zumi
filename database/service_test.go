package database

import (
	"context"
	"embed"
	"testing"

	"github.com/SeaRoll/zumi/config"
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

const cfgYaml = `
server:
  port: 8080
database:
  enabled: true
  host: localhost
  port: 5432
  user: postgres
  password: mysecretpassword
  name: foodie
`

func setupDatabase(ctx context.Context, t *testing.T) Database {
	t.Helper()

	cfg, err := config.FromYAML[config.BaseConfig](cfgYaml)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	t.Logf("Config: %+v", cfg)

	db, err := NewDatabase(ctx, cfg.GetBaseConfig().Database, testMigrations)
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
		err := db.WithTX(ctx, func(tx DBTX) error {
			bookTitle := uuid.NewString()
			err := ExecQuery(
				ctx,
				tx,
				"INSERT INTO books (title, description) VALUES ($1, $2)",
				bookTitle,
				"This is a test book description",
			)
			if err != nil {
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
		})
		if err != nil {
			t.Fatalf("Failed to insert and select one: %v", err)
		}
	})

}
