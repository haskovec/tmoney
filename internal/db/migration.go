package db

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// CurrentSchemaVersion is the latest schema version supported by this app.
const CurrentSchemaVersion = 24

// Migrate runs all pending migrations on the database.
// It reads the current schema_version and applies any migrations with a higher version.
func (db *DB) Migrate() error {
	currentVersion, err := db.SchemaVersion()
	if err != nil {
		return err
	}

	if currentVersion > CurrentSchemaVersion {
		return &VersionMismatchError{
			FileVersion: currentVersion,
			AppVersion:  CurrentSchemaVersion,
		}
	}

	if currentVersion == CurrentSchemaVersion {
		return nil // Already up to date
	}

	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.Version <= currentVersion {
			continue
		}

		if err := db.runMigration(m); err != nil {
			return fmt.Errorf("migration %03d failed: %w", m.Version, err)
		}
	}

	return nil
}

// Migration represents a single database migration.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// loadMigrations reads all migration files from the embedded filesystem.
func loadMigrations() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, &DatabaseError{Op: "read migrations dir", Err: err}
	}

	var migrations []Migration

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		version, migrationName, err := parseMigrationFilename(name)
		if err != nil {
			continue // Skip files that don't match pattern
		}

		content, err := migrationsFS.ReadFile(path.Join("migrations", name))
		if err != nil {
			return nil, &DatabaseError{Op: "read migration file", Err: err}
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    migrationName,
			SQL:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// parseMigrationFilename extracts version and name from a migration filename.
// Expected format: NNN_name.sql (e.g., 001_initial.sql)
func parseMigrationFilename(filename string) (int, string, error) {
	// Remove .sql extension
	base := strings.TrimSuffix(filename, ".sql")

	// Split on first underscore
	parts := strings.SplitN(base, "_", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("invalid version number in filename: %s", filename)
	}

	return version, parts[1], nil
}

// runMigration executes a single migration within a transaction.
func (db *DB) runMigration(m Migration) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return &DatabaseError{Op: "begin migration transaction", Err: err}
	}
	defer tx.Rollback()

	// Execute migration SQL
	_, err = tx.Exec(m.SQL)
	if err != nil {
		return &DatabaseError{Op: fmt.Sprintf("execute migration %s", m.Name), Err: err}
	}

	// Update schema version
	_, err = tx.Exec(`UPDATE _metadata SET value = ? WHERE key = 'schema_version'`, strconv.Itoa(m.Version))
	if err != nil {
		return &DatabaseError{Op: "update schema_version", Err: err}
	}

	if err := tx.Commit(); err != nil {
		return &DatabaseError{Op: "commit migration", Err: err}
	}

	return nil
}

// NeedsMigration checks if the database needs migration.
func (db *DB) NeedsMigration() (bool, error) {
	currentVersion, err := db.SchemaVersion()
	if err != nil {
		return false, err
	}

	if currentVersion > CurrentSchemaVersion {
		return false, &VersionMismatchError{
			FileVersion: currentVersion,
			AppVersion:  CurrentSchemaVersion,
		}
	}

	return currentVersion < CurrentSchemaVersion, nil
}
