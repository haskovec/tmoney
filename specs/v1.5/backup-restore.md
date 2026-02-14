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
| Backup file corrupted | `"Error: Backup file is not a valid TMoney database"` |
| Disk full during backup | `"Error: Not enough disk space to create backup"` |
| Permission denied | `"Error: Cannot write to backup location"` |
| Restore fails mid-copy | Original file is preserved (copy to temp first, then rename) |

### Safe Restore Process

To prevent data loss if restore fails:

1. Copy the backup to a temporary file in the same directory
2. Verify the temp file is a valid TMoney database (open and check schema)
3. Rename the current database to a `.restoring` suffix
4. Rename the temp file to the database name
5. Delete the `.restoring` file
6. If any step fails, roll back to the `.restoring` file

## Validation Rules

1. Backup destination directory must be writable
2. Backup file must be a valid TMoney database before restore
3. Cannot restore while a reconciliation is in progress
4. Restore creates a safety backup of current state first
5. Manual backups are never auto-deleted

## Config Changes

No config changes needed — backup behavior is automatic and uses the database file's directory.
