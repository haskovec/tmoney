package db

import "fmt"

// FileNotFoundError indicates the database file doesn't exist.
type FileNotFoundError struct {
	Path string
}

func (e *FileNotFoundError) Error() string {
	return fmt.Sprintf("file not found: %s", e.Path)
}

// FileExistsError indicates the file already exists when trying to create.
type FileExistsError struct {
	Path string
}

func (e *FileExistsError) Error() string {
	return fmt.Sprintf("file already exists: %s", e.Path)
}

// InvalidFileError indicates the file is not a valid TMoney database.
type InvalidFileError struct {
	Path   string
	Reason string
}

func (e *InvalidFileError) Error() string {
	return fmt.Sprintf("not a valid TMoney file: %s (%s)", e.Path, e.Reason)
}

// CorruptedFileError indicates the database file appears to be corrupted.
type CorruptedFileError struct {
	Path string
	Err  error
}

func (e *CorruptedFileError) Error() string {
	return fmt.Sprintf("file appears to be corrupted: %s (%v)", e.Path, e.Err)
}

func (e *CorruptedFileError) Unwrap() error {
	return e.Err
}

// DatabaseError is a general database operation error.
type DatabaseError struct {
	Op  string
	Err error
}

func (e *DatabaseError) Error() string {
	return fmt.Sprintf("database error during %s: %v", e.Op, e.Err)
}

func (e *DatabaseError) Unwrap() error {
	return e.Err
}

// MetadataNotFoundError indicates a metadata key doesn't exist.
type MetadataNotFoundError struct {
	Key string
}

func (e *MetadataNotFoundError) Error() string {
	return fmt.Sprintf("metadata key not found: %s", e.Key)
}

// VersionMismatchError indicates the file was created with a newer version.
type VersionMismatchError struct {
	FileVersion int
	AppVersion  int
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("file was created with a newer version of TMoney (file: v%d, app: v%d)", e.FileVersion, e.AppVersion)
}
