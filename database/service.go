package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:generate go run github.com/SeaRoll/interfacer/cmd -struct=dbo -name=Database -file=interface.go

type dbo struct {
	connectionUrl string
	migrations    *fs.FS
	pool          *pgxpool.Pool
	isTeardown    atomic.Bool
}

// NewDatabase creates a new database connection pool and runs migrations.
// It takes a context for the connection, a connection URL, and a filesystem containing migration files.
// It returns a Database interface or an error if the connection or migration fails.
func NewDatabase(
	ctx context.Context,
	connectionUrl string,
	migrations fs.FS,
) (Database, error) {
	d := &dbo{
		connectionUrl: connectionUrl,
		migrations:    &migrations,
		isTeardown:    atomic.Bool{},
	}

	if err := d.connectAndMigratePool(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect and migrate pool: %w", err)
	}

	d.runReconnect()
	return d, nil
}

// NewDatabaseNoMigrations creates a new database connection pool without running migrations.
// It takes a context for the connection and a connection URL.
// It returns a Database interface or an error if the connection fails.
func NewDatabaseNoMigrations(
	ctx context.Context,
	connectionUrl string,
) (Database, error) {
	d := &dbo{
		connectionUrl: connectionUrl,
		isTeardown:    atomic.Bool{},
		migrations:    nil,
	}

	if err := d.connectAndMigratePool(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect and migrate pool: %w", err)
	}

	d.runReconnect()
	return d, nil
}

// runReconnect starts a goroutine that periodically checks the health of the database connection pool.
func (d *dbo) runReconnect() {
	// create go coroutine to check every dbs health
	// if any db is not healthy, try to reconnect
	go func() {
		for {
			// check if tearing down is requested
			if d.isTeardown.Load() {
				slog.Info("db is being torn down, skipping health check")
				return
			}

			d.healthCheckPool()
			time.Sleep(5 * time.Second)
		}
	}()
}

// healthCheckPool checks the health of the database connection pool.
func (d *dbo) healthCheckPool() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.pool.Ping(ctx); err != nil {
		slog.Error("db is not healthy", "error", err)
		if err := d.connectAndMigratePool(ctx); err != nil {
			slog.Error("failed to reconnect to db", "error", err)
		} else {
			slog.Info("reconnected to db")
		}
	}
}

// connectAndMigratePool connects to the database and runs migrations.
// It constructs the database URL from the configuration, parses it, and creates a connection pool.
func (d *dbo) connectAndMigratePool(ctx context.Context) error {
	// give 15 seconds for the db to be ready
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	config, err := pgxpool.ParseConfig(d.connectionUrl)
	if err != nil {
		return fmt.Errorf("failed to parse database config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create database connection pool: %w", err)
	}

	if d.migrations != nil {
		dbo := stdlib.OpenDBFromPool(pool)
		if err := migrate(dbo, *d.migrations); err != nil {
			return fmt.Errorf("failed to run database migrations: %w", err)
		}
	}

	d.pool = pool

	return nil
}

// migrate runs the database migrations using goose.
func migrate(db *sql.DB, migrations fs.FS) error {
	// setup database connection
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// Disconnect closes the database connection pool and sets the teardown flag to true.
// This method should be called when the application is shutting down to ensure all resources are released properly.
// It sets the isTeardown flag to true to indicate that the database connection is being torn down.
// This prevents any further operations on the database connection pool after it has been closed.
//
// If `noTeardown` is true, it will not set the teardown flag,
// allowing the client to be reused later.
// // If `noTeardown` is false or not provided, it will set the teardown flag
// and close the client connection, preventing any further operations.
func (d *dbo) Disconnect(noTeardown ...bool) {
	if len(noTeardown) == 0 || !noTeardown[0] {
		d.isTeardown.Store(true) // Set teardown flag to true
	}
	d.pool.Close()
	slog.Info("Database connection pool closed")
}

// WithReadTX executes a function within a read-only database transaction context.
// If an existing transaction is provided via existingQ, it uses that instead of creating a new transaction.
// Otherwise, it begins a new read-only transaction, executes the provided function with the transaction-aware dbtx,
// and commits the transaction on success or rolls back on error.
// The function automatically handles transaction cleanup through deferred rollback.
// This method is optimized for read operations and may provide better performance for queries that don't modify data.
//
// Parameters:
//   - ctx: Context for the transaction operation
//   - fn: Function to execute within the transaction, receives a transaction interface
//   - existingQ: Optional existing transaction to reuse instead of creating a new transaction
//
// Returns:
//   - error: Any error from transaction operations or the executed function
func (d *dbo) WithReadTX(ctx context.Context, fn func(tx DBTX) error, existingQ ...DBTX) error {
	return d.runTransactionWithOpts(ctx, fn, pgx.TxOptions{AccessMode: pgx.ReadOnly}, existingQ...)
}

// WithTX executes a function within a database transaction context.
// If an existing transaction is provided via existingQ, it uses that instead of creating a new transaction.
// Otherwise, it begins a new read-write transaction, executes the provided function with the transaction-aware dbtx,
// and commits the transaction on success or rolls back on error.
// The function automatically handles transaction cleanup through deferred rollback.
//
// Parameters:
//   - ctx: Context for the transaction operation
//   - fn: Function to execute within the transaction, receives a transaction interface
//   - existingQ: Optional existing transaction to reuse instead of creating a new transaction
//
// Returns:
//   - error: Any error from transaction operations or the executed function
func (d *dbo) WithTX(ctx context.Context, fn func(tx DBTX) error, existingQ ...DBTX) error {
	return d.runTransactionWithOpts(ctx, fn, pgx.TxOptions{AccessMode: pgx.ReadWrite}, existingQ...)
}

// runTransactionWithOpts executes a function within a transaction context with specified options.
// If an existing tx is provided via existingQ, it uses that instead of creating a new transaction.
// Otherwise, it begins a new transaction with the provided options, executes the function with the transaction-aware dbtx,
// and commits the transaction on success or rolls back on error.
// The function automatically handles transaction cleanup through deferred rollback.
//
// Parameters:
//   - ctx: Context for the transaction operation
//   - fn: Function to execute within the transaction, receives a DBTX interface
//   - opts: Transaction options to configure the transaction behavior
//   - existingQ: Optional existing tx to reuse instead of creating a new transaction
//
// Returns:
//   - error: Any error from transaction operations or the executed function
func (d *dbo) runTransactionWithOpts(ctx context.Context, fn func(tx DBTX) error, opts pgx.TxOptions, existingQ ...DBTX) error {
	if len(existingQ) > 0 {
		return fn(existingQ[0])
	}

	tx, err := d.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return fmt.Errorf("transaction function failed: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
