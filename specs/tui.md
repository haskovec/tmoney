# TUI Interface Specification

## Overview

The TUI (Terminal User Interface) provides an interactive, keyboard-driven interface for managing finances. It uses the Bubbletea framework and follows modern TUI aesthetics similar to lazygit.

## Design Principles

1. **Modern aesthetic**: Clean, minimal design like lazygit, not retro
2. **Keyboard-first**: All actions accessible via keyboard
3. **Discoverable**: Key hints visible, help accessible
4. **Responsive**: Adapts to terminal size
5. **Fast**: Instant feedback, no lag

## Layout Structure

```
┌─────────────────────────────────────────────────────────────────────┐
│ Menu Bar                                                            │
├──────────────────┬──────────────────────────────────────────────────┤
│                  │                                                  │
│  Account         │  Main Content Area                               │
│  Sidebar         │  (Register, Dashboard, Reports, etc.)            │
│                  │                                                  │
│                  │                                                  │
│                  │                                                  │
├──────────────────┴──────────────────────────────────────────────────┤
│ Status Bar                                                          │
└─────────────────────────────────────────────────────────────────────┘
```

## Views

### Dashboard View (Home)

The default view showing financial overview.

```
┌─────────────────────────────────────────────────────────────────────┐
│ File  Accounts  Transactions  Securities  Reports  Help              │
├──────────────────┬──────────────────────────────────────────────────┤
│ ▼ Bank Accounts  │  DASHBOARD                          Jan 15, 2024 │
│   ▸ Checking     │  ─────────────────────────────────────────────── │
│   ▸ Savings      │                                                  │
│                  │  Net Worth:  $61,678.90                          │
│ ▼ Credit Cards   │                                                  │
│   ▸ Visa         │  ┌─────────────────┐  ┌─────────────────┐        │
│                  │  │ ASSETS          │  │ LIABILITIES     │        │
│ ▼ Investments    │  │ Checking  5,234 │  │ Visa      1,234 │        │
│   ▸ Brokerage    │  │ Savings  12,000 │  │                 │        │
│                  │  │ Invest.  45,678 │  │                 │        │
│                  │  │ ──────────────  │  │ ──────────────  │        │
│                  │  │ Total    62,913 │  │ Total     1,234 │        │
│                  │  └─────────────────┘  └─────────────────┘        │
│                  │                                                  │
│                  │  SCHEDULED (2 due)                               │
│                  │  • Rent - $1,500 due today                       │
│                  │  • Electric - ~$120 due in 3 days                │
│──────────────────┴──────────────────────────────────────────────────│
│ ↑↓ navigate  Enter select  n new transaction  ? help               │
└─────────────────────────────────────────────────────────────────────┘
```

### Account Register View

Transaction list for a specific account.

```
┌─────────────────────────────────────────────────────────────────────┐
│ File  Accounts  Transactions  Securities  Reports  Help              │
├──────────────────┬──────────────────────────────────────────────────┤
│ ▼ Bank Accounts  │  CHECKING ACCOUNT                   Bal: $5,234  │
│   ▸ Checking  ◀  │  ─────────────────────────────────────────────── │
│   ▸ Savings      │                                                  │
│                  │  Date       Payee          Category     Amount   │
│ ▼ Credit Cards   │  ──────────────────────────────────────────────  │
│   ▸ Visa         │  01/15 ✓   Kroger         Groceries    -125.43  │
│                  │  01/14     Employer       Salary      2,500.00  │
│ ▼ Investments    │▸ 01/12     Amazon         Shopping      -45.99  │
│   ▸ Brokerage    │  01/10 ✓   Shell          Gas           -42.50  │
│                  │  01/08 ✓   Transfer       Savings      -500.00  │
│                  │  01/05     Coffee Shop    Coffee         -5.75  │
│                  │  01/03 ✓   Landlord       Rent       -1,500.00  │
│                  │  01/01 ✓   Opening Bal    --          1,000.00  │
│                  │                                                  │
│                  │                                                  │
│                  │                                                  │
│──────────────────┴──────────────────────────────────────────────────│
│ ↑↓ navigate  Enter edit  n new  d delete  / search  ? help         │
└─────────────────────────────────────────────────────────────────────┘
```

