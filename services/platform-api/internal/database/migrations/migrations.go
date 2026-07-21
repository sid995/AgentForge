// Package migrations applies checked-in PostgreSQL schema migrations.
package migrations

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Apply migrates a database to the latest checked-in version. It is safe to
// invoke repeatedly; a fully migrated database returns nil.
func Apply(databaseURL, directory string) error {
	absDirectory, err := filepath.Abs(directory)
	if err != nil {
		return fmt.Errorf("resolve migration directory: %w", err)
	}
	migrator, err := migrate.New("file://"+filepath.ToSlash(absDirectory), databaseURL)
	if err != nil {
		return fmt.Errorf("open migrations: %w", err)
	}
	defer func() { _, _ = migrator.Close() }()
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// Version returns the current migration version and whether it is dirty.
func Version(databaseURL, directory string) (uint, bool, error) {
	absDirectory, err := filepath.Abs(directory)
	if err != nil {
		return 0, false, fmt.Errorf("resolve migration directory: %w", err)
	}
	migrator, err := migrate.New("file://"+filepath.ToSlash(absDirectory), databaseURL)
	if err != nil {
		return 0, false, fmt.Errorf("open migrations: %w", err)
	}
	defer func() { _, _ = migrator.Close() }()
	version, dirty, err := migrator.Version()
	if err != nil {
		return 0, false, fmt.Errorf("read migration version: %w", err)
	}
	return version, dirty, nil
}
