// Package undo provides an undo/redo framework using the command pattern.
// The undo history is session-based and not persisted to the database.
package undo

// Command represents an undoable operation. Each command knows how to
// execute itself and reverse its effect.
type Command interface {
	// Execute performs the operation.
	Execute() error
	// Undo reverses the operation.
	Undo() error
	// Description returns a human-readable description (e.g. "Create transaction").
	Description() string
}

// CompoundCommand groups multiple commands into a single undo step.
// If any sub-command fails during Execute, previously executed commands
// are rolled back in reverse order.
type CompoundCommand struct {
	commands    []Command
	description string
}

// NewCompoundCommand creates a compound command from the given sub-commands.
func NewCompoundCommand(description string, commands ...Command) *CompoundCommand {
	return &CompoundCommand{
		commands:    commands,
		description: description,
	}
}

// Execute runs all sub-commands in order. If one fails, previously
// executed commands are undone in reverse order and the original error
// is returned.
func (c *CompoundCommand) Execute() error {
	for i, cmd := range c.commands {
		if err := cmd.Execute(); err != nil {
			// Rollback previously executed commands in reverse
			for j := i - 1; j >= 0; j-- {
				_ = c.commands[j].Undo()
			}
			return err
		}
	}
	return nil
}

// Undo reverses all sub-commands in reverse order.
func (c *CompoundCommand) Undo() error {
	for i := len(c.commands) - 1; i >= 0; i-- {
		if err := c.commands[i].Undo(); err != nil {
			return err
		}
	}
	return nil
}

// Description returns the compound command's description.
func (c *CompoundCommand) Description() string {
	return c.description
}
