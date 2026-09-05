package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// AutoBackupInfix is the infix used for auto-backup filenames.
	AutoBackupInfix = ".backup."

	// ManualBackupInfix is the infix used for manual backup filenames.
	ManualBackupInfix = ".manual-backup."

	// TimestampFormat is the filesystem-safe timestamp format for backup names.
	TimestampFormat = "2006-01-02T15-04-05"

	// MaxAutoBackups is the rolling retention limit for auto-backups.
	MaxAutoBackups = 5

	// restoringExtension is the temporary extension used during restore.
	restoringExtension = ".restoring"
)

// BackupType distinguishes auto from manual backups.
type BackupType string

const (
	BackupTypeAuto   BackupType = "Auto"
	BackupTypeManual BackupType = "Manual"
)

// BackupInfo describes an existing backup file.
type BackupInfo struct {
	Path      string
	Timestamp time.Time
	Size      int64
	Type      BackupType
}

// CreateAutoBackup creates an auto-backup of the given database file and
// enforces rolling retention (keeping at most MaxAutoBackups auto-backups).
func CreateAutoBackup(dbPath string) (string, error) {
	return createAutoBackupAt(dbPath, time.Now())
}

// createAutoBackupAt is the internal implementation that accepts a timestamp for testability.
func createAutoBackupAt(dbPath string, now time.Time) (string, error) {
	backupPath := autoBackupPath(dbPath, now)

	if err := copyFile(dbPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	if err := enforceRetention(dbPath); err != nil {
		// Non-fatal: backup was created, retention cleanup failed
		return backupPath, nil
	}

	return backupPath, nil
}

// CreateManualBackup creates a manual backup of the given database file.
// Manual backups are never auto-deleted by rolling retention.
func CreateManualBackup(dbPath string) (string, error) {
	return createManualBackupAt(dbPath, time.Now())
}

// createManualBackupAt is the internal implementation that accepts a timestamp for testability.
func createManualBackupAt(dbPath string, now time.Time) (string, error) {
	backupPath := manualBackupPath(dbPath, now)

	if err := copyFile(dbPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to create manual backup: %w", err)
	}

	return backupPath, nil
}

// ListBackups returns all backups for the given database file, sorted newest first.
func ListBackups(dbPath string) ([]BackupInfo, error) {
	dir := filepath.Dir(dbPath)
	base := filepath.Base(dbPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		info, ok := parseBackupFilename(base, name)
		if !ok {
			continue
		}

		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}
		info.Path = filepath.Join(dir, name)
		info.Size = fileInfo.Size()

		backups = append(backups, info)
	}

	// Sort newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// Restore replaces the database file with the specified backup file using a
// safe process: copy to temp, safety-backup the current state, swap, cleanup.
//
// The database at dbPath must be CLOSED when this runs. On Windows the rename of
// an open file fails outright; on other platforms it succeeds, but DuckDB caches
// instances by path, so a reopen would hand back the still-open OLD instance and
// the restored file would never be read.
func Restore(dbPath, backupPath string) (safetyBackupPath string, err error) {
	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup file not found: %s", backupPath)
	}

	// Safe restore process:
	// 1. Copy backup to a temp file in the same directory. This runs BEFORE the
	//    safety backup on purpose: the safety backup is an auto-backup, and
	//    auto-backups enforce rolling retention. When the file being restored is
	//    itself the oldest of a full set of auto-backups, retention deletes it —
	//    so the copy has to be in hand before retention can run.
	dir := filepath.Dir(dbPath)
	tempFile, err := os.CreateTemp(dir, "tmoney-restore-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for restore: %w", err)
	}
	tempPath := tempFile.Name()
	_ = tempFile.Close()

	if err := copyFile(backupPath, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("failed to copy backup to temp file: %w", err)
	}

	// Create a safety backup of the current state
	safetyPath, err := CreateAutoBackup(dbPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("failed to create safety backup before restore: %w", err)
	}

	// 2. Rename current database to .restoring
	restoringPath := dbPath + restoringExtension
	if err := os.Rename(dbPath, restoringPath); err != nil {
		_ = os.Remove(tempPath)
		return safetyPath, fmt.Errorf("failed to move current database aside: %w", err)
	}

	// 3. Rename temp file to database name
	if err := os.Rename(tempPath, dbPath); err != nil {
		// Roll back: restore the .restoring file. If the rollback itself
		// fails the original database is still accessible at restoringPath;
		// surface both errors so the user can recover manually.
		_ = os.Remove(tempPath)
		if rollbackErr := os.Rename(restoringPath, dbPath); rollbackErr != nil {
			return safetyPath, fmt.Errorf("failed to move restored file into place: %w; rollback also failed, original database is at %s: %v", err, restoringPath, rollbackErr)
		}
		return safetyPath, fmt.Errorf("failed to move restored file into place: %w", err)
	}

	// 4. Delete the .restoring file
	_ = os.Remove(restoringPath)

	return safetyPath, nil
}

// autoBackupPath returns the backup file path for an auto-backup.
func autoBackupPath(dbPath string, t time.Time) string {
	return dbPath + AutoBackupInfix + t.Format(TimestampFormat)
}

// manualBackupPath returns the backup file path for a manual backup.
func manualBackupPath(dbPath string, t time.Time) string {
	return dbPath + ManualBackupInfix + t.Format(TimestampFormat)
}

// parseBackupFilename checks if a filename is a backup of the given base name
// and returns its info if so.
func parseBackupFilename(dbBase, filename string) (BackupInfo, bool) {
	var info BackupInfo

	if tsStr, ok := strings.CutPrefix(filename, dbBase+ManualBackupInfix); ok {
		t, err := time.Parse(TimestampFormat, tsStr)
		if err != nil {
			return info, false
		}
		info.Timestamp = t
		info.Type = BackupTypeManual
		return info, true
	}

	if tsStr, ok := strings.CutPrefix(filename, dbBase+AutoBackupInfix); ok {
		t, err := time.Parse(TimestampFormat, tsStr)
		if err != nil {
			return info, false
		}
		info.Timestamp = t
		info.Type = BackupTypeAuto
		return info, true
	}

	return info, false
}

// enforceRetention deletes the oldest auto-backups if there are more than MaxAutoBackups.
func enforceRetention(dbPath string) error {
	backups, err := ListBackups(dbPath)
	if err != nil {
		return err
	}

	// Filter to auto-backups only
	var autoBackups []BackupInfo
	for _, b := range backups {
		if b.Type == BackupTypeAuto {
			autoBackups = append(autoBackups, b)
		}
	}

	// Already sorted newest first; delete any beyond the limit
	if len(autoBackups) <= MaxAutoBackups {
		return nil
	}

	for _, b := range autoBackups[MaxAutoBackups:] {
		if err := os.Remove(b.Path); err != nil {
			return fmt.Errorf("failed to remove old backup %s: %w", b.Path, err)
		}
	}

	return nil
}

// copyFile copies src to dst. If dst already exists it is overwritten.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = srcFile.Close() }()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		_ = dstFile.Close()
		return err
	}
	if err := dstFile.Sync(); err != nil {
		_ = dstFile.Close()
		return err
	}
	return dstFile.Close()
}

// FormatSize formats a file size in human-readable form.
func FormatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
