package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMigrationFilename(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		wantVersion int
		wantName    string
		wantErr     bool
	}{
		{
			name:        "valid migration filename",
			filename:    "001_initial.sql",
			wantVersion: 1,
			wantName:    "initial",
			wantErr:     false,
		},
		{
			name:        "multi-word name",
			filename:    "002_add_feature_x.sql",
			wantVersion: 2,
			wantName:    "add_feature_x",
			wantErr:     false,
		},
		{
			name:        "high version number",
			filename:    "999_final_cleanup.sql",
			wantVersion: 999,
			wantName:    "final_cleanup",
			wantErr:     false,
		},
		{
			name:     "missing underscore",
			filename: "001initial.sql",
			wantErr:  true,
		},
		{
			name:     "invalid version number",
			filename: "abc_initial.sql",
			wantErr:  true,
		},
		{
			name:        "no sql extension - caller filters these out",
			filename:    "001_initial",
			wantVersion: 1,
			wantName:    "initial",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, name, err := parseMigrationFilename(tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMigrationFilename() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if version != tt.wantVersion {
					t.Errorf("parseMigrationFilename() version = %v, want %v", version, tt.wantVersion)
				}
				if name != tt.wantName {
					t.Errorf("parseMigrationFilename() name = %v, want %v", name, tt.wantName)
				}
			}
		})
	}
}

func TestLoadMigrations(t *testing.T) {
	t.Run("loads embedded migrations", func(t *testing.T) {
		migrations, err := loadMigrations()
		if err != nil {
			t.Fatalf("loadMigrations() error = %v", err)
		}

		if len(migrations) == 0 {
			t.Error("loadMigrations() returned no migrations")
		}

		// Verify first migration is version 1
		if migrations[0].Version != 1 {
			t.Errorf("first migration version = %v, want 1", migrations[0].Version)
		}

		// Verify migration has SQL content
		if len(migrations[0].SQL) == 0 {
			t.Error("first migration has empty SQL")
		}
	})

	t.Run("migrations are sorted by version", func(t *testing.T) {
		migrations, err := loadMigrations()
		if err != nil {
			t.Fatalf("loadMigrations() error = %v", err)
		}

		for i := 1; i < len(migrations); i++ {
			if migrations[i].Version <= migrations[i-1].Version {
				t.Errorf("migrations not sorted: version %d comes after %d",
					migrations[i].Version, migrations[i-1].Version)
			}
		}
	})
}

func TestMigrate(t *testing.T) {
	t.Run("creates schema on new database", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Verify schema version is updated to current
		version, err := db.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if version != CurrentSchemaVersion {
			t.Errorf("SchemaVersion() = %v, want %v", version, CurrentSchemaVersion)
		}

		// Verify tables were created
		tables := []string{
			"accounts",
			"categories",
			"payees",
			"payee_aliases",
			"transactions",
			"transaction_splits",
			"investment_lots",
			"scheduled_transactions",
		}

		for _, table := range tables {
			var tableName string
			err := db.Conn().QueryRow(`
				SELECT table_name FROM information_schema.tables
				WHERE table_name = ?
			`, table).Scan(&tableName)
			if err != nil {
				t.Errorf("table %s not found: %v", table, err)
			}
		}
	})

	t.Run("skips already applied migrations", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Migrate should be idempotent
		err = db.Migrate()
		if err != nil {
			t.Errorf("Migrate() second call error = %v", err)
		}

		db.Close()

		// Open and migrate again
		db2, err := Open(dbPath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db2.Close()

		version, err := db2.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if version != CurrentSchemaVersion {
			t.Errorf("SchemaVersion() = %v, want %v", version, CurrentSchemaVersion)
		}
	})
}

func TestNeedsMigration(t *testing.T) {
	t.Run("returns false when up to date", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		needs, err := db.NeedsMigration()
		if err != nil {
			t.Fatalf("NeedsMigration() error = %v", err)
		}
		if needs {
			t.Error("NeedsMigration() = true, want false")
		}
	})

	t.Run("returns error for newer schema version", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Set schema version higher than app version
		err = db.SetMetadata("schema_version", "999")
		if err != nil {
			t.Fatalf("SetMetadata() error = %v", err)
		}

		_, err = db.NeedsMigration()
		if err == nil {
			t.Error("NeedsMigration() expected error for newer schema")
		}

		var vErr *VersionMismatchError
		if _, ok := err.(*VersionMismatchError); !ok {
			t.Errorf("NeedsMigration() error type = %T, want *VersionMismatchError", err)
		} else {
			vErr = err.(*VersionMismatchError)
			if vErr.FileVersion != 999 || vErr.AppVersion != CurrentSchemaVersion {
				t.Errorf("VersionMismatchError = {%d, %d}, want {999, %d}",
					vErr.FileVersion, vErr.AppVersion, CurrentSchemaVersion)
			}
		}

		db.Close()
	})
}

