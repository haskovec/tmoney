package dberrors

import "fmt"

// NotFoundError is returned when an entity is not found.
type NotFoundError struct {
	Entity string
	ID     string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Entity, e.ID)
}

// HasDependentsError is returned when trying to delete an entity that has dependents.
type HasDependentsError struct {
	Entity     string
	ID         string
	Dependents string
	Count      int
}

func (e *HasDependentsError) Error() string {
	return fmt.Sprintf("cannot delete %s %s: has %d %s", e.Entity, e.ID, e.Count, e.Dependents)
}

// DuplicateError is returned when an entity with a unique field value already exists.
type DuplicateError struct {
	Entity string
	Field  string
	Value  string
}

func (e *DuplicateError) Error() string {
	return fmt.Sprintf("%s with %s %q already exists", e.Entity, e.Field, e.Value)
}
