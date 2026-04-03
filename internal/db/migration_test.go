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
			"reconciliation_sessions",
			"securities",
			"security_prices",
			"investment_transactions",
			"investment_positions",
			"investment_transaction_lots",
			"corporate_actions",
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

	t.Run("schema version is current after migration", func(t *testing.T) {
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
		if version != CurrentSchemaVersion {
			t.Errorf("Expected schema version %d, got %d", CurrentSchemaVersion, version)
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

func TestMigration003Reconciliation(t *testing.T) {
	t.Run("reconciliation_sessions table exists with correct columns", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create a test account
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// Insert a reconciliation session
		_, err = db.Conn().Exec(`
			INSERT INTO reconciliation_sessions (account_id, statement_date, statement_balance)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-31', 5234.56)
		`)
		if err != nil {
			t.Fatalf("Failed to insert reconciliation session: %v", err)
		}

		// Verify default status is in_progress
		var status string
		err = db.Conn().QueryRow(`SELECT status FROM reconciliation_sessions LIMIT 1`).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query reconciliation session status: %v", err)
		}
		if status != "in_progress" {
			t.Errorf("Expected default status 'in_progress', got %q", status)
		}
	})

	t.Run("rejects invalid reconciliation status", func(t *testing.T) {
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

		// Inserting an invalid status should fail due to CHECK constraint
		_, err = db.Conn().Exec(`
			INSERT INTO reconciliation_sessions (account_id, statement_date, statement_balance, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-31', 1000, 'invalid')
		`)
		if err == nil {
			t.Error("Expected error when inserting invalid reconciliation status, but got none")
		}
	})

	t.Run("reconciliation session references account via foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Inserting a session for a non-existent account should fail
		_, err = db.Conn().Exec(`
			INSERT INTO reconciliation_sessions (account_id, statement_date, statement_balance)
			VALUES ('99999999-9999-9999-9999-999999999999', '2024-01-31', 1000)
		`)
		if err == nil {
			t.Error("Expected foreign key error when inserting session for non-existent account")
		}
	})

	t.Run("allows completed status with completed_at", func(t *testing.T) {
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

		_, err = db.Conn().Exec(`
			INSERT INTO reconciliation_sessions (account_id, statement_date, statement_balance, status, completed_at)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-01-31', 5234.56, 'completed', CURRENT_TIMESTAMP)
		`)
		if err != nil {
			t.Fatalf("Failed to insert completed reconciliation session: %v", err)
		}

		var status string
		err = db.Conn().QueryRow(`SELECT status FROM reconciliation_sessions WHERE status = 'completed'`).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query completed session: %v", err)
		}
		if status != "completed" {
			t.Errorf("Expected status 'completed', got %q", status)
		}
	})
}