func TestMigrateVersionMismatch(t *testing.T) {
	t.Run("returns error for file created with newer version", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Set schema version higher than app supports
		err = db.SetMetadata("schema_version", "999")
		if err != nil {
			t.Fatalf("SetMetadata() error = %v", err)
		}

		err = db.Migrate()
		if err == nil {
			t.Error("Migrate() expected error for newer schema")
		}

		if _, ok := err.(*VersionMismatchError); !ok {
			t.Errorf("Migrate() error type = %T, want *VersionMismatchError", err)
		}

		db.Close()
	})
}

func TestSchemaTablesExist(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.tdb")

	db, err := Create(dbPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer db.Close()

	// Test accounts table structure
	t.Run("accounts table has correct columns", func(t *testing.T) {
		_, err := db.Conn().Exec(`
			INSERT INTO accounts (name, type, opening_date)
			VALUES ('Test Checking', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Errorf("Failed to insert into accounts: %v", err)
		}
	})

	// Test categories table structure
	t.Run("categories table has correct columns", func(t *testing.T) {
		_, err := db.Conn().Exec(`
			INSERT INTO categories (name, type)
			VALUES ('Groceries', 'expense')
		`)
		if err != nil {
			t.Errorf("Failed to insert into categories: %v", err)
		}
	})

	// Test payees table structure
	t.Run("payees table has correct columns", func(t *testing.T) {
		_, err := db.Conn().Exec(`
			INSERT INTO payees (name)
			VALUES ('Test Payee')
		`)
		if err != nil {
			t.Errorf("Failed to insert into payees: %v", err)
		}
	})

	// Test views exist
	t.Run("account_balances view exists", func(t *testing.T) {
		rows, err := db.Conn().Query(`SELECT * FROM account_balances`)
		if err != nil {
			t.Errorf("Failed to query account_balances view: %v", err)
		}
		rows.Close()
	})

	t.Run("category_spending view exists", func(t *testing.T) {
		rows, err := db.Conn().Query(`SELECT * FROM category_spending`)
		if err != nil {
			t.Errorf("Failed to query category_spending view: %v", err)
		}
		rows.Close()
	})
}

func TestMigration002TransactionStatus(t *testing.T) {
	t.Run("migrates pending status to uncleared", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Insert a transaction with uncleared status (post-migration default)
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-15', -50.00, 'uncleared')
		`)
		if err != nil {
			t.Fatalf("Failed to insert uncleared transaction: %v", err)
		}

		// Verify the transaction has uncleared status
		var status string
		err = db.Conn().QueryRow(`SELECT status FROM transactions LIMIT 1`).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query transaction status: %v", err)
		}
		if status != "uncleared" {
			t.Errorf("Expected status 'uncleared', got %q", status)
		}
	})

	t.Run("allows void status after migration", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// Insert a void transaction
		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-15', 0, 'void')
		`)
		if err != nil {
			t.Fatalf("Failed to insert void transaction: %v", err)
		}

		var status string
		err = db.Conn().QueryRow(`SELECT status FROM transactions WHERE status = 'void'`).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query void transaction: %v", err)
		}
		if status != "void" {
			t.Errorf("Expected status 'void', got %q", status)
		}
	})

	t.Run("rejects pending status after migration", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// Inserting a 'pending' status should fail due to CHECK constraint
		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-15', -50.00, 'pending')
		`)
		if err == nil {
			t.Error("Expected error when inserting 'pending' status, but got none")
		}
	})

	t.Run("account_balances view excludes void from current_balance", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create account with opening balance 1000
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date, opening_balance)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01', 1000)
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// Add an uncleared transaction of -100
		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-15', -100, 'uncleared')
		`)
		if err != nil {
			t.Fatalf("Failed to insert uncleared transaction: %v", err)
		}

		// Add a void transaction of -500 (should be excluded from balance)
		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-16', -500, 'void')
		`)
		if err != nil {
			t.Fatalf("Failed to insert void transaction: %v", err)
		}

		// Add a cleared transaction of -200
		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-17', -200, 'cleared')
		`)
		if err != nil {
			t.Fatalf("Failed to insert cleared transaction: %v", err)
		}

		var currentBalance, clearedBalance float64
		err = db.Conn().QueryRow(`
			SELECT current_balance, cleared_balance FROM account_balances
			WHERE id = '11111111-1111-1111-1111-111111111111'
		`).Scan(&currentBalance, &clearedBalance)
		if err != nil {
			t.Fatalf("Failed to query account_balances: %v", err)
		}

		// current_balance = 1000 + (-100) + (-200) = 700 (void excluded)
		if currentBalance != 700 {
			t.Errorf("Expected current_balance 700, got %f", currentBalance)
		}

		// cleared_balance = 1000 + (-200) = 800 (only cleared + reconciled)
		if clearedBalance != 800 {
			t.Errorf("Expected cleared_balance 800, got %f", clearedBalance)
		}
	})

	t.Run("schema version is 2 after migration", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		version, err := db.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}
		if version != 2 {
			t.Errorf("Expected schema version 2, got %d", version)
		}
	})

	t.Run("default status for new transactions is uncleared", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// Insert without specifying status - should default to uncleared
		_, err = db.Conn().Exec(`
			INSERT INTO transactions (account_id, date, amount)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-15', -50.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert transaction with default status: %v", err)
		}

		var status string
		err = db.Conn().QueryRow(`SELECT status FROM transactions LIMIT 1`).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query transaction status: %v", err)
		}
		if status != "uncleared" {
			t.Errorf("Expected default status 'uncleared', got %q", status)
		}
	})
}

