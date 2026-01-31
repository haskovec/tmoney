# Payees Specification

## Overview

Payees represent entities you pay money to or receive money from. They enable quick transaction entry through auto-completion and default category assignment.

## Payee Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `name` | string | Yes | Display name (e.g., "Amazon") |
| `default_category_id` | UUID | No | Category to auto-fill |
| `notes` | string | No | User notes about this payee |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

## Payee Aliases

Aliases allow multiple variations to map to a single payee. This is essential for imported transactions where bank descriptions vary.

### Alias Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `payee_id` | UUID | Yes | Parent payee |
| `pattern` | string | Yes | Match pattern |
| `match_type` | enum | Yes | `exact`, `contains`, `starts_with`, `regex` |
| `created_at` | timestamp | Yes | When record was created |

### Alias Match Types

| Type | Description | Example Pattern | Matches |
|------|-------------|-----------------|---------|
| `exact` | Exact string match | "AMAZON.COM" | "AMAZON.COM" only |
| `contains` | Substring match | "AMZN" | "AMZN*1234", "AMZN MKTP" |
| `starts_with` | Prefix match | "KROGER" | "KROGER #123", "KROGER FUEL" |
| `regex` | Regular expression | `^AMZN.*MKTP` | "AMZN MKTP US", "AMZN*MKTP" |

### Alias Matching Order

When matching an imported transaction description:
1. Try exact matches first
2. Then starts_with matches (longest pattern first)
3. Then contains matches (longest pattern first)
4. Finally regex matches (in order defined)

## Auto-Creation

When a user enters a new payee name that doesn't exist:
1. Create new payee with that name
2. Set default_category_id to the category used in this transaction
3. Add the exact name as an alias (match_type: exact)

## Default Category Behavior

When a payee is selected during transaction entry:
1. If payee has default_category_id set, auto-populate the category field
2. User can override the auto-populated category
3. Optionally update payee's default to the new category (prompt or setting)

### Learning Default Category

The default category can be updated based on usage:
- Option 1: Always use the last category used with this payee
- Option 2: Use the most frequently used category
- Option 3: Never auto-update (user must manually set)

For v1: Use the category from the most recent transaction with this payee.

## Validation Rules

1. `name` must be unique
2. `name` cannot be empty
3. Alias `pattern` must be unique across all aliases
4. Alias `pattern` cannot be empty
5. Regex patterns must be valid

## Operations

### Create Payee

Required: name
Optional: default_category_id, notes

Automatically creates an exact-match alias for the name.

### Edit Payee

- `name` can be changed (updates the auto-created alias too)
- `default_category_id` can be changed
- `notes` can be changed

### Delete Payee

1. Check for transactions using this payee
2. If transactions exist, prompt:
   - Delete anyway (transactions keep payee name as text, lose link)
   - Reassign to another payee
   - Cancel
3. Delete all associated aliases

### Merge Payees

1. Select source and target payees
2. Move all aliases from source to target
3. Update all transactions from source to target
4. Delete source payee

## Alias Management

### Add Alias

When user notices an unmatched import:
1. Select the imported transaction
2. Choose "Assign to Payee"
3. Select existing payee or create new
4. The import description becomes a new alias

### Edit Alias

- Pattern can be changed
- Match type can be changed

### Delete Alias

- Cannot delete the primary alias (same as payee name) unless renaming payee
- Other aliases can be deleted freely

## Usage in Transaction Entry

### TUI Flow

1. User starts typing payee name
2. Autocomplete shows matching payees
3. User selects payee (or types new name)
4. If existing payee: category auto-fills
5. If new payee: created after transaction is saved

### CLI Flow

```
tmoney --add-transaction --account "Checking" --date 2024-01-15 \
       --amount -50.00 --payee "Kroger" --category "Food:Groceries"
```

If "Kroger" doesn't exist, it's created with "Food:Groceries" as default.

## Import Matching

During import (v1.5):
1. For each imported transaction description
2. Run through alias matching
3. If match found: assign payee and default category
4. If no match: leave payee blank for user review

## Suggested Payee Improvements (v1.5+)

- Payee address/contact information
- Payee website
- Payee icon/logo
- Payee spending statistics
