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
	// Creating the database
	db, err := database.NewDatabase(
		ctx,
		cfg.GetBaseConfig().Database,
		embedMigrations,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return db, nil
}
