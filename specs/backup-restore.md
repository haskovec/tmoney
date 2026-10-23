# Backup/Restore Specification (v1.5)

## Overview

Backup/Restore provides automatic and manual data protection for TMoney database files. Auto-backups run on app close with rolling retention, and users can restore from any available backup.

## Auto-backup

### Trigger

Auto-backup runs **on app close**:
- TUI: when the user quits the application (Ctrl+Q or File → Exit)
- CLI: when a CLI command that modifies data completes successfully

### Backup File Naming

Pattern: `<filename>.tdb.backup.<ISO-timestamp>`

Example:
```
personal.tdb.backup.2024-03-15T14-30-00
personal.tdb.backup.2024-03-14T09-15-23
personal.tdb.backup.2024-03-13T18-45-12
```

Timestamp format: `YYYY-MM-DDTHH-MM-SS` (using hyphens instead of colons for filesystem compatibility).
A new backup is always the newest of its kind, and it never replaces another
one: it takes the time now, or one second after the newest backup of its kind
if that is later. More than one backup per second therefore gives names ahead
of the clock. The names hold the local wall clock, and retention deletes by
name order, so a new backup is never the one it deletes, also in the hour that
the end of daylight saving time repeats.

A name that is ahead of the clock makes the new names of its kind follow it
until the clock passes it: a clock that was ahead and went back, a move to a
time zone to the west, or a backup copied in from another machine. Only the
times in the list are wrong. To make them right again, rename or delete the
backups of that kind whose names are ahead of the clock.

### Backup Location

Same directory as the database file.

Example: if the database is at `~/Documents/TMoney/personal.tdb`, backups are stored in `~/Documents/TMoney/`.

### Rolling Retention

- **Keep the last 5 backups**
- When creating the 6th backup, the oldest backup is deleted
- Only auto-backups participate in rolling retention — manual backups do not count against the limit

### Backup Process

1. Create a copy of the `.tdb` file with the timestamped name
2. Check how many auto-backups exist for this database
3. If more than 5, delete the oldest one(s)

### Performance

- Backup is a simple file copy — fast for typical database sizes
- Backup runs synchronously before the app exits
- For very large databases, a progress indicator is shown

## Manual Backup

Users can create a backup at any time via:

### TUI

File menu → Create Backup

Status bar shows: `"Backup created: personal.tdb.backup.2024-03-15T14-30-00"`

### CLI

```bash
tmoney --backup
```

Output:
```
Backup created: /Users/you/Documents/TMoney/personal.tdb.backup.2024-03-15T14-30-00
```

Manual backups:
- Use the same naming convention as auto-backups
- Are stored in the same location
- **Do not** count against the 5-backup rolling limit (they are never auto-deleted)
- Can be distinguished by a metadata marker (or simply: manual backups are never auto-rotated because only the rotation logic tracks which backups it created)

### Implementation Note

To distinguish manual from auto backups for rotation purposes, either:
1. Use a slightly different naming pattern (e.g., `personal.tdb.manual-backup.<timestamp>`)
2. Or track auto-backup filenames in a small metadata file

Option 1 is simpler and recommended.

## Restore

### TUI Workflow

1. File menu → Restore from Backup...
2. **Backup selection dialog** showing available backups:

```
┌──────────────────────────────────────────────────────┐
│  RESTORE FROM BACKUP                             [×]  │
├──────────────────────────────────────────────────────┤
│                                                        │
│  Available backups for personal.tdb:                  │
│                                                        │
│  Date                    Size                          │
│  ──────────────────────────────────────────────────   │
│▸ 2024-03-15 14:30:00    2.4 MB                        │
│  2024-03-14 09:15:23    2.3 MB                        │
│  2024-03-13 18:45:12    2.3 MB                        │
│  2024-03-12 11:00:05    2.2 MB                        │
│  2024-03-11 16:30:45    2.1 MB                        │
│                                                        │
├──────────────────────────────────────────────────────┤
│           [Cancel]                 [Restore]           │
└──────────────────────────────────────────────────────┘
```