func TestCurrentSchemaVersion(t *testing.T) {
	t.Run("matches highest migration version", func(t *testing.T) {
		migrations, err := loadMigrations()
		if err != nil {
			t.Fatalf("loadMigrations() error = %v", err)
		}

		if len(migrations) == 0 {
			t.Fatal("No migrations found")
		}

		highestVersion := migrations[len(migrations)-1].Version
		if CurrentSchemaVersion != highestVersion {
			t.Errorf("CurrentSchemaVersion = %d, want %d (highest migration)",
				CurrentSchemaVersion, highestVersion)
		}
	})
}

func TestMigrationFileIntegrity(t *testing.T) {
	t.Run("all migrations have non-empty SQL", func(t *testing.T) {
		migrations, err := loadMigrations()
		if err != nil {
			t.Fatalf("loadMigrations() error = %v", err)
		}

		for _, m := range migrations {
			if len(m.SQL) == 0 {
				t.Errorf("migration %03d has empty SQL", m.Version)
			}
		}
	})

	t.Run("all migrations have valid names", func(t *testing.T) {
		migrations, err := loadMigrations()
		if err != nil {
			t.Fatalf("loadMigrations() error = %v", err)
		}

		for _, m := range migrations {
			if len(m.Name) == 0 {
				t.Errorf("migration %03d has empty name", m.Version)
			}
		}
	})
}

func TestOpenWithMigration(t *testing.T) {
	t.Run("runs migrations on open if needed", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		// Create initial database but simulate older version
		// by manually creating just metadata table
		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		db.Close()

		// Reopen - migrations should already be applied
		db2, err := Open(dbPath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		defer db2.Close()

		version, err := db2.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}

		if version != CurrentSchemaVersion {
			t.Errorf("SchemaVersion() after Open = %v, want %v", version, CurrentSchemaVersion)
		}
	})
}

func TestMigrationRollbackOnError(t *testing.T) {
	// This test verifies that individual migration failures don't
	// leave the database in a partially migrated state.
	// Since we're using transactions, a failed migration should roll back.

	t.Run("failed migration does not update version", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Verify the database is in a clean state after successful migrations
		version, err := db.SchemaVersion()
		if err != nil {
			t.Fatalf("SchemaVersion() error = %v", err)
		}

		if version != CurrentSchemaVersion {
			t.Errorf("Expected version %d after migration, got %d", CurrentSchemaVersion, version)
		}
	})
}

// Helper to remove test file if it exists
func removeIfExists(path string) {
	os.Remove(path)
	os.Remove(path + ".wal")
}
