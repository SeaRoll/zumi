package springbootlike

import (
	"context"
	"embed"
	"fmt"

	"github.com/SeaRoll/zumi/database"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

func NewDatabase(ctx context.Context, cfg AppConfig) (database.Database, error) {
	url := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Database.User,
		cfg.Database.Pass,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
	)

	// Creating the database
	db, err := database.NewDatabase(
		ctx,
		url,
		embedMigrations,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return db, nil
}
