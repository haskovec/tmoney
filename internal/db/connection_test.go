package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestCreate(t *testing.T) {
	t.Run("creates new database file", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Verify file was created
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("database file was not created")
		}

		// Verify path is stored
		if db.Path() != path {
			t.Errorf("Path() = %v, want %v", db.Path(), path)
		}

		// Verify connection is valid
		if db.Conn() == nil {
			t.Error("Conn() returned nil")
		}
	})

	t.Run("adds .tdb extension if missing", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test")
		expectedPath := path + FileExtension

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		if db.Path() != expectedPath {
			t.Errorf("Path() = %v, want %v", db.Path(), expectedPath)
		}
	})

	t.Run("creates parent directories", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "subdir", "nested", "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("database file was not created in nested directory")
		}
	})

	t.Run("returns error if file already exists", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		// Create the file first
		db1, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		db1.Close()

		// Try to create again
		_, err = Create(path)
		if err == nil {
			t.Fatal("Create() expected error for existing file")
		}

		var fileExistsErr *FileExistsError
		if !errors.As(err, &fileExistsErr) {
			t.Errorf("expected FileExistsError, got %T: %v", err, err)
		}
	})

	t.Run("initializes metadata table", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Check app_identifier
		identifier, err := db.GetMetadata("app_identifier")
		if err != nil {
			t.Fatalf("GetMetadata(app_identifier) error = %v", err)
		}
		if identifier != AppIdentifier {
			t.Errorf("app_identifier = %v, want %v", identifier, AppIdentifier)
		}

		// Check schema_version
		version, err := db.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if version != 1 {
			t.Errorf("SchemaVersion() = %v, want 1", version)
		}

		// Check default_currency
		currency, err := db.GetMetadata("default_currency")
		if err != nil {
			t.Fatalf("GetMetadata(default_currency) error = %v", err)
		}
		if currency != "USD" {
			t.Errorf("default_currency = %v, want USD", currency)
		}
	})
}

func TestOpen(t *testing.T) {
	t.Run("opens existing database", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		// Create a valid database first
		db1, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		db1.Close()

		// Open it
		db2, err := Open(path)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db2.Close()

		// Verify connection works
		version, err := db2.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if version != 1 {
			t.Errorf("SchemaVersion() = %v, want 1", version)
		}
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		_, err := Open("/nonexistent/path/test.tdb")
		if err == nil {
			t.Fatal("Open() expected error for non-existent file")
		}

		var fileNotFoundErr *FileNotFoundError
		if !errors.As(err, &fileNotFoundErr) {
			t.Errorf("expected FileNotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("returns error for non-tmoney database", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		// Create a raw DuckDB file without TMoney metadata
		rawDB, err := createRawDuckDB(path)
		if err != nil {
			t.Fatalf("failed to create raw DuckDB: %v", err)
		}
		rawDB.Close()

		// Try to open as TMoney file
		_, err = Open(path)
		if err == nil {
			t.Fatal("Open() expected error for non-TMoney file")
		}

		var invalidFileErr *InvalidFileError
		if !errors.As(err, &invalidFileErr) {
			t.Errorf("expected InvalidFileError, got %T: %v", err, err)
		}
	})

	t.Run("returns error for wrong app_identifier", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		// Create a database with wrong identifier
		rawDB, err := createRawDuckDB(path)
		if err != nil {
			t.Fatalf("failed to create raw DuckDB: %v", err)
		}
		_, err = rawDB.Conn().Exec(`
			CREATE TABLE _metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
			INSERT INTO _metadata VALUES ('app_identifier', 'not_tmoney');
		`)
		if err != nil {
			t.Fatalf("failed to insert wrong metadata: %v", err)
		}
		rawDB.Close()

		// Try to open
		_, err = Open(path)
		if err == nil {
			t.Fatal("Open() expected error for wrong app_identifier")
		}

		var invalidFileErr *InvalidFileError
		if !errors.As(err, &invalidFileErr) {
			t.Errorf("expected InvalidFileError, got %T: %v", err, err)
		}
	})
}

func TestClose(t *testing.T) {
	t.Run("closes connection", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		err = db.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}

		// Verify connection is nil after close
		if db.Conn() != nil {
			t.Error("Conn() should be nil after Close()")
		}
	})

	t.Run("close is idempotent", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Close multiple times should not error
		if err := db.Close(); err != nil {
			t.Errorf("first Close() error = %v", err)
		}
		if err := db.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}
	})
}

func TestMetadata(t *testing.T) {
	t.Run("GetMetadata returns stored value", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		value, err := db.GetMetadata("default_currency")
		if err != nil {
			t.Fatalf("GetMetadata() error = %v", err)
		}
		if value != "USD" {
			t.Errorf("GetMetadata() = %v, want USD", value)
		}
	})

	t.Run("GetMetadata returns error for missing key", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.GetMetadata("nonexistent_key")
		if err == nil {
			t.Fatal("GetMetadata() expected error for missing key")
		}

		var notFoundErr *MetadataNotFoundError
		if !errors.As(err, &notFoundErr) {
			t.Errorf("expected MetadataNotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("SetMetadata updates existing value", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		err = db.SetMetadata("default_currency", "EUR")
		if err != nil {
			t.Fatalf("SetMetadata() error = %v", err)
		}

		value, err := db.GetMetadata("default_currency")
		if err != nil {
			t.Fatalf("GetMetadata() error = %v", err)
		}
		if value != "EUR" {
			t.Errorf("GetMetadata() = %v, want EUR", value)
		}
	})

	t.Run("SetMetadata creates new value", func(t *testing.T) {
		tempDir := t.TempDir()
		path := filepath.Join(tempDir, "test.tdb")

		db, err := Create(path)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		err = db.SetMetadata("custom_key", "custom_value")
		if err != nil {
			t.Fatalf("SetMetadata() error = %v", err)
		}

		value, err := db.GetMetadata("custom_key")
		if err != nil {
			t.Fatalf("GetMetadata() error = %v", err)
		}
		if value != "custom_value" {
			t.Errorf("GetMetadata() = %v, want custom_value", value)
		}
	})
}

func TestDefaultDirectory(t *testing.T) {
	t.Run("returns path with TMoney directory", func(t *testing.T) {
		dir := DefaultDirectory()

		// Should end with Documents/TMoney
		if filepath.Base(dir) != "TMoney" {
			t.Errorf("DefaultDirectory() = %v, want directory named TMoney", dir)
		}
		if filepath.Base(filepath.Dir(dir)) != "Documents" {
			t.Errorf("DefaultDirectory() = %v, want parent named Documents", dir)
		}
	})
}

// createRawDuckDB creates a plain DuckDB file without TMoney metadata
func createRawDuckDB(path string) (*DB, error) {
	conn, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, err
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, err
	}

	return &DB{conn: conn, path: path}, nil
}
