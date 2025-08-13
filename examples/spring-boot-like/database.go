package springbootlike

import (
	"context"
	"embed"

	"github.com/SeaRoll/zumi/database"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func NewDatabase(ctx context.Context, url string) (database.Database, error) {
	// Creating the database
	return database.NewDatabase(
		ctx,
		url,
		embedMigrations,
	)
}
