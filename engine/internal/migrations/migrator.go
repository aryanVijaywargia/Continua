package migrations

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	// Register the pgx database/sql driver used by golang-migrate's Postgres backend.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// EngineMigrationsTable isolates engine migration bookkeeping from the platform
// migrator so both can coexist in the same shared Postgres database.
const EngineMigrationsTable = "engine_schema_migrations"

// New constructs a golang-migrate migrator from an fs-backed migration source.
func New(databaseURL string, fsys fs.FS) (*migrate.Migrate, error) {
	return NewWithTable(databaseURL, fsys, EngineMigrationsTable)
}

// NewWithTable constructs a golang-migrate migrator from an fs-backed migration
// source using the supplied migrations table.
func NewWithTable(databaseURL string, fsys fs.FS, migrationsTable string) (*migrate.Migrate, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open engine database: %w", err)
	}

	driverConfig := &postgres.Config{}
	if migrationsTable != "" {
		driverConfig.MigrationsTable = migrationsTable
	}

	driver, err := postgres.WithInstance(db, driverConfig)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create engine migration driver: %w", err)
	}

	source, err := iofs.New(fsys, ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create engine migration source: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create engine migrator: %w", err)
	}

	return migrator, nil
}

// Close closes the migrator source and database handles.
func Close(migrator *migrate.Migrate) error {
	if migrator == nil {
		return nil
	}

	sourceErr, dbErr := migrator.Close()
	return errors.Join(sourceErr, dbErr)
}
