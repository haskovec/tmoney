# Undo/Redo Specification (v1.5)

## Overview

Undo/redo allows users to reverse and re-apply data operations in the TUI, providing a safety net for accidental changes. The system covers all data operations and uses a session-based stack.

## Scope

Undo/redo covers **all data operations**:

| Operation Type | Examples |
|----------------|----------|
| Transactions | Create, edit, delete, void |
| Accounts | Create, edit, delete, close, reopen |
| Categories | Create, edit, delete, merge |
| Payees | Create, edit, delete, merge |
| Scheduled transactions | Create, edit, delete, post, skip |
| Reconciliation | Complete reconciliation (un-reconcile reverts status) |
| Import | Undo reverts all imported/updated transactions as a batch |
| Transfers | Create, edit, delete, void (both sides as one operation) |

## Undo Stack

### Session-based

- Undo history is maintained **for the current TUI session only**
- History is **cleared on app restart** — it is not persisted to the database
- This keeps the implementation simple and avoids database bloat

### Stack Behavior

- Standard undo/redo semantics:
  - Each operation pushes onto the undo stack
  - Undo pops from undo stack and pushes onto redo stack
  - Redo pops from redo stack and pushes onto undo stack
  - **Performing a new operation clears the redo stack**
- Stack depth: **unlimited** within a session

### Compound Operations

Some user actions involve multiple database operations. These are grouped as a **single undo step**:

| User Action | Grouped Operations |
|-------------|-------------------|
| Void a transfer | Void transaction A + void transaction B |
| Delete a transfer | Delete transaction A + delete transaction B |
| Edit a transfer amount | Update transaction A + update transaction B |
| Post scheduled transaction | Create transaction + advance schedule |
| Import transactions | Create/update N transactions |
| Merge categories | Update N transactions + delete source category |
| Merge payees | Update N transactions + delete source payee |
| Complete reconciliation | Update N transaction statuses |

## Command Pattern

### Interface

```go
type UndoableCommand interface {
    // Execute performs the operation
    Execute() error
    // Undo reverses the operation
    Undo() error
    // Description returns a human-readable description
    Description() string
}
```

### Example Commands

```go
// CreateTransactionCommand
type CreateTransactionCommand struct {
    service     *TransactionService
    transaction models.Transaction
    createdID   models.ID  // populated after Execute
}

func (c *CreateTransactionCommand) Execute() error {
    id, err := c.service.Create(c.transaction)
    c.createdID = id
    return err
}

func (c *CreateTransactionCommand) Undo() error {
    return c.service.Delete(c.createdID)
}

func (c *CreateTransactionCommand) Description() string {
    return "Create transaction"
}
```

### Compound Command

```go
type CompoundCommand struct {
    commands    []UndoableCommand
    description string
}

func (c *CompoundCommand) Execute() error {
    for i, cmd := range c.commands {
        if err := cmd.Execute(); err != nil {
            // Undo previously executed commands in reverse
            for j := i - 1; j >= 0; j-- {
                c.commands[j].Undo()
            }
            return err
        }
    }
    return nil
}

func (c *CompoundCommand) Undo() error {
    // Undo in reverse order
    for i := len(c.commands) - 1; i >= 0; i-- {
        if err := c.commands[i].Undo(); err != nil {
            return err
        }
    }
    return nil
}
```

### Undo Manager

```go
type UndoManager struct {
    undoStack []UndoableCommand
    redoStack []UndoableCommand
}

func (m *UndoManager) Execute(cmd UndoableCommand) error {
    if err := cmd.Execute(); err != nil {
        return err
    }
    m.undoStack = append(m.undoStack, cmd)
    m.redoStack = nil // Clear redo stack
    return nil
}

func (m *UndoManager) Undo() (string, error) {
    if len(m.undoStack) == 0 {
        return "", ErrNothingToUndo
    }
    cmd := m.undoStack[len(m.undoStack)-1]
    m.undoStack = m.undoStack[:len(m.undoStack)-1]
    if err := cmd.Undo(); err != nil {
        return "", err
    }
    m.redoStack = append(m.redoStack, cmd)
    return cmd.Description(), nil
}

func (m *UndoManager) Redo() (string, error) {
    if len(m.redoStack) == 0 {
        return "", ErrNothingToRedo
    }
    cmd := m.redoStack[len(m.redoStack)-1]
    m.redoStack = m.redoStack[:len(m.redoStack)-1]
    if err := cmd.Execute(); err != nil {
        return "", err
    }
    m.undoStack = append(m.undoStack, cmd)
    return cmd.Description(), nil
}
```

## TUI Keybindings

| Platform | Undo | Redo |
|----------|------|------|
| macOS | `Cmd+Z` | `Cmd+Y` |
| Windows/Linux | `Ctrl+Z` | `Ctrl+Y` |

### Platform Detection

Detect the platform at startup:
- `runtime.GOOS == "darwin"` → macOS keybindings
- Otherwise → Windows/Linux keybindings

Note: Bubbletea receives `Cmd+Z` as a distinct key event on macOS terminals that support it. Some terminals may send `Ctrl+Z` regardless — handle both on macOS.

### Status Bar Feedback

| Action | Status Bar Message |
|--------|-------------------|
| Successful undo | `"Undo: Create transaction"` (shows command description) |
| Successful redo | `"Redo: Create transaction"` |
| Nothing to undo | `"Nothing to undo"` |
| Nothing to redo | `"Nothing to redo"` |

Messages fade after 3 seconds or on next action.

## CLI

- **No CLI undo** — CLI operations are final and do not participate in the undo system
- CLI operations do not push onto the undo stack
- The undo stack only exists within the TUI process

## Data Integrity

### Before/After State

Each command stores enough state to reverse the operation:

| Operation | Stored State |
|-----------|-------------|
| Create | The created entity's ID (for deletion on undo) |
| Edit | The entity's state before editing (for restoration on undo) |
| Delete | The full entity data (for recreation on undo) |
| Void | The original amount, memo, and status (for restoration on undo) |
| Merge | All affected entity IDs and their original values |

### Error Handling

- If an undo operation fails, show an error message but don't corrupt the stack
- If a redo operation fails, show an error message
- Failed undo/redo does not modify the stacks

### Concurrent Modifications

The undo system assumes single-user, single-session access (which is the TMoney model). No conflict resolution is needed.

## Menu Integration

### Edit Menu (New)

Add an Edit menu to the TUI menu bar:

| Menu Item | Shortcut | Action |
|-----------|----------|--------|
| Undo | Cmd+Z / Ctrl+Z | Undo last operation |
| Redo | Cmd+Y / Ctrl+Y | Redo last undone operation |

The Edit menu shows the specific action that would be undone/redone:
- "Undo Create transaction"
- "Redo Delete account"
- "Undo (nothing)" when stack is empty (grayed out)
