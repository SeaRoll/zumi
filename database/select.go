package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SelectRow executes a query and returns a single row as a struct of type T.
// It uses the provided dbtx to execute the query and collects the result into a struct of type T.
// If the query fails or no rows are returned, it returns an error.
func SelectRow[T any](ctx context.Context, dbtx DBTX, query string, args ...any) (T, error) {
	var result T
	row, err := dbtx.Query(ctx, query, args...)
	if err != nil {
		return result, fmt.Errorf("failed to execute query: %w", err)
	}

	result, err = pgx.CollectOneRow(row, pgx.RowToStructByName[T])
	if err != nil {
		return result, fmt.Errorf("failed to collect row: %w", err)
	}

	return result, nil
}

// SelectRows executes a query and returns multiple rows as a slice of structs of type T.
// It uses the provided dbtx to execute the query and collects the results into a slice of type T.
// If the query fails or no rows are returned, it returns an error.
func SelectRows[T any](ctx context.Context, dbtx DBTX, query string, args ...any) ([]T, error) {
	rows, err := dbtx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, fmt.Errorf("failed to collect rows: %w", err)
	}

	return results, nil
}

// ExecQuery executes a query that does not return rows (e.g., INSERT, UPDATE, DELETE).
// It uses the provided dbtx to execute the query and returns an error if the execution fails.
func ExecQuery(ctx context.Context, dbtx DBTX, query string, args ...any) error {
	_, err := dbtx.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to execute query: %w", err)
	}
	return nil
}
