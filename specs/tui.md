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
│ File  Accounts  Transactions  Reports  Help                         │
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
│ File  Accounts  Transactions  Reports  Help                         │
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
│ File  Accounts  Transactions  Reports  Help                         │
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
│ File  Accounts  Transactions  Reports  Help                         │
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

### Dropdown/Autocomplete

| Key | Action |
|-----|--------|
| `↓` | Open dropdown / Next suggestion |
| `↑` | Previous suggestion |
| `Enter` | Select suggestion |
| `Esc` | Close dropdown |
| Typing | Filter suggestions |

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