3. User selects a backup and clicks Restore
4. **Confirmation dialog**:
   ```
   Restore from backup dated 2024-03-15 14:30:00?
   Current data will be overwritten.
   A backup of the current state will be created first.

   [Cancel]  [Restore]
   ```
5. On confirm:
   a. Create an auto-backup of the current database state (safety net)
   b. Replace the current database file with the selected backup
   c. Reload the database connection
   d. Status bar: `"Restored from backup: 2024-03-15 14:30:00"`

### CLI Workflow

#### List Backups

```bash
tmoney --list-backups
```

Output:
```
BACKUPS: personal.tdb
=====================
Date                    Size      Type
----                    ----      ----
2024-03-15 14:30:00    2.4 MB    Auto
2024-03-14 09:15:23    2.3 MB    Auto
2024-03-13 18:45:12    2.3 MB    Auto
2024-03-12 11:00:05    2.2 MB    Auto
2024-03-11 16:30:45    2.1 MB    Auto
2024-03-10 20:00:00    2.0 MB    Manual

6 backups found
```

#### Restore

```bash
tmoney --restore personal.tdb.backup.2024-03-15T14-30-00
```

Output:
```
Creating backup of current state...
Backup created: personal.tdb.backup.2024-03-15T16-45-00

Restoring from: personal.tdb.backup.2024-03-15T14-30-00
Restore complete.
```

Safety: the current state is always backed up before restoring.

## Error Handling

| Error | Handling |
|-------|----------|
| Backup file not found | `"Error: Backup file not found: <path>"` |
| Backup file corrupted | `"Error: failed to restore: backup file <path> is not a valid TMoney database: <reason>"` |
| Backup of a newer TMoney | `"Error: failed to restore: cannot restore backup file <path>: file was created with a newer version of TMoney (file: v<n>, app: v<m>)"` |
| Backup file cannot be read | `"Error: failed to restore: cannot read backup file <path>: open <path>: permission denied"` |
| Backup file open in another program | `"Error: failed to restore: backup file <path> is open in another program; close it and try again: <reason>"` |
| Disk full during backup | `"Error: Not enough disk space to create backup"` |
| Permission denied | `"Error: Cannot write to backup location"` |
| Restore fails mid-copy | Original file is preserved (copy to temp first, then rename) |

### Safe Restore Process

To prevent data loss if restore fails:

1. Verify the backup is a valid TMoney database (open it for reading, attach
   it read-only, check the metadata, refuse a newer schema version). A backup
   that this user cannot read, or that another program has open, is refused
   with that reason: it is not called invalid.
2. Copy the backup to a temporary file (`<db>.tmoney-partial-<n>`) in the same
   directory, with the permissions of the current database. If the backup has
   write-ahead log files beside it, apply them to the copy.
3. Verify the temp file the same way: it is what goes in place
4. Apply the write-ahead log files of the current database (`<db>.wal`,
   `<db>.wal.checkpoint`, `<db>.wal.recovery`) to it, in place, so that the
   file alone holds every committed row
5. Create the safety backup of the current state
6. Rename the temp file over the database name in one step. If the process
   dies before the rename, the database is as it was; after it, the database
   is the complete copy.

If DuckDB cannot apply the log files of the current database (a damaged log,
or the log of another file), it cannot open the database either. Then the
safety backup is the file alone with raw copies of the logs beside it
(`<safety backup>.wal`), and the logs move aside before the rename (and back
if the rename fails), because DuckDB would replay them onto the restored file.

Every backup is written the same way: a temp file beside the final name, with
the write-ahead log files of the source applied to it, flushed to disk, then
renamed. A crash during a backup leaves only a `.tmoney-partial-<n>` file,
which no listing shows. The next auto-backup deletes such a file when it is
more than an hour old. The sweep finds these files by name only, so the name
has "tmoney" in it: a file of the user is not likely to have such a name.

## Validation Rules

1. Backup destination directory must be writable
2. Backup file must be a valid TMoney database before restore
3. Cannot restore while a reconciliation is in progress
4. Restore creates a safety backup of current state first
5. Manual backups are never auto-deleted

## Config Changes

No config changes needed — backup behavior is automatic and uses the database file's directory.
