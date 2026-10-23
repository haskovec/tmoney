package backup

import (
	"errors"
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
	// <final name>.tmoney-partial-<digits> (os.CreateTemp puts random digits
	// for its "*") and renames it to <final name> only when it is complete and
	// on disk. Such a name never parses as a backup (see parseBackupFilename),
	// so ListBackups and retention skip it. The sweep of stale partial files
	// deletes by name, so the infix names the app: a user can name a file
	// "<db>.partial-2025", but not likely "<db>.tmoney-partial-2025".
	partialInfix = ".tmoney-partial-"

	// stalePartialAge is how old a partial file must be before enforceRetention
	// deletes it as the leftover of a killed copy.
	stalePartialAge = time.Hour
)

// validateDatabase checks, without changing it, that a file is a TMoney
// database before Restore puts it in place of the database. A var so that
// tests with fake file content can replace it.
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
	return createAutoBackupAt(dbPath, time.Now())
}

// createAutoBackupAt is the internal implementation that accepts a timestamp for
// testability. keep names files that the retention pass must not delete;
// Restore passes the backup it is restoring from and its own copy of it.
func createAutoBackupAt(dbPath string, now time.Time, keep ...string) (string, error) {
	backupPath, err := copyDatabase(dbPath, func() string { return freeBackupPath(dbPath, AutoBackupInfix, now) }, true)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	// Non-fatal: the backup exists even if retention cleanup fails.
	_ = enforceRetention(dbPath, keep...)

	return backupPath, nil
}

// CreateManualBackup creates a manual backup of the given database file.
// Manual backups are never auto-deleted by rolling retention.
func CreateManualBackup(dbPath string) (string, error) {
	return createManualBackupAt(dbPath, time.Now())
}

// createManualBackupAt is the internal implementation that accepts a timestamp for testability.
func createManualBackupAt(dbPath string, now time.Time) (string, error) {
	backupPath, err := copyDatabase(dbPath, func() string { return freeBackupPath(dbPath, ManualBackupInfix, now) }, true)
	if err != nil {
		return "", fmt.Errorf("failed to create manual backup: %w", err)
	}

	return backupPath, nil
}

// createRawSafetyBackup is the safety backup of Restore when the WAL files of
// the database cannot be folded into it (a damaged log, which DuckDB cannot
// open the database with either, or a read-only file): an auto-backup of the
// file alone, with raw copies of the WAL files beside it (<backup>.wal and so
// on), so that no byte of the current state is lost. A restore of that backup
// applies a good log; it refuses a damaged one and says to move it away, and
// then the file alone restores.
func createRawSafetyBackup(dbPath string, now time.Time, keep ...string) (string, error) {
	backupPath, err := copyDatabase(dbPath, func() string { return freeBackupPath(dbPath, AutoBackupInfix, now) }, false)
	if err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}
	_ = enforceRetention(dbPath, keep...)
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
// safe process: check that the backup is a TMoney database, copy it to a temp
// file beside the database and check the copy, safety-backup the current
// state, and rename the copy over the database. The safety backup is an
// auto-backup, but it never takes the name of an existing backup, and its
// retention pass never deletes backupPath, so the source of a restore survives
// whatever happens next.
//
// The rename replaces the database file in one step: if the process dies,
// dbPath holds the old database or the complete copy, never a part of one and
// never nothing. Before it, the WAL files of the current database (see
// db.WALSuffixes) are folded into it, so that the old database is whole by
// itself; DuckDB would replay them onto the restored file. If they cannot be
// folded, the safety backup keeps raw copies of them, and they move aside.
// Restore refuses a database that another process has open.
//
// The database at dbPath must be closed while this runs (see db.DB.Close and
// db.DB.WithFileClosed).
func Restore(dbPath, backupPath string) (safetyBackupPath string, err error) {
	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return "", fmt.Errorf("backup file not found: %s", backupPath)
	}

	// 1. Check the backup: a backup cut short by a crash, a file that is not a
	//    TMoney database, one of a newer app version, one that this user
	//    cannot read, or one that another program has open stops here, and the
	//    reason names the file the user chose.
	if err := validateDatabase(backupPath); err != nil {
		return "", checkFailed("backup file "+backupPath, err, walHint(backupPath))
	}

	// 2. Copy the backup beside the database, with the mode of the database
	//    (not of the backup, which can be read-only), before anything can
	//    touch the set of backups.
	tempPath, err := copyToTemp(backupPath, dbPath, dbPath, true)
	if err != nil {
		return "", fmt.Errorf("failed to copy backup to temp file: %w", err)
	}
	defer func() {
		if err != nil {
			removeCopy(tempPath)
		}
	}()

	// 3. Check the copy too, which is what goes in place.
	if err := validateDatabase(tempPath); err != nil {
		return "", checkFailed("the copy of backup file "+backupPath, err, "")
	}

	// 4. Fold the WAL files of the current database into it, in place: then
	//    the file alone holds every committed row, and a kill before the
	//    rename below leaves the database whole. The read-write open also
	//    shows another process that has the database open (a TMoney in
	//    another terminal): a swap under it would lose its later writes, so
	//    Restore stops here, before it changes anything. A damaged log, which
	//    DuckDB cannot open the database with either, or a read-only file
	//    fails the fold; the safety backup then keeps raw copies of the logs.
	foldErr := db.CheckpointFile(dbPath)
	if db.IsLockedElsewhere(foldErr) {
		return "", fmt.Errorf("the database is open in another program; close it and try again: %w", foldErr)
	}

	// 5. Safety backup of the current state. Its retention pass keeps
	//    backupPath and the copy.
	var safetyPath string
	if foldErr == nil {
		safetyPath, err = createAutoBackupAt(dbPath, time.Now(), backupPath, tempPath)
	} else {
		safetyPath, err = createRawSafetyBackup(dbPath, time.Now(), backupPath, tempPath)
	}
	if err != nil {
		return "", fmt.Errorf("failed to create safety backup before restore: %w", err)
	}

	// 6. Move the logs that the fold could not apply aside, rename the copy
	//    over the database, and delete the moved logs: the safety backup has
	//    them. If the rename fails, the logs move back.
	moved, err := moveWALAside(dbPath, tempPath)
	if err != nil {
		return safetyPath, fmt.Errorf("failed to move the write-ahead log of the current database aside: %w", err)
	}
	if beforeSwap != nil {
		beforeSwap(dbPath)
	}
	if err := os.Rename(tempPath, dbPath); err != nil {
		moveBack(moved)
		return safetyPath, fmt.Errorf("failed to move restored file into place: %w", err)
	}
	syncDir(filepath.Dir(dbPath))
	for to := range moved {
		_ = os.Remove(to)
	}

	return safetyPath, nil
}

