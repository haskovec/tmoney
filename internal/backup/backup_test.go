package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func createTestDB(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "test.tdb")
	if err := os.WriteFile(path, []byte("test database content"), 0644); err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	return path
}

func TestCreateAutoBackup(t *testing.T) {
	t.Run("creates backup file with correct naming", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)
		now := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

		backupPath, err := createAutoBackupAt(dbPath, now, "")
		if err != nil {
			t.Fatalf("createAutoBackupAt() error = %v", err)
		}

		expected := dbPath + ".backup.2024-03-15T14-30-00"
		if backupPath != expected {
			t.Errorf("backup path = %v, want %v", backupPath, expected)
		}

		// Verify file exists
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Error("backup file was not created")
		}

		// Verify content matches
		original, _ := os.ReadFile(dbPath)
		backup, _ := os.ReadFile(backupPath)
		if string(original) != string(backup) {
			t.Error("backup content does not match original")
		}
	})

	t.Run("enforces rolling retention of 5 auto-backups", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		// Create 7 auto-backups
		baseTime := time.Date(2024, 3, 10, 10, 0, 0, 0, time.UTC)
		for i := range 7 {
			ts := baseTime.Add(time.Duration(i) * 24 * time.Hour)
			_, err := createAutoBackupAt(dbPath, ts, "")
			if err != nil {
				t.Fatalf("createAutoBackupAt() error on backup %d: %v", i, err)
			}
		}

		// List backups
		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		autoCount := 0
		for _, b := range backups {
			if b.Type == BackupTypeAuto {
				autoCount++
			}
		}

		if autoCount != MaxAutoBackups {
			t.Errorf("auto backup count = %d, want %d", autoCount, MaxAutoBackups)
		}

		// Verify the oldest 2 were deleted (days 10 and 11)
		oldest1 := dbPath + ".backup.2024-03-10T10-00-00"
		oldest2 := dbPath + ".backup.2024-03-11T10-00-00"
		if _, err := os.Stat(oldest1); !os.IsNotExist(err) {
			t.Errorf("oldest backup should have been deleted: %s", oldest1)
		}
		if _, err := os.Stat(oldest2); !os.IsNotExist(err) {
			t.Errorf("second oldest backup should have been deleted: %s", oldest2)
		}

		// Verify newest 5 still exist
		for i := 2; i < 7; i++ {
			ts := baseTime.Add(time.Duration(i) * 24 * time.Hour)
			path := dbPath + ".backup." + ts.Format(TimestampFormat)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Errorf("backup %d should still exist: %s", i, path)
			}
		}
	})

	t.Run("manual backups do not count against retention limit", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		// Create 3 manual backups
		baseTime := time.Date(2024, 3, 1, 10, 0, 0, 0, time.UTC)
		for i := range 3 {
			ts := baseTime.Add(time.Duration(i) * 24 * time.Hour)
			_, err := createManualBackupAt(dbPath, ts)
			if err != nil {
				t.Fatalf("createManualBackupAt() error: %v", err)
			}
		}

		// Create 6 auto-backups (should retain only 5)
		autoBase := time.Date(2024, 3, 10, 10, 0, 0, 0, time.UTC)
		for i := range 6 {
			ts := autoBase.Add(time.Duration(i) * 24 * time.Hour)
			_, err := createAutoBackupAt(dbPath, ts, "")
			if err != nil {
				t.Fatalf("createAutoBackupAt() error: %v", err)
			}
		}

		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		autoCount := 0
		manualCount := 0
		for _, b := range backups {
			switch b.Type {
			case BackupTypeAuto:
				autoCount++
			case BackupTypeManual:
				manualCount++
			}
		}

		if autoCount != MaxAutoBackups {
			t.Errorf("auto backup count = %d, want %d", autoCount, MaxAutoBackups)
		}
		if manualCount != 3 {
			t.Errorf("manual backup count = %d, want 3", manualCount)
		}
	})
}

