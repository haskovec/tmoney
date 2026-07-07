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
│ ↑↓ navigate  ←→ collapse/expand  Enter select  ? help              │
└─────────────────────────────────────────────────────────────────────┘
```

An investment account on the dashboard shows a headline balance and total-return
row, and — when it holds securities — an expandable list of its top holdings
marked with a `▸`/`▾` affordance. Such accounts start **expanded** on load.
`Left`/`Right` (or `h`/`l`) collapse/expand the holdings of the account under the
sidebar cursor; a collapse is remembered for the session (a dashboard reload —
e.g. after posting a transaction — will not re-expand an account the user
collapsed). Collapsing is the way to shrink a tall dashboard so the ASSETS/
LIABILITIES totals and the SCHEDULED section stay on screen: the content pane is
clipped to the available height (it never overwrites the status bar), so content
that overflows the bottom is otherwise unreachable from this view.

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

After saving a new or edited transaction, the cursor moves to that
transaction's row, matched by ID rather than by position. This keeps the
selection on the saved row wherever it sorts — including a back-dated entry
that lands in the middle of the list, not at the top. The same applies to the
investment register (buy, sell, dividend, reinvest, cash operations, and share
transfers). Reloads that aren't saves — toggling cleared status, deleting —
leave the cursor where it was.

#### Investment register: filter by security

In an investment account's register, `/` opens a **security filter** for
drilling into a single holding (e.g. to audit a position or track down a
data-entry error). Typing narrows the list live to rows whose security matches
the query as a case-insensitive substring on **either the ticker or the full
name** — so tickerless holdings (a collective trust carried by name) are
reachable too. Cash rows (deposit, withdrawal, fee, interest, cash transfer)
carry no security and drop out of a non-empty query.

An active-filter line renders under the title showing the matched security's
`TICKER — Full Name` (degrading to name-only for a tickerless holding) and the
match count, or an `N securities` count while the query still matches more than
one. Pressing `Enter` when the query resolves to **exactly one** security
**locks** the filter to it (clearing the typed text); an ambiguous or empty
match keeps the user typing. `Esc` clears the filter (typed or locked) and
restores the full register. While a filter is active, the running cash-balance
column and the account-wide total-return header are hidden (both are
account-wide and can't be sliced per security), and while typing, the arrow /
page / Home / End keys still navigate the narrowed list. Pressing `n` while a
security is locked pre-selects that security in the security-bearing
new-transaction dialogs (Buy, Sell, Dividend, Reinvest, Fee via Liquidation,
Transfer Shares). The filter is transient — it clears when the register is left
(a different account, another view) and does not persist across launches.

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
last *saved* entry — shared across the transaction, investment
(Buy/Sell/Dividend/Reinvest/Cash/Transfer) and corporate-action
(Stock Split/Merger/Spin-Off) dialogs (Cancel does not update the
seed). The seed resets to today on app restart — there is no
cross-launch persistence.

The Category field is a typeahead combo box (see [Category Combo
Box](#category-combo-box) below): typing filters the list, and the
last row `[+ Add new category…]` opens a small sub-dialog that
creates the category, auto-selects it, and advances focus to Amount.

### Split Transaction Dialog

When "Split transaction" is checked:

```
┌──────────────────────────────────────────────────────────────┐
│  SPLIT TRANSACTION                                      [×]  │
├──────────────────────────────────────────────────────────────┤
│  Parent: +3,067.50                                           │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  Category / Target          Amount      Memo                 │
│  ───────────────────────────────────────────────────────     │
│  Income > Salary           +5,000.00                         │
│  Tax > Federal               -800.00                         │
│  Tax > Social Security       -310.00                         │
│  Tax > Medicare               -72.50                         │
│  Transfer → 401k             -500.00                         │
│  Insurance > Health          -150.00                         │
│  Transfer → HSA              -100.00                         │
│  [+ Add split]                                               │
│                                                              │
│  Imbalance: $0.00                                            │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│           [Cancel]                    [Save]                 │
└──────────────────────────────────────────────────────────────┘
```

Lines may be **categorized** (pick a category from the combo box) or **transfers** (pick the `Transfer →` sentinel option in the combo, then pick a target account). Line amounts may be mixed-sign — the parent amount is the signed sum of all lines. The "Imbalance" indicator updates live as the user types; **Save is disabled until imbalance is zero** (no auto-balancing plug). See [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md) for the full primitive.

The Category / Target picker also exposes a `[+ Add new category…]`
action row below the `Transfer →` sentinel — Down past `Transfer →`
parks on it and Enter opens the create-category sub-dialog (see
[Category Combo Box](#category-combo-box)) for the originating row.

### Scheduled Transaction Preview Dialog

Pressing Enter on a due scheduled transaction (Scheduled view or Dashboard's Due panel) opens this preview dialog rather than posting immediately. The dialog is pre-filled with the schedule's template values and lets the user adjust **this one occurrence** before saving. For a multi-line scheduled transaction (e.g., a paycheck):

```
┌──────────────────────────────────────────────────────────────┐
│  POST SCHEDULED TRANSACTION                             [×]  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  Date:      [01/23/2026]                                     │
│  Payee:     [Employer________________]  ▼                    │
│  Account:   [Checking ▼]                                     │
│  Memo:      [_______________________________________]        │
│                                                              │
│  Parent: +3,067.50                                           │
│  ───────────────────────────────────────────────────         │
│  Category / Target          Amount      Memo                 │
│  Income > Salary           +5,000.00                         │
│  Tax > Federal               -800.00                         │
│  Tax > Social Security       -310.01    ← edited             │
│  Tax > Medicare               -72.50                         │
│  Transfer → 401k             -500.00                         │
│  Insurance > Health          -149.99    ← edited             │
│  Transfer → HSA              -100.00                         │
│  [+ Add split]                                               │
│                                                              │
│  Imbalance: $0.00                                            │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│           [Cancel]                    [Save]                 │
└──────────────────────────────────────────────────────────────┘
```

Semantics (full detail in [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md)):

- Date edits are **one-off**: the schedule's `next_date` advances per the template, not the edit.
- Amount and line edits are **one-off**: edits do not modify the template.
- Save is disabled while a multi-line preview is imbalanced.
- To modify the template for all future occurrences, press `e` (Edit Series) on the scheduled item in the list instead of `Enter`.

For a single-line scheduled transaction, the preview renders the regular Transaction Entry/Edit Dialog shape pre-filled with template values. The Category combo on the single-line preview exposes a `[+ Add new category…]` action row to create a new category inline (see [Category Combo Box](#category-combo-box)).

### Paycheck Schedule Wizard

A guided form (TUI-only) creates a multi-line scheduled paycheck. Opened from **Transactions → New Paycheck Schedule…**. The wizard is pure UI sugar — the saved record is a standard multi-line scheduled transaction.

The wizard is organized into five sections that mirror US pay-stub structure (earnings → pre-tax deductions → statutory withholdings → post-tax deductions → net pay destinations):

```
┌──────────────────────────────────────────────────────────────┐
│  PAYCHECK SCHEDULE                                      [×]  │
├──────────────────────────────────────────────────────────────┤
│                                                              │
│  Employer (payee):  [_______________________]                │
│  Pay frequency:     [Biweekly ▼]    Next payday: [MM/DD/YYYY]│
│                                                              │
│  EARNINGS                                                    │
│    $[__________]   [Income:Salary ▼]                         │
│    [+ Add earnings line]                                     │
│                                                              │
│  PRE-TAX DEDUCTIONS                                          │
│    [+ Add pre-tax line]                                      │
│                                                              │
│  TAXES                                                       │
│    $[____]   [Tax:Federal ▼]                                 │
│    $[____]   [Tax:Social Security ▼]                         │
│    $[____]   [Tax:Medicare ▼]                                │
│    [+ Add tax line]                                          │
│                                                              │
│  POST-TAX DEDUCTIONS                                         │
│    [+ Add post-tax line]                                     │
│                                                              │
│  NET PAY DESTINATIONS                                        │
│    Primary deposit: [Checking ▼]   ($X,XXX.XX — remainder)   │
│    [+ Add transfer]                                          │
│                                                              │
├──────────────────────────────────────────────────────────────┤
│           [Cancel]                    [Save]                 │
└──────────────────────────────────────────────────────────────┘
```

Only universally-applicable rows are pre-populated: a single `Income:Salary` row in Earnings, and the three federal-statutory withholdings (`Tax:Federal`, `Tax:Social Security`, `Tax:Medicare`) in Taxes. Employer-specific items (HSA, 401(k), supplemental life, state income tax, health insurance) are added via `[+ Add line]`. Rows left at $0 are silently dropped on save.

Each row's Category picker exposes a `[+ Add new category…]` action
row at the bottom of the option list to create a new category inline
(see [Category Combo Box](#category-combo-box)). The create-category
sub-dialog opens with its Type radio defaulted per the originating
section: Earnings and Net Pay Destination rows default to Income;
Tax, Pre-tax, and Post-tax rows default to Expense.

The Earnings section supports multiple lines so real pay-stub itemization (base salary plus shift differential, housing allowance, **imputed income** for employer-paid benefits like LTD coverage) can live on the recurring template. Imputed income is entered as two independent lines — a positive earnings line and a same-category negative offset in Post-tax — because the offset is what keeps the parent transaction's net deposit equal to what actually hits the bank.

The "Primary deposit" line is the schedule's parent account; its amount is computed at save time as the signed sum of all other lines. Each saved split item is tagged with its `paycheck_section` (`earnings` / `pre_tax` / `tax` / `post_tax` / `net_pay_destination`) so the wizard can round-trip — the **Edit as paycheck →** affordance in the Edit Series dialog relaunches this wizard with values pre-filled, lines grouped back into their original sections. The affordance hides if any line is NULL-tagged (e.g., added via the generic multi-line dialog) until the schedule is re-saved through the wizard. See [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md) for the full spec, including the `paycheck_section` data-model addition.

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
- Open Recent
- Import Transactions…
- Create Backup
- Restore from Backup
- Close File
- Exit

### Edit Menu
- Undo
- Redo

### View Menu
- Show closed positions — a toggle (a `✓` marks it on)
- Theme entries — one row per available theme (the built-ins `default`,
  `light`, and `turbo-vision`, plus any user-installed theme from the theme
  directory), the active one marked `✓`. Selecting one applies it live and
  persists it. See [Theming](#theming).

### Accounts Menu
- New Account
- New Loan…
- Edit Account
- Close Account
- Reopen Account
- Delete Account
- Reconcile Account

### Transactions Menu
- New Transaction
- New Transfer
- Edit Transaction
- Delete Transaction
- Search…
- Link Transfers…
- New Paycheck Schedule…

### Securities Menu
- Security Master
- Prices
- Stock Split…
- Merger…
- Spin-Off…
- Corporate Action History…

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
| `/` | Search transactions (investment register: filter by security — see [Investment register: filter by security](#investment-register-filter-by-security)) |

### Scheduled Transaction Keys

| Key | Action |
|-----|--------|
| `Enter` | Open the post-time preview dialog (edit this one occurrence, then save) |
| `s` | Skip occurrence |
| `e` | Edit series — modify the template (affects all future occurrences) |
| `n` | New scheduled transaction |
| `t` | New scheduled transfer (mirrors the register's `t`) |
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
- A date before the account's opening date is rejected inline on the Date
  field (e.g. `Before Brokerage opened (01/01/2020)`) so the user can fix it
  without losing the entry. This mirrors the service-layer guard (see
  [`specs/transactions.md`](transactions.md) validation rule 7) and applies to
  the New/Edit Transaction, Transfer, and every investment entry dialog
  (Buy/Sell/Dividend/Reinvest/cash/Transfer Shares).
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

### Numeric Fields

The investment-register entry dialogs use a **numeric-only** text
field for their quantity and money inputs. These fields accept only
the digits `0`–`9` and a single decimal point; any other keystroke —
a second `.`, a sign, `$`, a comma, a space, or a letter — is
silently dropped at the input layer (the same approach the masked
Date field takes with non-digits), so the field value is always a
clean, parseable non-negative number. The field is an ordinary text
input in every other respect (cursor movement, Backspace, click-to-
position). On-submit validation (required, positive, price-or-total)
is unchanged and still runs as the backstop.

This applies to:

- **Buy** — Shares, Total, Price/Share, Commission
- **Sell** — Shares, Total, Price/Share, Commission, and the per-lot
  allocation fields
- **Dividend** — Amount
- **Reinvest Dividend** — Shares, Total, Price/Share
- **Cash operations** (Deposit / Withdrawal / Fee / Interest) — Amount
- **Transfer Shares** — Shares and the per-lot allocation fields

All of these are positive magnitudes, so no sign is ever accepted.
The regular transaction **Amount** field is a normal text field and
is unaffected — it still accepts a leading `-` for expenses. (Memo
fields on these dialogs are likewise plain text.)

### Category Combo Box

The Category field is a typeahead combo box on:

- The New Transaction and Edit Transaction dialogs.
- The New Scheduled Transaction and Edit Scheduled Transaction dialogs.
- The Scheduled Transaction Preview dialog (single-line schedules).
- The Transfer, Edit Transfer, and Scheduled Transfer dialogs.
- The single-line transfer post-time preview.

On those surfaces, typing filters the option list inline; arrow keys
navigate the filtered subset; Enter or Tab commits the highlighted
match. A transfer's category is always optional; investment↔investment
transfers cannot carry one (the Edit Transfer dialog omits the field,
and the Transfer dialog rejects a selected category on submit), since
there is nowhere to store it — see
[`specs/transfer-categories.md`](transfer-categories.md).

The Split Transaction dialog and the Paycheck Schedule Wizard use a
simpler index-navigated picker (no typeahead) — Up/Down cycles through
the full option list rather than a typed-filter subset — but they
expose the same `[+ Add new category…]` action at the bottom of the
option list, so the create-category sub-dialog opens identically from
every Category-input surface.

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

The last rows of the filtered list are always:

- `Transfer →` — present only when the combo box is used on a **split line** (not on the top-level Category field of a single-line transaction). Activating it swaps the field for an account picker (excluding the parent's account); the resulting line is stored as a transfer-line with `transfer_account_id` set and `category_id` NULL. See [`specs/multiline-splits-and-paycheck.md`](multiline-splits-and-paycheck.md).
- `[+ Add new category…]`

Activating `[+ Add new category…]` opens a small create-category
sub-dialog with three fields:

- **Name** — pre-filled from the typed query (or, if the query
  contains a `:`, split into `Parent:Child` with Parent pre-filled
  from the left side).
- **Parent** — itself a combo box that accepts an existing parent
  category or a typed-but-unmatched name (the latter creates a new
  top-level parent on save).
- **Income/Expense** — radio.

The Type radio defaults are context-aware:

- Earnings and Net Pay Destination rows of the Paycheck Wizard
  default to Income.
- Tax, Pre-tax, and Post-tax rows of the Paycheck Wizard default to
  Expense.
- The Transaction, Split, Scheduled, and Scheduled Preview dialogs
  infer the default from the typed amount: a parseable strictly-
  positive number defaults to Income; anything else (empty, zero,
  negative, or unparseable) defaults to Expense.

The user can override the default with a single keystroke before
confirming; the default is just a head-start, not a constraint.

On confirm the new category (and parent, if needed) is persisted
immediately; the originating dialog reopens with all other field
values preserved, the new category auto-selected, and focus advanced
to Amount. Cancel returns to the originating dialog with the previous
selection intact.

## Mouse Support

Mouse interaction is supported for common operations. The guiding rule is
**single click selects, double click opens**: a single click on a list or
table row moves the cursor there, and a double-click on the same row within
the double-click threshold drills in (opens the account register, a
ticker's price history, etc.). Buttons, menu items, dialog fields, and other
affordances activate on a single click.

| Action | Effect |
|--------|--------|
| Click menu label | Open / close that menu's dropdown |
| Click dropdown item | Execute the menu action |
| Click account in sidebar | Select the account |
| Double-click account in sidebar | Open the account register |
| Click group header in sidebar | Select the group heading |
| Click a list / table row | Select that row |
| Double-click a prices-list row | View that ticker's price history |
| Click sidebar / table area | Move focus between panes |
| Scroll wheel | Navigate the focused list or table |
| Click a dialog field | Focus it (a text field also positions the cursor at the click) |
| Click a dialog checkbox | Toggle it |
| Click a dialog list item | Select it |
| Click a combo-box dropdown row | Commit that row (see below) |
| Scroll wheel over a focused combo / list field | Move the highlight |
| Click a dialog button | Activate it (Save, Cancel, Lookup, …) |
| Click a dialog `[x]` | Close the dialog |

### Combo box mouse behavior

The [Category Combo Box](#category-combo-box) and the investment Security
pickers (Buy / Sell / Dividend / Reinvest / Fee via Liquidation / Transfer
Shares, and the Merger / Spin-Off security fields) render a dropdown panel
below the input line while the field is focused. Mouse interaction mirrors
the keyboard:

- **Click a match row** — commits that option (like `Enter` / `Tab`),
  clears the typed filter query, and advances focus to the next field.
- **Click the `[+ Add new category…]` action row** — opens the
  create-category sub-dialog for the originating field (like `Enter` on that
  row), leaving focus on the combo and preserving the typed query.
- **Scroll wheel** over a focused combo — moves the dropdown highlight up /
  down, the same as the arrow keys.
- **Click the input (header) line** — focuses the combo and opens its
  dropdown. A combo shows only its header line while unfocused, so a first
  click opens the list and a second click on a now-visible row commits it.

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

### Theming

The palette above describes the built-in **default** theme. TMoney ships
three built-in themes — `default`, `light`, and `turbo-vision` — and also
loads any theme (a TOML file) found in the user theme directory
(`$XDG_CONFIG_HOME/tmoney/themes/`). Switch themes live from **View →
Theme** (the active theme is marked `✓`); the choice is persisted to the
config file's `theme` key and restored on the next launch. A theme with an
unreadable or invalid file falls back to the default palette rather than
failing to open. On the CLI, `tmoney theme list` shows the available themes
and `tmoney theme generate-from-wal` writes a theme derived from the current
pywal cache into the user theme directory (where it then appears under View
→ Theme).

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

Mouse support, theming, and vim-style navigation keys (`h`/`j`/`k`/`l`)
have since shipped — see [Mouse Support](#mouse-support),
[Theming](#theming), and [Navigation Keys](#navigation-keys). Vim keys are
always on, so no enable/disable toggle is planned. Still deferred:

- Command palette (`:` key)
- Customizable (user-remappable) keybindings