// checkFailed is the error of Restore when the check of a file fails. file
// says which file, by the backup that the user chose. Only a file that is not
// a TMoney database is called invalid: a backup of a newer app needs a newer
// TMoney, one that this user cannot read needs other permissions, and one
// that another program has open needs that program closed. hint follows the
// error of an invalid file only.
func checkFailed(file string, err error, hint string) error {
	var newer *db.VersionMismatchError
	var unreadable *db.UnreadableFileError
	var locked *db.LockedFileError
	switch {
	case errors.As(err, &newer):
		return fmt.Errorf("cannot restore %s: %w", file, err)
	case errors.As(err, &unreadable):
		return fmt.Errorf("cannot read %s: %w", file, err)
	case errors.As(err, &locked):
		return fmt.Errorf("%s is open in another program; close it and try again: %w", file, err)
	default:
		return fmt.Errorf("%s is not a valid TMoney database: %w%s", file, err, hint)
	}
}

// walHint is the hint of checkFailed for a backup with a log beside it: a
// raw safety backup keeps a log that could not be applied, and without it the
// file alone restores.
func walHint(backupPath string) string {
	if !db.HasWAL(backupPath) {
		return ""
	}
	return fmt.Sprintf("; if its write-ahead log cannot be applied, move %s.wal away to restore the file alone", backupPath)
}

// beforeSwap, when set, runs right before Restore renames its copy over the
// database. A test hook (see export_test.go).
var beforeSwap func(dbPath string)

// moveWALAside renames the WAL files of dbPath to <tempPath><suffix>-old and
// returns the moves, as a map from the new name to the old one. On an error
// it moves back what it moved.
func moveWALAside(dbPath, tempPath string) (map[string]string, error) {
	moved := map[string]string{}
	for _, suffix := range db.WALSuffixes {
		from, to := dbPath+suffix, tempPath+suffix+"-old"
		if err := os.Rename(from, to); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			moveBack(moved)
			return nil, err
		}
		moved[to] = from
	}
	return moved, nil
}

// moveBack undoes moveWALAside.
func moveBack(moved map[string]string) {
	for to, from := range moved {
		_ = os.Rename(to, from)
	}
}

