package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/haskovec/tmoney/internal/db"
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

	// partialInfix marks a file that a copy is still writing: the copy writes
	// <final name>.partial-<random> and renames it to <final name> only when it
	// is complete and on disk. Such a name never parses as a backup (see
	// parseBackupFilename), so ListBackups and retention skip it.
	partialInfix = ".partial-"

	// stalePartialAge is how old a partial file must be before enforceRetention
	// deletes it as the leftover of a killed copy.
	stalePartialAge = time.Hour
)

// validateDatabase checks that a file is a TMoney database before Restore puts
// it in place of the database. A var so that tests with fake file content can
// replace it.
var validateDatabase = db.ValidateFile

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
	return createAutoBackupAt(dbPath, time.Now(), "")
}

// createAutoBackupAt is the internal implementation that accepts a timestamp for
// testability. keep names an auto-backup that retention must not delete ("" for
// none); Restore passes the backup it is restoring from.
func createAutoBackupAt(dbPath string, now time.Time, keep string) (string, error) {
	backupPath := autoBackupPath(dbPath, now)

	if err := copyDatabase(dbPath, backupPath); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Non-fatal: the backup exists even if retention cleanup fails.
	_ = enforceRetention(dbPath, keep)

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

	if err := copyDatabase(dbPath, backupPath); err != nil {
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
// safe process: copy the backup to a temp file beside the database, check that
// the copy is a TMoney database, safety-backup the current state, and rename
// the copy over the database. The safety backup is an auto-backup, but its
// retention pass never deletes backupPath, so the source of a restore survives
// whatever happens next.
//
// The rename is the only step that changes the database file, and it replaces
// it in one step: if the process dies, dbPath holds the old database or the
// complete copy, never a part of one and never nothing.
//
// The database at dbPath must be closed while this runs (see db.DB.Close and
// db.DB.WithFileClosed).
func Restore(dbPath, backupPath string) (safetyBackupPath string, err error) {
	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup file not found: %s", backupPath)
	}

	// 1. Copy the backup beside the database, before anything can touch the
	//    set of backups.
	tempPath, err := copyToTemp(backupPath, dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to copy backup to temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tempPath)
		}
	}()

	// 2. Check the copy, which is what goes in place: a backup cut short by a
	//    crash, or a file that is not a TMoney database, stops here.
	if err := validateDatabase(tempPath); err != nil {
		return "", fmt.Errorf("backup file %s is not a valid TMoney database: %w", backupPath, err)
	}

	// 3. Safety backup of the current state, keeping backupPath out of retention.
	safetyPath, err := createAutoBackupAt(dbPath, time.Now(), backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create safety backup before restore: %w", err)
	}

	// 4. Rename the copy over the database.
	if err := os.Rename(tempPath, dbPath); err != nil {
		return safetyPath, fmt.Errorf("failed to move restored file into place: %w", err)
	}
	syncDir(filepath.Dir(dbPath))

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

// enforceRetention deletes the oldest auto-backups if there are more than
// MaxAutoBackups. The backup at keep (if any) is never deleted and does not
// count toward the limit.
func enforceRetention(dbPath, keep string) error {
	removeStalePartials(dbPath, time.Now())

	backups, err := ListBackups(dbPath)
	if err != nil {
		return err
	}

	// Filter to auto-backups only
	var autoBackups []BackupInfo
	for _, b := range backups {
		if b.Type == BackupTypeAuto && !samePath(b.Path, keep) {
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

// samePath reports whether a and b name the same file, tolerating a relative
// spelling on one side. It is a cleaned-path comparison, not an inode check.
func samePath(a, b string) bool {
	if b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return absA == absB
}

// removeStalePartials deletes the partial files of dbPath (see partialInfix)
// that were last written more than stalePartialAge before now: a crash or a
// kill left them. It also deletes the "tmoney-restore-*.tmp" files that a
// killed Restore left before it named its copies this way. Best effort: a file
// it cannot delete stays, and is harmless, because no listing shows it.
func removeStalePartials(dbPath string, now time.Time) {
	dir, base := filepath.Dir(dbPath), filepath.Base(dbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		ours := isPartialOf(base, name) || isLegacyRestoreTemp(name)
		if !ours || !entry.Type().IsRegular() {
			continue
		}
		if info, err := entry.Info(); err == nil && now.Sub(info.ModTime()) >= stalePartialAge {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// isPartialOf reports whether name is a partial file of a copy to dbBase
// (Restore) or to one of dbBase's backups.
func isPartialOf(dbBase, name string) bool {
	i := strings.LastIndex(name, partialInfix)
	if i < 0 {
		return false
	}
	final := name[:i]
	_, ok := parseBackupFilename(dbBase, final)
	return ok || final == dbBase
}

// isLegacyRestoreTemp reports whether name is a temp file of Restore from
// before the partial names.
func isLegacyRestoreTemp(name string) bool {
	return strings.HasPrefix(name, "tmoney-restore-") && strings.HasSuffix(name, ".tmp")
}

// copyDatabase copies the database at src to dst so that dst is never a part
// of a copy: it writes a partial file beside dst (see copyToTemp) and renames
// it to dst. If the process dies first, dst is as it was (absent, or the
// earlier file of that name) and only the partial file is left.
func copyDatabase(src, dst string) error {
	tempPath, err := copyToTemp(src, dst)
	if err != nil {
		return err
	}
	if err := os.Rename(tempPath, dst); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	syncDir(filepath.Dir(dst))
	return nil
}

// copyToTemp copies the database at src to a new partial file beside dst,
// flushed to disk and closed, and returns its path. On error it removes what it
// wrote.
func copyToTemp(src, dst string) (_ string, err error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return "", err
	}

	out, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+partialInfix+"*")
	if err != nil {
		return "", err
	}
	tempPath := out.Name()
	defer func() {
		if err != nil {
			_ = out.Close()
			_ = os.Remove(tempPath)
		}
	}()
	// CreateTemp makes the file 0600; keep the source's permissions, as the
	// copy had before it went through a temp file.
	if err := out.Chmod(info.Mode().Perm()); err != nil {
		return "", err
	}
	n, err := io.Copy(out, in)
	if err != nil {
		return "", err
	}
	if n != info.Size() {
		return "", fmt.Errorf("copied %d bytes of %s, which had %d: it changed during the copy", n, src, info.Size())
	}
	// The data must be on disk before the rename gives it its final name, or
	// a power loss can leave that name on a file with holes.
	if err := out.Sync(); err != nil {
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return tempPath, nil
}

// syncDir flushes the directory dir, so that a rename into it is on disk too.
// Best effort: the renamed file is already complete, so a failure here can at
// worst lose the rename on a power loss, which leaves the earlier state, not a
// torn file. Windows cannot flush a directory handle, so it does nothing there.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
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