func TestCreateManualBackup(t *testing.T) {
	t.Run("creates backup with manual-backup infix", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)
		now := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)

		backupPath, err := createManualBackupAt(dbPath, now)
		if err != nil {
			t.Fatalf("createManualBackupAt() error = %v", err)
		}

		expected := dbPath + ".manual-backup.2024-03-15T14-30-00"
		if backupPath != expected {
			t.Errorf("backup path = %v, want %v", backupPath, expected)
		}

		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Error("manual backup file was not created")
		}
	})
}

func TestListBackups(t *testing.T) {
	t.Run("lists auto and manual backups sorted newest first", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		// Create backups in mixed order
		times := []time.Time{
			time.Date(2024, 3, 12, 10, 0, 0, 0, time.UTC),
			time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC),
			time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC),
		}

		createAutoBackupAt(dbPath, times[0], "")
		createManualBackupAt(dbPath, times[1])
		createAutoBackupAt(dbPath, times[2], "")

		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		if len(backups) != 3 {
			t.Fatalf("ListBackups() returned %d backups, want 3", len(backups))
		}

		// Verify sorted newest first
		if !backups[0].Timestamp.Equal(times[1]) {
			t.Errorf("first backup timestamp = %v, want %v", backups[0].Timestamp, times[1])
		}
		if backups[0].Type != BackupTypeManual {
			t.Errorf("first backup type = %v, want Manual", backups[0].Type)
		}

		if !backups[1].Timestamp.Equal(times[0]) {
			t.Errorf("second backup timestamp = %v, want %v", backups[1].Timestamp, times[0])
		}

		if !backups[2].Timestamp.Equal(times[2]) {
			t.Errorf("third backup timestamp = %v, want %v", backups[2].Timestamp, times[2])
		}
	})

	t.Run("returns empty list when no backups exist", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		if len(backups) != 0 {
			t.Errorf("ListBackups() returned %d backups, want 0", len(backups))
		}
	})

	t.Run("ignores unrelated files", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		// Create an unrelated file
		os.WriteFile(filepath.Join(dir, "other.tdb.backup.2024-03-15T14-30-00"), []byte("other"), 0644)
		os.WriteFile(filepath.Join(dir, "test.tdb.somethingelse"), []byte("unrelated"), 0644)

		// Create one real backup
		createAutoBackupAt(dbPath, time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC), "")

		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		if len(backups) != 1 {
			t.Errorf("ListBackups() returned %d backups, want 1", len(backups))
		}
	})

	t.Run("backups have correct size", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		createAutoBackupAt(dbPath, time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC), "")

		backups, err := ListBackups(dbPath)
		if err != nil {
			t.Fatalf("ListBackups() error = %v", err)
		}

		if len(backups) != 1 {
			t.Fatalf("ListBackups() returned %d backups, want 1", len(backups))
		}

		originalInfo, _ := os.Stat(dbPath)
		if backups[0].Size != originalInfo.Size() {
			t.Errorf("backup size = %d, want %d", backups[0].Size, originalInfo.Size())
		}
	})
}