Key:
- `✓` = Cleared transaction
- `▸` = Selected row
- `◀` = Currently selected account in sidebar

### Transaction Entry/Edit Dialog

Modal dialog for entering or editing transactions.

```
┌──────────────────────────────────────────────────────────┐
│  NEW TRANSACTION                                    [×]  │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Date:      [2024-01-15    ]                            │
│                                                          │
│  Payee:     [Kroger________]  ▼                         │
│                                                          │
│  Category:  [Food > Groceries]  ▼                       │
│                                                          │
│  Amount:    [$_____________]                            │
│                                                          │
│  Memo:      [Weekly groceries___________________]       │
│                                                          │
│  Status:    ( ) Pending  (•) Cleared                    │
│                                                          │
│  [ ] Split transaction                                   │
│                                                          │
├──────────────────────────────────────────────────────────┤
│           [Cancel]                    [Save]             │
└──────────────────────────────────────────────────────────┘
```

The Date field is sticky within a session: the first open of the
dialog seeds today's date; each subsequent open seeds the date of the
last *saved* transaction (Cancel does not update the seed). The seed
resets to today on app restart — there is no cross-launch persistence.

The Category field is a typeahead combo box (see [Category Combo
Box](#category-combo-box) below): typing filters the list, and the
last row `[+ Add new category…]` opens a small sub-dialog that
creates the category, auto-selects it, and advances focus to Amount.

### Split Transaction Dialog

When "Split transaction" is checked:

```
┌──────────────────────────────────────────────────────────┐
│  SPLIT TRANSACTION                                  [×]  │
├──────────────────────────────────────────────────────────┤
│  Total: $150.00                     Remaining: $0.00     │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Category              Amount        Memo                │
│  ─────────────────────────────────────────────────────   │
│  Food > Groceries      $120.00       Food items          │
│  Household > Cleaning   $30.00       Cleaning supplies   │
│  [+ Add split]                                           │
│                                                          │
├──────────────────────────────────────────────────────────┤
│           [Cancel]                    [Save]             │
└──────────────────────────────────────────────────────────┘
```

### Scheduled Transactions View

```
┌─────────────────────────────────────────────────────────────────────┐
│ File  Accounts  Transactions  Securities  Reports  Help              │
├──────────────────┬──────────────────────────────────────────────────┤
│ ▼ Bank Accounts  │  SCHEDULED TRANSACTIONS                          │
│   ▸ Checking     │  ─────────────────────────────────────────────── │
│   ▸ Savings      │                                                  │
│                  │  DUE NOW                                         │
│ ▼ Credit Cards   │  ──────────────────────────────────────────────  │
│   ▸ Visa         │▸ 01/15  Landlord      Rent        -$1,500  [Post]│
│                  │                                                  │
│ ▼ Investments    │  UPCOMING                                        │
│   ▸ Brokerage    │  ──────────────────────────────────────────────  │
│                  │  01/20  Electric Co   Utilities     ~$120  5 days│
│ ▼ Scheduled ◀    │  01/31  Auto Finance  Car Loan      -$450  16 day│
│                  │  02/01  Landlord      Rent        -$1,500  17 day│
│                  │                                                  │
│                  │                                                  │
│                  │                                                  │
│──────────────────┴──────────────────────────────────────────────────│
│ Enter post  s skip  e edit  n new  d delete  ? help                 │
└─────────────────────────────────────────────────────────────────────┘
```

### Reports View

```
┌─────────────────────────────────────────────────────────────────────┐
│ File  Accounts  Transactions  Securities  Reports  Help              │
├──────────────────┬──────────────────────────────────────────────────┤
│                  │  SPENDING BY CATEGORY              January 2024  │
│ ▸ Net Worth      │  ─────────────────────────────────────────────── │
│ ▸ Spending    ◀  │                                                  │
│   ▸ This Month   │  Housing         ████████████████░░░░  $1,500.00 │
│   ▸ This Year    │  Food            █████░░░░░░░░░░░░░░░    $523.45 │
│                  │  Transportation  ████░░░░░░░░░░░░░░░░    $345.67 │
│                  │  Shopping        ███░░░░░░░░░░░░░░░░░    $245.99 │
│                  │  Utilities       ██░░░░░░░░░░░░░░░░░░    $156.78 │
│                  │  Entertainment   █░░░░░░░░░░░░░░░░░░░     $89.50 │
│                  │  Other           █░░░░░░░░░░░░░░░░░░░     $45.00 │
│                  │                                       ──────────  │
│                  │  Total                                $2,906.39  │
│                  │                                                  │
│                  │  ◀ Dec 2023         Jan 2024         Feb 2024 ▶  │
│──────────────────┴──────────────────────────────────────────────────│
│ ←→ change period  Enter drill down  ? help                          │
└─────────────────────────────────────────────────────────────────────┘
```

## Menu Bar

Each menu label has its shortcut letter underlined to indicate the `Alt+key` shortcut (e.g., the "F" in "File" is underlined for `Alt+F`). Pressing `Alt+key` opens the corresponding menu; pressing it again toggles the menu closed.

### File Menu
- New File
- Open File
- Open Recent →
- Close File
- Exit

### Accounts Menu
- New Account
- Edit Account
- Close Account
- Delete Account

### Transactions Menu
- New Transaction
- New Transfer
- Edit Transaction
- Delete Transaction
- Search...

### Securities Menu
- Securities Master
- Prices

### Reports Menu
- Dashboard
- Net Worth
- Spending by Category

### Help Menu
- Keyboard Shortcuts
- About

## Keyboard Navigation

### Global Keys

| Key | Action |
|-----|--------|
| `?` | Show help/keyboard shortcuts |
| `Esc` | Close dialog / Cancel / Go back |
| `Ctrl+Q` | Quit application |
| `Tab` | Next field / Next pane |
| `Shift+Tab` | Previous field / Previous pane |
| `/` | Search |
| `Alt+F` | Open File menu |
| `Alt+A` | Open Accounts menu |
| `Alt+T` | Open Transactions menu |
| `Alt+S` | Open Securities menu |
| `Alt+R` | Open Reports menu |
| `Alt+H` | Open Help menu |
| `F10` | Activate menu bar |
| `:` | Command palette (v1.5) |

### Navigation Keys

| Key | Action |
|-----|--------|
| `↑` / `k` | Move up (optional vim keys) |
| `↓` / `j` | Move down |
| `←` / `h` | Collapse / Previous |
| `→` / `l` | Expand / Next |
| `Home` | Go to first item |
| `End` | Go to last item |
| `PgUp` | Page up |
| `PgDn` | Page down |

### Action Keys

| Key | Action |
|-----|--------|
| `Enter` | Select / Edit / Confirm |
| `n` | New item (transaction, account, etc.) |
| `e` | Edit selected item |
| `d` | Delete selected item |
| `Space` | Toggle selection |

### Account List Keys

| Key | Action |
|-----|--------|
| `Enter` | Open account register |
| `n` | New account |
| `e` | Edit account |

### Register Keys

| Key | Action |
|-----|--------|
| `n` | New transaction |
| `Enter` | Edit transaction |
| `d` | Delete transaction |
| `c` | Toggle cleared status |
| `t` | New transfer |
| `r` | Reconcile account |
| `/` | Search transactions |

### Scheduled Transaction Keys

| Key | Action |
|-----|--------|
| `Enter` | Post scheduled transaction |
| `s` | Skip occurrence |
| `e` | Edit scheduled transaction |
| `n` | New scheduled transaction |
| `d` | Delete scheduled transaction |

## Dialogs

All dialogs are modal and keyboard-navigable.

### Dialog Navigation

| Key | Action |
|-----|--------|
| `Tab` | Next field |
| `Shift+Tab` | Previous field |
| `Enter` | Submit / Confirm |
| `Esc` | Cancel / Close |
| `↑` / `↓` | Navigate options |

### Field Validation

- Required fields are marked with a red `*` next to the label
- On submit, all fields are validated simultaneously and inline errors appear below each invalid field
- Editing a field clears its error message
- Cross-field errors (e.g., transfer from/to same account) appear as a dialog-level message above the buttons
- Async/service errors after dialog close still use the full-screen error display

### Date Fields

Date fields use a fixed-width masked-input widget rather than free
text. Two formats ship: `MM/DD/YYYY` (most dialogs — Date, Start Date,
End Date, etc.) and `YYYY-MM-DD` (the Prices add/edit dialog).

| Key | Action |
|-----|--------|
| `0`–`9` | Overwrite the digit at the cursor and advance |
| `←` / `→` | Move cursor between digit positions (separators are skipped) |
| `Home` / `End` | Jump to first / last digit |
| `Backspace` | Replace the digit at the cursor with `0` and step back |

The cursor only ever lands on digit positions; the slashes (or
dashes) render as static literals. Typing is overwrite-style — there
is no insertion or shifting of digits — so the value is always a
canonical 10-character mask shape. Non-digit input is ignored. Per-
keystroke validation is intentionally minimal: the field rejects
non-digits but does not reject impossible combinations like month
`13` mid-edit; the existing on-Tab/Enter validation flow runs the
strict parse.

Optional date fields (e.g., the Scheduled Transaction End Date)
permit the all-blank canonical mask (`  /  /    `) as a meaningful
"no value." On those fields, Backspace clears the digit *before* the
cursor with a space instead of overwriting with `0`, so the user can
return to the canonical blank.

### Category Combo Box

The Category field on the New Transaction dialog is a typeahead combo
box: typing filters the option list inline; arrow keys navigate the
filtered subset; Enter or Tab commits the highlighted match.

| Key | Action |
|-----|--------|
| Letter / digit | Append to filter query |
| `Backspace` | Remove last character of filter query |
| `↑` / `↓` | Move highlight within the filtered list |
| `Enter` / `Tab` | Commit the highlighted match and advance focus |
| `Esc` | Clear a non-empty filter query in place (does not close the dialog) |

Filter ranking: case-insensitive substring match. Prefix matches on
the leaf segment (the part after the last ` > ` or `:`) rank ahead of
plain-substring matches; alphabetical within each rank group.

The last row of the filtered list is always
`[+ Add new category…]`. Activating it opens a small create-category
sub-dialog with three fields:

- **Name** — pre-filled from the typed query (or, if the query
  contains a `:`, split into `Parent:Child` with Parent pre-filled
  from the left side).
- **Parent** — itself a combo box that accepts an existing parent
  category or a typed-but-unmatched name (the latter creates a new
  top-level parent on save).
- **Income/Expense** — radio.

On confirm the new category (and parent, if needed) is persisted
immediately; the transaction dialog reopens with all other field
values preserved, the new category auto-selected, and focus advanced
to Amount. Cancel returns to the transaction dialog with the previous
selection intact.

## Status Bar

The status bar shows:
- Current view/context
- Key hints for current context
- Notifications/alerts (scheduled transactions due, etc.)

## Color Scheme

Modern, minimal color palette:

| Element | Color |
|---------|-------|
| Background | Terminal default (transparent) |
| Text | Terminal default |
| Selected row | Inverted or highlighted |
| Positive amounts | Green |
| Negative amounts | Red |
| Pending items | Dim/gray |
| Headers | Bold |
| Borders | Subtle/dim |
| Alerts | Yellow/orange |

## Responsive Design

The layout adapts to terminal size:

- **Small** (< 80 cols): Sidebar collapses, single-pane view
- **Medium** (80-120 cols): Standard two-pane layout
- **Large** (> 120 cols): Additional detail panels

## Accessibility

- High contrast mode support
- Screen reader friendly (when possible)
- Keyboard-only navigation
- No reliance on color alone for meaning

## v1.5 Features (Not in v1)

- Mouse support
- Command palette (`:` key)
- Customizable keybindings
- Theme support
- Vim-style keybindings toggle
