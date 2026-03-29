package category

import "fmt"

// IsSystemError is returned when trying to modify or delete a system category.
type IsSystemError struct {
	ID   string
	Name string
}

func (e *IsSystemError) Error() string {
	return fmt.Sprintf("cannot modify system category %q (%s)", e.Name, e.ID)
}

// MergeTypeMismatchError is returned when trying to merge categories of different types.
type MergeTypeMismatchError struct {
	SourceID   string
	SourceType string
	TargetID   string
	TargetType string
}

func (e *MergeTypeMismatchError) Error() string {
	return fmt.Sprintf("cannot merge categories: source %s is %s, target %s is %s",
		e.SourceID, e.SourceType, e.TargetID, e.TargetType)
}

// MergeSameError is returned when trying to merge a category into itself.
type MergeSameError struct {
	ID string
}

func (e *MergeSameError) Error() string {
	return fmt.Sprintf("cannot merge category %s into itself", e.ID)
}