func TestRestore(t *testing.T) {
	t.Run("restores database from backup", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		// Create a backup
		backupPath, err := createAutoBackupAt(dbPath, time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC), "")
		if err != nil {
			t.Fatalf("createAutoBackupAt() error = %v", err)
		}

		// Modify the original database
		os.WriteFile(dbPath, []byte("modified content"), 0644)

		// Restore from backup
		safetyPath, err := Restore(dbPath, backupPath)
		if err != nil {
			t.Fatalf("Restore() error = %v", err)
		}

		// Verify the database was restored
		content, _ := os.ReadFile(dbPath)
		if string(content) != "test database content" {
			t.Errorf("restored content = %q, want %q", string(content), "test database content")
		}

		// Verify safety backup was created
		if safetyPath == "" {
			t.Error("Restore() did not return a safety backup path")
		}
		if _, err := os.Stat(safetyPath); os.IsNotExist(err) {
			t.Error("safety backup file was not created")
		}

		// Verify safety backup contains the modified content
		safetyContent, _ := os.ReadFile(safetyPath)
		if string(safetyContent) != "modified content" {
			t.Errorf("safety backup content = %q, want %q", string(safetyContent), "modified content")
		}

		// Verify .restoring file was cleaned up
		restoringPath := dbPath + restoringExtension
		if _, err := os.Stat(restoringPath); !os.IsNotExist(err) {
			t.Error(".restoring file should have been cleaned up")
		}
	})

	t.Run("restoring the oldest of a full auto-backup set does not lose it to retention", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		// Fill the retention window: MaxAutoBackups auto-backups already exist.
		// The oldest one holds the content we want back.
		baseTime := time.Date(2024, 3, 10, 10, 0, 0, 0, time.UTC)
		oldest, err := createAutoBackupAt(dbPath, baseTime, "")
		if err != nil {
			t.Fatalf("createAutoBackupAt() error = %v", err)
		}
		os.WriteFile(dbPath, []byte("modified content"), 0644)
		for i := 1; i < MaxAutoBackups; i++ {
			if _, err := createAutoBackupAt(dbPath, baseTime.Add(time.Duration(i)*24*time.Hour), ""); err != nil {
				t.Fatalf("createAutoBackupAt() error = %v", err)
			}
		}

		// The safety backup Restore takes is the (MaxAutoBackups+1)th auto-backup.
		// Retention must skip the restore source, so it deletes nothing here.
		if _, err := Restore(dbPath, oldest); err != nil {
			t.Fatalf("Restore() error = %v", err)
		}

		content, _ := os.ReadFile(dbPath)
		if string(content) != "test database content" {
			t.Errorf("restored content = %q, want %q", string(content), "test database content")
		}
		if _, err := os.Stat(oldest); err != nil {
			t.Errorf("the restore source must survive retention, but: %v", err)
		}
	})

	t.Run("returns error for non-existent backup", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := createTestDB(t, dir)

		_, err := Restore(dbPath, "/nonexistent/backup")
		if err == nil {
			t.Fatal("Restore() expected error for non-existent backup")
		}
	})
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{2621440, "2.5 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatSize(tt.bytes)
			if result != tt.expected {
				t.Errorf("FormatSize(%d) = %v, want %v", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestParseBackupFilename(t *testing.T) {
	t.Run("parses auto-backup filename", func(t *testing.T) {
		info, ok := parseBackupFilename("test.tdb", "test.tdb.backup.2024-03-15T14-30-00")
		if !ok {
			t.Fatal("parseBackupFilename() returned false for valid auto-backup")
		}
		if info.Type != BackupTypeAuto {
			t.Errorf("type = %v, want Auto", info.Type)
		}
		expected := time.Date(2024, 3, 15, 14, 30, 0, 0, time.UTC)
		if !info.Timestamp.Equal(expected) {
			t.Errorf("timestamp = %v, want %v", info.Timestamp, expected)
		}
	})

	t.Run("parses manual-backup filename", func(t *testing.T) {
		info, ok := parseBackupFilename("test.tdb", "test.tdb.manual-backup.2024-03-15T14-30-00")
		if !ok {
			t.Fatal("parseBackupFilename() returned false for valid manual-backup")
		}
		if info.Type != BackupTypeManual {
			t.Errorf("type = %v, want Manual", info.Type)
		}
	})

	t.Run("rejects unrelated filenames", func(t *testing.T) {
		_, ok := parseBackupFilename("test.tdb", "other.tdb.backup.2024-03-15T14-30-00")
		if ok {
			t.Error("parseBackupFilename() should reject filename with different base")
		}
	})

	t.Run("rejects invalid timestamps", func(t *testing.T) {
		_, ok := parseBackupFilename("test.tdb", "test.tdb.backup.not-a-timestamp")
		if ok {
			t.Error("parseBackupFilename() should reject invalid timestamp")
		}
	})
}
