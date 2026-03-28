# Transaction Status Specification (v1.5)

## Overview

v1.5 expands transaction status from two states (pending, cleared) to four: Uncleared, Cleared, Reconciled, and Void. This is a foundational change required by reconciliation and improves transaction lifecycle management.

## Status Definitions

| Status | Code | Balance Impact | Editable | Description |
|--------|------|----------------|----------|-------------|
| `uncleared` | `U` | Yes | Yes | Default state for new transactions |
| `cleared` | `C` | Yes | Yes | Confirmed by user or bank statement |
| `reconciled` | `R` | Yes | **No** | Locked after reconciliation |
| `void` | `V` | **No** | No | Zeroed out and marked as void |

## Status Transitions

```
uncleared ──→ cleared ──→ reconciled
    │             │            │
    ▼             ▼            ▼
   void         void     un-reconcile → cleared
```

- Any non-reconciled transaction can be voided
- Reconciled transactions must be un-reconciled before editing or voiding
- Un-reconciling shows a warning about breaking reconciliation integrity

## Void Behavior

When a transaction is voided:

1. **Amount is set to 0** — the original amount is discarded
2. **Memo is replaced with `**VOID**`** — the original memo is discarded
3. **Status is set to `void`**
4. Transaction remains **visible in the register** but is **excluded from all balance calculations**
5. Void is irreversible through normal operations (but can be undone via undo/redo)

### Voiding Transfers

When voiding a transaction that is part of a transfer pair:
- **Both sides are automatically voided** — the counterpart transaction is also voided
- This is a single atomic operation (one undo step)

### Voiding Split Transactions

When voiding a split transaction:
- The parent transaction is voided (amount → 0, memo → `**VOID**`)
- All split items are removed or zeroed

## Reconciled Behavior

- Reconciled transactions **cannot be edited** — attempting to edit shows an error
- User must **un-reconcile** the transaction first
- Un-reconciling:
  - Sets status back to `cleared`
  - Shows a **warning dialog**: "This transaction was reconciled on <date>. Un-reconciling may break reconciliation integrity. Continue?"
  - Does not affect other reconciled transactions from the same session

## Display

### Register View

| Status | Indicator | Style |
|--------|-----------|-------|
| Uncleared | (empty) | Normal text |
| Cleared | `✓` | Normal text |
| Reconciled | `R` | Normal text, locked icon if editing attempted |
| Void | `V` | Dimmed/strikethrough text |

### CLI Output

Status column shows: `U`, `C`, `R`, or `V`

## Database Migration

### Schema Changes

1. Rename `pending` status value to `uncleared`
2. Add `void` as a valid status value
3. Ensure `reconciled` is a valid status value (was planned but not active in v1)

### Migration Strategy

- All existing transactions with status `pending` are migrated to `uncleared`
- No data loss — this is a rename only
- Existing `cleared` transactions remain unchanged

## Balance Calculation Changes

Current (v1):
```
balance = opening_balance + sum(all transactions)
cleared_balance = opening_balance + sum(cleared transactions)
```

Updated (v1.5):
```
balance = opening_balance + sum(transactions WHERE status != 'void')
cleared_balance = opening_balance + sum(transactions WHERE status IN ('cleared', 'reconciled'))
```

## Validation Rules

1. New transactions default to `uncleared` status
2. Status transitions must follow the allowed paths
3. Void transactions cannot be edited (except via undo)
4. Reconciled transactions cannot be edited without un-reconciling first
5. Voiding a transfer must void both sides atomically

## CLI Changes

- `--status` flag accepts: `uncleared`, `cleared`, `reconciled`, `void`
- `--void <txn-id>` — void a transaction (and its transfer counterpart if applicable)
- Transaction list output shows the new status codes

## TUI Changes

- Register `c` key cycles: uncleared → cleared (same as v1, just renamed)
- New `v` key to void selected transaction (with confirmation dialog)
- Reconciled transactions show lock indicator when selected
- Void transactions shown with dimmed styling