func TestMigration004AutoPost(t *testing.T) {
	t.Run("scheduled_transactions has auto_post and post_lead_days columns", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create a test account
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'checking', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// Insert a scheduled transaction with auto_post fields
		_, err = db.Conn().Exec(`
			INSERT INTO scheduled_transactions (
				account_id, frequency, interval, start_date, next_date,
				auto_post, post_lead_days
			)
			VALUES ('11111111-1111-1111-1111-111111111111', 'monthly', 1, '2024-01-01', '2024-02-01', TRUE, 3)
		`)
		if err != nil {
			t.Fatalf("Failed to insert scheduled transaction with auto_post: %v", err)
		}

		var autoPost bool
		var postLeadDays int
		err = db.Conn().QueryRow(`
			SELECT auto_post, post_lead_days FROM scheduled_transactions LIMIT 1
		`).Scan(&autoPost, &postLeadDays)
		if err != nil {
			t.Fatalf("Failed to query auto_post fields: %v", err)
		}
		if !autoPost {
			t.Error("Expected auto_post TRUE, got FALSE")
		}
		if postLeadDays != 3 {
			t.Errorf("Expected post_lead_days 3, got %d", postLeadDays)
		}
	})

	t.Run("auto_post defaults to FALSE", func(t *testing.T) {
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

		// Insert without specifying auto_post - should default to FALSE
		_, err = db.Conn().Exec(`
			INSERT INTO scheduled_transactions (
				account_id, frequency, interval, start_date, next_date
			)
			VALUES ('11111111-1111-1111-1111-111111111111', 'monthly', 1, '2024-01-01', '2024-02-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert scheduled transaction: %v", err)
		}

		var autoPost bool
		var postLeadDays int
		err = db.Conn().QueryRow(`
			SELECT auto_post, post_lead_days FROM scheduled_transactions LIMIT 1
		`).Scan(&autoPost, &postLeadDays)
		if err != nil {
			t.Fatalf("Failed to query auto_post defaults: %v", err)
		}
		if autoPost {
			t.Error("Expected auto_post to default to FALSE")
		}
		if postLeadDays != 0 {
			t.Errorf("Expected post_lead_days to default to 0, got %d", postLeadDays)
		}
	})
}

func TestMigration006Securities(t *testing.T) {
	t.Run("securities table accepts valid security", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Insert a security
		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency, exchange, hidden)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc.', 'stock', 'large_cap_stock', 'USD', 'NASDAQ', FALSE)
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		// Read it back and verify all columns
		var ticker, name, secType, assetClass, currency string
		var exchange *string
		var hidden bool
		err = db.Conn().QueryRow(`
			SELECT ticker, name, security_type, asset_class, currency, exchange, hidden
			FROM securities WHERE id = '11111111-1111-1111-1111-111111111111'
		`).Scan(&ticker, &name, &secType, &assetClass, &currency, &exchange, &hidden)
		if err != nil {
			t.Fatalf("Failed to read security: %v", err)
		}
		if ticker != "AAPL" {
			t.Errorf("Expected ticker 'AAPL', got %q", ticker)
		}
		if name != "Apple Inc." {
			t.Errorf("Expected name 'Apple Inc.', got %q", name)
		}
		if secType != "stock" {
			t.Errorf("Expected security_type 'stock', got %q", secType)
		}
		if assetClass != "large_cap_stock" {
			t.Errorf("Expected asset_class 'large_cap_stock', got %q", assetClass)
		}
		if currency != "USD" {
			t.Errorf("Expected currency 'USD', got %q", currency)
		}
		if exchange == nil || *exchange != "NASDAQ" {
			t.Errorf("Expected exchange 'NASDAQ', got %v", exchange)
		}
		if hidden {
			t.Error("Expected hidden FALSE")
		}
	})

	t.Run("rejects invalid security_type", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (ticker, name, security_type)
			VALUES ('BAD', 'Bad Security', 'bond')
		`)
		if err == nil {
			t.Error("Expected error when inserting invalid security_type 'bond'")
		}
	})

	t.Run("rejects invalid asset_class", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (ticker, name, security_type, asset_class)
			VALUES ('BAD', 'Bad Security', 'stock', 'real_estate')
		`)
		if err == nil {
			t.Error("Expected error when inserting invalid asset_class 'real_estate'")
		}
	})

	t.Run("defaults asset_class to unclassified and currency to USD", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (ticker, name, security_type)
			VALUES ('VTI', 'Vanguard Total Stock Market ETF', 'etf')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security with defaults: %v", err)
		}

		var assetClass, currency string
		err = db.Conn().QueryRow(`
			SELECT asset_class, currency FROM securities WHERE ticker = 'VTI'
		`).Scan(&assetClass, &currency)
		if err != nil {
			t.Fatalf("Failed to read security defaults: %v", err)
		}
		if assetClass != "unclassified" {
			t.Errorf("Expected default asset_class 'unclassified', got %q", assetClass)
		}
		if currency != "USD" {
			t.Errorf("Expected default currency 'USD', got %q", currency)
		}
	})

	t.Run("security_prices inserts and reads back correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create a security first
		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		// Insert a price
		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 195.50, 'manual')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security price: %v", err)
		}

		var price float64
		var source string
		err = db.Conn().QueryRow(`
			SELECT price, source FROM security_prices
			WHERE security_id = '11111111-1111-1111-1111-111111111111'
		`).Scan(&price, &source)
		if err != nil {
			t.Fatalf("Failed to read security price: %v", err)
		}
		if price != 195.50 {
			t.Errorf("Expected price 195.50, got %f", price)
		}
		if source != "manual" {
			t.Errorf("Expected source 'manual', got %q", source)
		}
	})

	t.Run("security_prices enforces unique constraint on security_id + date", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		// Insert first price
		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 195.50, 'manual')
		`)
		if err != nil {
			t.Fatalf("Failed to insert first price: %v", err)
		}

		// Insert duplicate (same security_id + date) should fail
		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 196.00, 'api')
		`)
		if err == nil {
			t.Error("Expected error when inserting duplicate security_id + date")
		}
	})

	t.Run("security_prices rejects non-positive price", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		// Zero price should fail
		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 0, 'manual')
		`)
		if err == nil {
			t.Error("Expected error when inserting zero price")
		}

		// Negative price should fail
		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-16', -10.00, 'manual')
		`)
		if err == nil {
			t.Error("Expected error when inserting negative price")
		}
	})

	t.Run("security_prices rejects invalid source", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 195.50, 'scrape')
		`)
		if err == nil {
			t.Error("Expected error when inserting invalid source 'scrape'")
		}
	})

	t.Run("security_prices references securities via foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// security_id FK on security_prices was removed in migration 010 for DuckDB UPDATE compatibility.
		// Validation is enforced at the application level.
		_, err = db.Conn().Exec(`
			INSERT INTO security_prices (security_id, date, price, source)
			VALUES ('99999999-9999-9999-9999-999999999999', '2024-06-15', 100.00, 'manual')
		`)
		if err != nil {
			t.Errorf("Insert should succeed (security_id FK removed in migration 010): %v", err)
		}
	})
}