// freeBackupPath returns the name of a new backup of dbPath of the kind that
// infix marks (AutoBackupInfix or ManualBackupInfix): at the time now, or one
// second after the newest backup of that kind if that is later. Names have
// one-second resolution and retention deletes by name order, so a new backup
// must sort after all the others: a name in a second that retention freed
// would sort as the oldest, and its own retention pass would delete it. More
// than one backup per second therefore pushes the names ahead of the clock.
// With publish, a backup never replaces another one, such as the safety backup
// of a restore.
//
// A name that is ahead of the clock makes the new names follow it, ahead of
// their real time, until the clock passes it: a clock that was ahead and went
// back, a move to a time zone to the west (the names hold the local wall
// clock), or a backup copied in from another machine. Only the times in the
// list are wrong; to make them right again, rename or delete the names of that
// kind that are ahead of the clock. A cap that ignores names far ahead would
// lose backups: with MaxAutoBackups auto-backups ahead, a new one would sort
// as the oldest, and its own retention pass would delete it.
func freeBackupPath(dbPath, infix string, now time.Time) string {
	// The names hold the wall clock, and ListBackups and retention order them
	// as text (time.Parse reads them as UTC). Compare in that frame too: in
	// instants, the hour that the end of daylight saving time repeats sorts
	// in another order than its names.
	at := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
	prefix := filepath.Base(dbPath) + infix
	entries, _ := os.ReadDir(filepath.Dir(dbPath))
	for _, entry := range entries {
		ts, ok := strings.CutPrefix(entry.Name(), prefix)
		if !ok {
			continue
		}
		if t, err := time.Parse(TimestampFormat, ts); err == nil && !t.Before(at) {
			at = t.Add(time.Second)
		}
	}
	// The scan does not see a name in another spelling (a file system that
	// ignores case), so take the next second while the name is taken.
	for {
		path := dbPath + infix + at.Format(TimestampFormat)
		if _, err := os.Lstat(path); err != nil {
			return path
		}
		at = at.Add(time.Second)
	}
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
// MaxAutoBackups, and the stale partial files of dbPath. The files in keep are
// never deleted, and a kept backup does not count toward the limit.
func enforceRetention(dbPath string, keep ...string) error {
	removeStalePartials(dbPath, time.Now(), keep...)

	backups, err := ListBackups(dbPath)
	if err != nil {
		return err
	}

	// Filter to auto-backups only
	var autoBackups []BackupInfo
	for _, b := range backups {
		if b.Type == BackupTypeAuto && !isKept(b.Path, keep) {
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
		for _, suffix := range db.WALSuffixes { // raw logs of a raw safety backup
			_ = os.Remove(b.Path + suffix)
		}
	}

	return nil
}

// isKept reports whether path is one of keep.
func isKept(path string, keep []string) bool {
	for _, k := range keep {
		if samePath(path, k) {
			return true
		}
	}
	return false
}

// samePath reports whether a and b name the same file, tolerating a relative
// spelling on one side. It is a cleaned-path comparison, not an inode check.
func samePath(a, b string) bool {
	if b == "" { // no file; filepath.Abs("") would give the working directory
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
// killed Restore left before it named its copies this way. It never deletes a
// file in keep: a clock that is behind (a network share) can give a copy in
// progress an old time. Best effort: a file it cannot delete stays, and is
// harmless, because no listing shows it.
func removeStalePartials(dbPath string, now time.Time, keep ...string) {
	dir, base := filepath.Dir(dbPath), filepath.Base(dbPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		ours := isPartialOf(base, name) || isLegacyRestoreTemp(name)
		if !ours || !entry.Type().IsRegular() || isKept(filepath.Join(dir, name), keep) {
			continue
		}
		if info, err := entry.Info(); err == nil && now.Sub(info.ModTime()) >= stalePartialAge {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

// isPartialOf reports whether name is a partial file of a copy to dbBase
// (Restore) or to one of dbBase's backups: exactly
// <final>.tmoney-partial-<digits>, or a log beside one (<partial>.wal and the
// other db.WALSuffixes, and Restore's moved <partial>.wal-old). Any other text
// after the infix is a file of the user, such as another database named
// "<dbBase>.tmoney-partial-2025.tdb".
func isPartialOf(dbBase, name string) bool {
	i := strings.LastIndex(name, partialInfix)
	if i < 0 {
		return false
	}
	tail := name[i+len(partialInfix):]
	for _, suffix := range db.WALSuffixes {
		if t, ok := strings.CutSuffix(tail, suffix); ok {
			tail = t
			break
		}
		if t, ok := strings.CutSuffix(tail, suffix+"-old"); ok {
			tail = t
			break
		}
	}
	if !isDigits(tail) {
		return false
	}
	final := name[:i]
	_, ok := parseBackupFilename(dbBase, final)
	return ok || final == dbBase
}

// isLegacyRestoreTemp reports whether name is a temp file of Restore from
// before the partial names: exactly tmoney-restore-<digits>.tmp.
func isLegacyRestoreTemp(name string) bool {
	digits, ok := strings.CutPrefix(name, "tmoney-restore-")
	digits, ok2 := strings.CutSuffix(digits, ".tmp")
	return ok && ok2 && isDigits(digits)
}

// isDigits reports whether s is one or more ASCII digits.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// copyDatabase copies the database at src to a new backup, named by next
// (freeBackupPath), and returns its path. The backup is never a part of a
// copy: copyDatabase writes a partial file beside it (see copyToTemp) and then
// gives it the name (publish). If the process dies first, only the partial
// file is left. If another process takes the name first, next names the
// backup again. With fold false, raw copies of the WAL files of src go beside
// the backup (see createRawSafetyBackup).
func copyDatabase(src string, next func() string, fold bool) (string, error) {
	dst := next()
	tempPath, err := copyToTemp(src, dst, src, fold)
	if err != nil {
		return "", err
	}
	for tries := 1; ; tries++ {
		err := publish(tempPath, dst)
		if err == nil {
			if err := publishRawLogs(tempPath, dst); err != nil {
				removeCopy(dst)
				removeCopy(tempPath)
				return "", fmt.Errorf("the raw copy of a write-ahead log of %s did not get its name: %w", src, err)
			}
			syncDir(filepath.Dir(dst))
			return dst, nil
		}
		if !errors.Is(err, os.ErrExist) || tries == 10 {
			removeCopy(tempPath)
			return "", err
		}
		dst = next()
	}
}

// publishRawLogs gives the raw copies of the logs beside the partial file at
// tempPath (copyToTemp without fold) their names beside the backup at dst.
// A backup without them would miss rows, so a failure is an error.
func publishRawLogs(tempPath, dst string) error {
	for _, suffix := range db.WALSuffixes {
		err := os.Rename(tempPath+suffix, dst+suffix)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// removeCopy deletes the partial file at tempPath and the logs beside it.
func removeCopy(tempPath string) {
	_ = os.Remove(tempPath)
	for _, suffix := range db.WALSuffixes {
		_ = os.Remove(tempPath + suffix)
	}
}

// publish gives the complete copy at tempPath the name dst, but never over an
// existing file: then it fails with an error that wraps os.ErrExist. A hard
// link does both in one step. A file system with no hard links (FAT, some
// network shares) gets a check and a rename, which a second process can still
// race.
func publish(tempPath, dst string) error {
	err := os.Link(tempPath, dst)
	if err == nil {
		_ = os.Remove(tempPath) // a leftover is only a stale partial
		return nil
	}
	if errors.Is(err, os.ErrExist) {
		return err
	}
	if _, statErr := os.Lstat(dst); statErr == nil {
		return &os.LinkError{Op: "publish", Old: tempPath, New: dst, Err: os.ErrExist}
	}
	return os.Rename(tempPath, dst)
}

// copyToTemp copies the database at src to a new partial file beside dst,
// flushed to disk and closed, with the permissions of the file at modeOf, and
// returns its path. When src has WAL files (see db.WALSuffixes), they hold
// committed rows that are not in src yet: copyToTemp copies them beside the
// partial file and, with fold, checkpoints them into it (db.CheckpointFile),
// so the copy is one complete file and src does not change. Without fold the
// raw copies stay beside the partial file. On error it removes what it wrote.
func copyToTemp(src, dst, modeOf string, fold bool) (_ string, err error) {
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
			removeCopy(tempPath)
		}
	}()
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

	if db.HasWAL(src) {
		for _, suffix := range db.WALSuffixes {
			err := copyPlain(src+suffix, tempPath+suffix)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if fold {
			if err := db.CheckpointFile(tempPath); err != nil {
				return "", fmt.Errorf("the write-ahead log of %s cannot be applied: %w", src, err)
			}
		}
	}

	// CreateTemp makes the file 0600. Give it the permissions of modeOf, as
	// the copy had before it went through a temp file; after the fold, which
	// must write the file. Best effort: a network share can refuse a chmod,
	// and 0600 still opens.
	if modeInfo, err := os.Stat(modeOf); err == nil {
		_ = os.Chmod(tempPath, modeInfo.Mode().Perm())
	}
	return tempPath, nil
}

// copyPlain copies src to a new file dst.
func copyPlain(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync() // a raw copy is the only copy once Restore deletes the log
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return err
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
