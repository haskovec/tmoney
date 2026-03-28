# Mouse Support Specification (v1.5)

## Overview

Mouse support adds click-to-select and scroll-wheel interaction to the TUI, making it more accessible to users who prefer using a mouse alongside keyboard navigation.

## Capabilities

### Click to Select

Mouse clicks select or focus elements in all views:

| Target | Click Action |
|--------|-------------|
| Account in sidebar | Select the account (same as arrow key + highlighting) |
| Transaction in register | Select the transaction row |
| Menu bar item | Open the menu dropdown |
| Menu dropdown item | Execute the menu action |
| Dialog field | Focus the field |
| Dialog button | Activate the button (Cancel, Save, etc.) |
| Checkbox | Toggle the checkbox |
| Radio button | Select the radio option |
| Tab/section in reports | Switch to that report section |
| Reconciliation checkbox | Toggle transaction checked state |
| Import review row | Select the row |

### Scroll Wheel

Mouse wheel scrolls lists and tables:

| Context | Scroll Action |
|---------|---------------|
| Account sidebar | Scroll account list up/down |
| Transaction register | Scroll transaction list up/down |
| Scheduled transactions | Scroll the list up/down |
| Report view | Scroll report content |
| Reconciliation view | Scroll transaction list |
| Import review | Scroll import list |
| Dialog with scrollable content | Scroll within the dialog |
| File browser | Scroll file list |

### Not Supported (Deferred)

- Drag to resize panes
- Right-click context menus
- Text selection
- Drag and drop

## Toggle

### Default State

Mouse support is **enabled by default**.

### Disable via CLI Flag

```bash
tmoney --no-mouse                    # Disable mouse for this session
tmoney --no-mouse ~/finances.tdb     # Disable with specific file
```

### Disable via Config

The config file supports a persistent mouse setting:

```json
{
  "mouse_enabled": true,
  ...
}
```

Setting `"mouse_enabled": false` disables mouse support by default. The CLI flag overrides the config setting.

### Priority

1. CLI flag `--no-mouse` → disabled (highest priority)
2. Config file `mouse_enabled` setting
3. Default: enabled

## Implementation

### Bubbletea Integration

Enable mouse support in the Bubbletea program initialization:

```go
if mouseEnabled {
    p := tea.NewProgram(model, tea.WithMouseCellMotion())
} else {
    p := tea.NewProgram(model)
}
```

Use `tea.WithMouseCellMotion()` for click and scroll events without full motion tracking (less noisy than `tea.WithMouseAllMotion()`).

### Mouse Event Handling

Bubbletea provides `tea.MouseMsg` with:
- `Type`: `tea.MouseLeft`, `tea.MouseRight`, `tea.MouseMiddle`, `tea.MouseWheelUp`, `tea.MouseWheelDown`, etc.
- `X`, `Y`: screen coordinates of the event

### Hit Testing

Each component needs to know its screen bounds to determine if a click targets it:

1. **Sidebar**: fixed width on the left
2. **Main content**: right of sidebar
3. **Menu bar**: top row
4. **Status bar**: bottom row
5. **Dialog**: centered overlay with known dimensions

Components translate screen coordinates to their local coordinate system and determine which item was clicked (e.g., which row index in a table).

### Scroll Handling

Map scroll events to existing navigation:
- `MouseWheelUp` → same as `↑` key (move selection up by 1, or scroll by 3 lines in content areas)
- `MouseWheelDown` → same as `↓` key (move selection down by 1, or scroll by 3 lines in content areas)

Scroll speed: 3 lines per scroll tick for content areas, 1 item per tick for selectable lists.

## Interaction Details

### Double-click

Double-click on a transaction row opens the edit dialog (same as pressing Enter). This is optional and can be deferred if implementation is complex.

### Click Outside Dialog

Clicking outside an open dialog does **not** close it. Dialogs must be explicitly closed with Esc or a button. This prevents accidental data loss.

### Focus Management

Click-to-select respects the same focus rules as keyboard navigation:
- Clicking in the sidebar focuses the sidebar
- Clicking in the main content focuses the main content
- Clicking a dialog field focuses that field within the dialog
- When a dialog is open, clicks outside the dialog are ignored

## Testing Notes

- Mouse support should be tested in multiple terminal emulators (iTerm2, Terminal.app, Alacritty, Windows Terminal)
- Verify scroll wheel works correctly in all scrollable contexts
- Verify click coordinates are accurate after terminal resize
- Test with mouse disabled to ensure no regressions in keyboard-only mode