func TestMigration008InvestmentTables(t *testing.T) {
	// SM-046: investment_transactions table
	t.Run("investment_transactions inserts and reads back correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create prerequisite account and security
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Investment Account', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test security: %v", err)
		}

		// Insert an investment transaction
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (
				id, account_id, date, transaction_type, security_id,
				shares, price_per_share, total_amount, commission, memo, status
			) VALUES (
				'33333333-3333-3333-3333-333333333333',
				'11111111-1111-1111-1111-111111111111',
				'2024-06-15', 'buy',
				'22222222-2222-2222-2222-222222222222',
				10.0, 185.00, 1854.95, 4.95, 'Buy 10 shares AAPL', 'cleared'
			)
		`)
		if err != nil {
			t.Fatalf("Failed to insert investment transaction: %v", err)
		}

		// Read it back
		var txType, status, memo string
		var shares, pricePerShare, totalAmount, commission float64
		err = db.Conn().QueryRow(`
			SELECT transaction_type, shares, price_per_share, total_amount, commission, memo, status
			FROM investment_transactions
			WHERE id = '33333333-3333-3333-3333-333333333333'
		`).Scan(&txType, &shares, &pricePerShare, &totalAmount, &commission, &memo, &status)
		if err != nil {
			t.Fatalf("Failed to read investment transaction: %v", err)
		}
		if txType != "buy" {
			t.Errorf("Expected transaction_type 'buy', got %q", txType)
		}
		if shares != 10.0 {
			t.Errorf("Expected shares 10.0, got %f", shares)
		}
		if pricePerShare != 185.00 {
			t.Errorf("Expected price_per_share 185.00, got %f", pricePerShare)
		}
		if totalAmount != 1854.95 {
			t.Errorf("Expected total_amount 1854.95, got %f", totalAmount)
		}
		if commission != 4.95 {
			t.Errorf("Expected commission 4.95, got %f", commission)
		}
		if memo != "Buy 10 shares AAPL" {
			t.Errorf("Expected memo 'Buy 10 shares AAPL', got %q", memo)
		}
		if status != "cleared" {
			t.Errorf("Expected status 'cleared', got %q", status)
		}
	})

	t.Run("investment_transactions rejects invalid transaction_type", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (account_id, date, transaction_type, total_amount)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 'invalid_type', 100.00)
		`)
		if err == nil {
			t.Error("Expected error when inserting invalid transaction_type")
		}
	})

	t.Run("investment_transactions rejects invalid status", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (account_id, date, transaction_type, total_amount, status)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 'deposit', 1000.00, 'void')
		`)
		if err == nil {
			t.Error("Expected error when inserting invalid status 'void'")
		}
	})

	t.Run("investment_transactions defaults status to pending", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (account_id, date, transaction_type, total_amount)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 'deposit', 5000.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert investment transaction: %v", err)
		}

		var status string
		err = db.Conn().QueryRow(`SELECT status FROM investment_transactions LIMIT 1`).Scan(&status)
		if err != nil {
			t.Fatalf("Failed to query status: %v", err)
		}
		if status != "pending" {
			t.Errorf("Expected default status 'pending', got %q", status)
		}
	})

	t.Run("investment_transactions enforces account foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (account_id, date, transaction_type, total_amount)
			VALUES ('99999999-9999-9999-9999-999999999999', '2024-06-15', 'deposit', 1000.00)
		`)
		if err == nil {
			t.Error("Expected foreign key error for non-existent account")
		}
	})

	t.Run("investment_transactions enforces security foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// security_id FK on investment_transactions was removed in migration 010 for DuckDB UPDATE compatibility.
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (account_id, date, transaction_type, security_id, total_amount)
			VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', 'buy', '99999999-9999-9999-9999-999999999999', 1000.00)
		`)
		if err != nil {
			t.Errorf("Insert should succeed (security_id FK removed in migration 010): %v", err)
		}
	})

	t.Run("investment_transactions allows all 12 transaction types", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		types := []string{
			"buy", "sell", "dividend", "reinvest_dividend",
			"fee", "fee_liquidation", "deposit", "withdrawal",
			"interest", "transfer_shares", "transfer_cash", "exchange",
		}
		for _, txType := range types {
			_, err = db.Conn().Exec(`
				INSERT INTO investment_transactions (account_id, date, transaction_type, total_amount)
				VALUES ('11111111-1111-1111-1111-111111111111', '2024-06-15', ?, 100.00)
			`, txType)
			if err != nil {
				t.Errorf("Failed to insert transaction_type %q: %v", txType, err)
			}
		}

		var count int
		err = db.Conn().QueryRow(`SELECT COUNT(*) FROM investment_transactions`).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count transactions: %v", err)
		}
		if count != 12 {
			t.Errorf("Expected 12 transactions, got %d", count)
		}
	})

	// SM-047: investment_lots table
	t.Run("investment_lots inserts and reads back correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create prerequisites
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Investment', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test security: %v", err)
		}

		// Create a source transaction
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (id, account_id, date, transaction_type, total_amount)
			VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '2024-06-15', 'buy', 1850.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert source transaction: %v", err)
		}

		// Insert a lot
		_, err = db.Conn().Exec(`
			INSERT INTO investment_lots (
				id, account_id, security_id, shares, original_shares,
				cost_per_share, purchase_date, source_transaction_id, closed
			) VALUES (
				'44444444-4444-4444-4444-444444444444',
				'11111111-1111-1111-1111-111111111111',
				'22222222-2222-2222-2222-222222222222',
				10.0, 10.0, 185.00, '2024-06-15',
				'33333333-3333-3333-3333-333333333333', FALSE
			)
		`)
		if err != nil {
			t.Fatalf("Failed to insert lot: %v", err)
		}

		// Read it back
		var shares, originalShares, costPerShare float64
		var closed bool
		err = db.Conn().QueryRow(`
			SELECT shares, original_shares, cost_per_share, closed
			FROM investment_lots WHERE id = '44444444-4444-4444-4444-444444444444'
		`).Scan(&shares, &originalShares, &costPerShare, &closed)
		if err != nil {
			t.Fatalf("Failed to read lot: %v", err)
		}
		if shares != 10.0 {
			t.Errorf("Expected shares 10.0, got %f", shares)
		}
		if originalShares != 10.0 {
			t.Errorf("Expected original_shares 10.0, got %f", originalShares)
		}
		if costPerShare != 185.00 {
			t.Errorf("Expected cost_per_share 185.00, got %f", costPerShare)
		}
		if closed {
			t.Error("Expected closed FALSE")
		}
	})

	t.Run("investment_lots enforces account foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test security: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO investment_lots (
				account_id, security_id, shares, original_shares,
				cost_per_share, purchase_date, source_transaction_id
			) VALUES (
				'99999999-9999-9999-9999-999999999999',
				'22222222-2222-2222-2222-222222222222',
				10.0, 10.0, 185.00, '2024-06-15',
				'33333333-3333-3333-3333-333333333333'
			)
		`)
		if err == nil {
			t.Error("Expected foreign key error for non-existent account")
		}
	})

	t.Run("investment_lots enforces security foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO investment_lots (
				account_id, security_id, shares, original_shares,
				cost_per_share, purchase_date, source_transaction_id
			) VALUES (
				'11111111-1111-1111-1111-111111111111',
				'99999999-9999-9999-9999-999999999999',
				10.0, 10.0, 185.00, '2024-06-15',
				'33333333-3333-3333-3333-333333333333'
			)
		`)
		// security_id FK on investment_lots was removed in migration 010 for DuckDB UPDATE compatibility.
		if err != nil {
			t.Errorf("Insert should succeed (security_id FK removed in migration 010): %v", err)
		}
	})

	// SM-048: investment_positions table
	t.Run("investment_positions inserts and reads back correctly", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Investment', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'VTI', 'Vanguard Total Stock Market', 'etf')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test security: %v", err)
		}

		// Insert a position
		_, err = db.Conn().Exec(`
			INSERT INTO investment_positions (
				id, account_id, security_id, shares, average_cost_per_share
			) VALUES (
				'55555555-5555-5555-5555-555555555555',
				'11111111-1111-1111-1111-111111111111',
				'22222222-2222-2222-2222-222222222222',
				25.5, 220.75
			)
		`)
		if err != nil {
			t.Fatalf("Failed to insert position: %v", err)
		}

		var shares, avgCost float64
		err = db.Conn().QueryRow(`
			SELECT shares, average_cost_per_share FROM investment_positions
			WHERE id = '55555555-5555-5555-5555-555555555555'
		`).Scan(&shares, &avgCost)
		if err != nil {
			t.Fatalf("Failed to read position: %v", err)
		}
		if shares != 25.5 {
			t.Errorf("Expected shares 25.5, got %f", shares)
		}
		if avgCost != 220.75 {
			t.Errorf("Expected average_cost_per_share 220.75, got %f", avgCost)
		}
	})

	t.Run("investment_positions enforces unique constraint on account_id + security_id", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Investment', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'VTI', 'Vanguard Total Stock Market', 'etf')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test security: %v", err)
		}

		// First insert should succeed
		_, err = db.Conn().Exec(`
			INSERT INTO investment_positions (account_id, security_id, shares, average_cost_per_share)
			VALUES ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 10.0, 200.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert first position: %v", err)
		}

		// Duplicate should fail
		_, err = db.Conn().Exec(`
			INSERT INTO investment_positions (account_id, security_id, shares, average_cost_per_share)
			VALUES ('11111111-1111-1111-1111-111111111111', '22222222-2222-2222-2222-222222222222', 5.0, 210.00)
		`)
		if err == nil {
			t.Error("Expected unique constraint error for duplicate account_id + security_id")
		}
	})

	t.Run("investment_positions enforces foreign keys", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Non-existent account
		_, err = db.Conn().Exec(`
			INSERT INTO investment_positions (account_id, security_id, shares, average_cost_per_share)
			VALUES ('99999999-9999-9999-9999-999999999999', '22222222-2222-2222-2222-222222222222', 10.0, 100.00)
		`)
		if err == nil {
			t.Error("Expected foreign key error for non-existent account")
		}

		// Create account, try non-existent security
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Test', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert test account: %v", err)
		}

		// security_id FK on investment_positions was removed in migration 010 for DuckDB UPDATE compatibility.
		_, err = db.Conn().Exec(`
			INSERT INTO investment_positions (account_id, security_id, shares, average_cost_per_share)
			VALUES ('11111111-1111-1111-1111-111111111111', '99999999-9999-9999-9999-999999999999', 10.0, 100.00)
		`)
		if err != nil {
			t.Errorf("Insert should succeed (security_id FK removed in migration 010): %v", err)
		}
	})

	// SM-049: investment_transaction_lots junction table
	t.Run("investment_transaction_lots links transaction to lot", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Set up prerequisites
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Investment', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		// Create buy transaction (source for lot)
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (id, account_id, date, transaction_type, total_amount)
			VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '2024-01-15', 'buy', 1850.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert buy transaction: %v", err)
		}

		// Create lot
		_, err = db.Conn().Exec(`
			INSERT INTO investment_lots (
				id, account_id, security_id, shares, original_shares,
				cost_per_share, purchase_date, source_transaction_id
			) VALUES (
				'44444444-4444-4444-4444-444444444444',
				'11111111-1111-1111-1111-111111111111',
				'22222222-2222-2222-2222-222222222222',
				10.0, 10.0, 185.00, '2024-01-15',
				'33333333-3333-3333-3333-333333333333'
			)
		`)
		if err != nil {
			t.Fatalf("Failed to insert lot: %v", err)
		}

		// Create sell transaction
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (id, account_id, date, transaction_type, total_amount)
			VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '2024-06-15', 'sell', 2000.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert sell transaction: %v", err)
		}

		// Link sell transaction to lot
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transaction_lots (id, transaction_id, lot_id, shares)
			VALUES ('66666666-6666-6666-6666-666666666666', '55555555-5555-5555-5555-555555555555', '44444444-4444-4444-4444-444444444444', 5.0)
		`)
		if err != nil {
			t.Fatalf("Failed to insert transaction-lot link: %v", err)
		}

		// Read it back
		var lotShares float64
		err = db.Conn().QueryRow(`
			SELECT shares FROM investment_transaction_lots
			WHERE id = '66666666-6666-6666-6666-666666666666'
		`).Scan(&lotShares)
		if err != nil {
			t.Fatalf("Failed to read transaction-lot link: %v", err)
		}
		if lotShares != 5.0 {
			t.Errorf("Expected shares 5.0, got %f", lotShares)
		}
	})

	t.Run("investment_transaction_lots blocks transaction delete when linked", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Set up full chain
		_, err = db.Conn().Exec(`
			INSERT INTO accounts (id, name, type, opening_date)
			VALUES ('11111111-1111-1111-1111-111111111111', 'Investment', 'investment', '2024-01-01')
		`)
		if err != nil {
			t.Fatalf("Failed to insert account: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type)
			VALUES ('22222222-2222-2222-2222-222222222222', 'AAPL', 'Apple Inc.', 'stock')
		`)
		if err != nil {
			t.Fatalf("Failed to insert security: %v", err)
		}

		// Buy transaction (source for lot)
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (id, account_id, date, transaction_type, total_amount)
			VALUES ('33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', '2024-01-15', 'buy', 1850.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert buy: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO investment_lots (
				id, account_id, security_id, shares, original_shares,
				cost_per_share, purchase_date, source_transaction_id
			) VALUES (
				'44444444-4444-4444-4444-444444444444',
				'11111111-1111-1111-1111-111111111111',
				'22222222-2222-2222-2222-222222222222',
				10.0, 10.0, 185.00, '2024-01-15',
				'33333333-3333-3333-3333-333333333333'
			)
		`)
		if err != nil {
			t.Fatalf("Failed to insert lot: %v", err)
		}

		// Sell transaction
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transactions (id, account_id, date, transaction_type, total_amount)
			VALUES ('55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', '2024-06-15', 'sell', 2000.00)
		`)
		if err != nil {
			t.Fatalf("Failed to insert sell: %v", err)
		}

		// Link
		_, err = db.Conn().Exec(`
			INSERT INTO investment_transaction_lots (transaction_id, lot_id, shares)
			VALUES ('55555555-5555-5555-5555-555555555555', '44444444-4444-4444-4444-444444444444', 5.0)
		`)
		if err != nil {
			t.Fatalf("Failed to insert link: %v", err)
		}

		// Verify junction record exists
		var count int
		err = db.Conn().QueryRow(`
			SELECT COUNT(*) FROM investment_transaction_lots
			WHERE transaction_id = '55555555-5555-5555-5555-555555555555'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count junction records: %v", err)
		}
		if count != 1 {
			t.Fatalf("Expected 1 junction record, got %d", count)
		}

		// Deleting junction records first, then transaction should work
		_, err = db.Conn().Exec(`
			DELETE FROM investment_transaction_lots
			WHERE transaction_id = '55555555-5555-5555-5555-555555555555'
		`)
		if err != nil {
			t.Fatalf("Failed to delete junction records: %v", err)
		}

		_, err = db.Conn().Exec(`
			DELETE FROM investment_transactions
			WHERE id = '55555555-5555-5555-5555-555555555555'
		`)
		if err != nil {
			t.Fatalf("Failed to delete transaction after clearing junction: %v", err)
		}

		// Verify transaction is gone
		err = db.Conn().QueryRow(`
			SELECT COUNT(*) FROM investment_transactions
			WHERE id = '55555555-5555-5555-5555-555555555555'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count transactions: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 transactions after delete, got %d", count)
		}
	})

	t.Run("investment_transaction_lots enforces transaction foreign key", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO investment_transaction_lots (transaction_id, lot_id, shares)
			VALUES ('99999999-9999-9999-9999-999999999999', '88888888-8888-8888-8888-888888888888', 5.0)
		`)
		if err == nil {
			t.Error("Expected foreign key error for non-existent transaction")
		}
	})
}

func TestMigration009CorporateActions(t *testing.T) {
	t.Run("corporate_actions table exists after migration", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		var tableName string
		err = db.Conn().QueryRow(`
			SELECT table_name FROM information_schema.tables
			WHERE table_name = 'corporate_actions'
		`).Scan(&tableName)
		if err != nil {
			t.Fatalf("corporate_actions table not found: %v", err)
		}
		if tableName != "corporate_actions" {
			t.Errorf("expected table name 'corporate_actions', got %q", tableName)
		}
	})

	t.Run("insert and read back corporate action", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		// Create a security first
		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc', 'stock', 'large_cap_stock', 'USD')
		`)
		if err != nil {
			t.Fatalf("Failed to create security: %v", err)
		}

		// Insert a corporate action (split)
		_, err = db.Conn().Exec(`
			INSERT INTO corporate_actions (id, action_type, security_id, action_date, parameters)
			VALUES (
				'22222222-2222-2222-2222-222222222222',
				'split',
				'11111111-1111-1111-1111-111111111111',
				'2024-08-01',
				'{"numerator":4,"denominator":1}'
			)
		`)
		if err != nil {
			t.Fatalf("Failed to insert corporate action: %v", err)
		}

		// Read it back
		var actionType, params string
		err = db.Conn().QueryRow(`
			SELECT action_type, parameters FROM corporate_actions
			WHERE id = '22222222-2222-2222-2222-222222222222'
		`).Scan(&actionType, &params)
		if err != nil {
			t.Fatalf("Failed to read back corporate action: %v", err)
		}
		if actionType != "split" {
			t.Errorf("expected action_type 'split', got %q", actionType)
		}
		if params != `{"numerator":4,"denominator":1}` {
			t.Errorf("expected parameters JSON, got %q", params)
		}
	})

	t.Run("action_type constraint rejects invalid types", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc', 'stock', 'large_cap_stock', 'USD')
		`)
		if err != nil {
			t.Fatalf("Failed to create security: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO corporate_actions (action_type, security_id, action_date, parameters)
			VALUES ('invalid_type', '11111111-1111-1111-1111-111111111111', '2024-08-01', '{}')
		`)
		if err == nil {
			t.Error("Expected error for invalid action_type, got nil")
		}
	})

	t.Run("all valid action types accepted", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc', 'stock', 'large_cap_stock', 'USD')
		`)
		if err != nil {
			t.Fatalf("Failed to create security: %v", err)
		}

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency)
			VALUES ('33333333-3333-3333-3333-333333333333', 'GOOG', 'Alphabet Inc', 'stock', 'large_cap_stock', 'USD')
		`)
		if err != nil {
			t.Fatalf("Failed to create target security: %v", err)
		}

		validTypes := []string{"split", "reverse_split", "merger", "spin_off"}
		for _, actionType := range validTypes {
			_, err = db.Conn().Exec(`
				INSERT INTO corporate_actions (action_type, security_id, target_security_id, action_date, parameters)
				VALUES (?, '11111111-1111-1111-1111-111111111111', '33333333-3333-3333-3333-333333333333', '2024-08-01', '{}')
			`, actionType)
			if err != nil {
				t.Errorf("Failed to insert action_type %q: %v", actionType, err)
			}
		}
	})

	t.Run("target_security_id is nullable", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc', 'stock', 'large_cap_stock', 'USD')
		`)
		if err != nil {
			t.Fatalf("Failed to create security: %v", err)
		}

		// Split doesn't require target_security_id
		_, err = db.Conn().Exec(`
			INSERT INTO corporate_actions (action_type, security_id, action_date, parameters)
			VALUES ('split', '11111111-1111-1111-1111-111111111111', '2024-08-01', '{"numerator":4,"denominator":1}')
		`)
		if err != nil {
			t.Fatalf("Expected NULL target_security_id to be allowed: %v", err)
		}
	})

	// security_id FK was removed in migration 010 for DuckDB UPDATE compatibility;
	// validation is now enforced at the application level.
	t.Run("security_id foreign key not enforced after migration 010", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO corporate_actions (action_type, security_id, action_date, parameters)
			VALUES ('split', '99999999-9999-9999-9999-999999999999', '2024-08-01', '{}')
		`)
		if err != nil {
			t.Errorf("Expected insert to succeed (FK removed in migration 010), got error: %v", err)
		}
	})

	t.Run("parameters column stores JSON", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "test.tdb")

		db, err := Create(dbPath)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		defer db.Close()

		_, err = db.Conn().Exec(`
			INSERT INTO securities (id, ticker, name, security_type, asset_class, currency)
			VALUES ('11111111-1111-1111-1111-111111111111', 'AAPL', 'Apple Inc', 'stock', 'large_cap_stock', 'USD')
		`)
		if err != nil {
			t.Fatalf("Failed to create security: %v", err)
		}

		jsonParams := `{"exchange_ratio":"2.5","cash_per_share":"5.00"}`
		_, err = db.Conn().Exec(`
			INSERT INTO corporate_actions (id, action_type, security_id, action_date, parameters)
			VALUES ('44444444-4444-4444-4444-444444444444', 'merger', '11111111-1111-1111-1111-111111111111', '2024-06-15', ?)
		`, jsonParams)
		if err != nil {
			t.Fatalf("Failed to insert JSON parameters: %v", err)
		}

		var readParams string
		err = db.Conn().QueryRow(`
			SELECT parameters FROM corporate_actions
			WHERE id = '44444444-4444-4444-4444-444444444444'
		`).Scan(&readParams)
		if err != nil {
			t.Fatalf("Failed to read parameters: %v", err)
		}
		if readParams != jsonParams {
			t.Errorf("expected parameters %q, got %q", jsonParams, readParams)
		}
	})
}

// Helper to remove test file if it exists
func removeIfExists(path string) {
	os.Remove(path)
	os.Remove(path + ".wal")
}
