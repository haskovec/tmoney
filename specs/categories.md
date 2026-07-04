# Categories Specification

## Overview

Categories organize transactions for reporting and budgeting. They follow a two-level hierarchy (parent category and optional subcategory) and are typed as either income or expense.

## Category Properties

| Property | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | UUID | Yes | Unique identifier |
| `name` | string | Yes | Category name |
| `parent_id` | UUID | No | Parent category (null for top-level) |
| `type` | enum | Yes | `income` or `expense` |
| `created_at` | timestamp | Yes | When record was created |
| `updated_at` | timestamp | Yes | When record was last updated |

## Category Type

The category type determines how transactions are treated in reports:

| Type | Description | Example |
|------|-------------|---------|
| `income` | Money coming in | Salary, Interest, Refunds |
| `expense` | Money going out | Groceries, Rent, Utilities |

### Why Type Matters

A refund from a store is a positive amount (credit) but should reduce your expenses, not count as income. By marking the category as `expense` type, reports correctly show it as a reduction in spending.

Example:
- "Return at Amazon" → +$50.00 → Category: "Shopping" (expense type)
- In spending report: Shopping category shows net spending reduced by $50

## Hierarchy Rules

1. Maximum depth: 2 levels (parent → child)
2. Top-level categories have `parent_id = null`
3. Subcategories must reference a valid parent
4. A parent category cannot become a child
5. All categories in a tree share the same type (income or expense)

### Hierarchy Example

```
Income (type: income)
├── Salary
├── Interest
└── Other Income

Food (type: expense)
├── Groceries
├── Restaurants
└── Coffee

Transfer (type: expense, special)
├── [Auto-generated per account]
```

## Special Categories

### Transfer Category

The "Transfer" category is system-managed:
- Type: `expense` (arbitrary, transfers net to zero)
- Subcategories are auto-generated for each account
- Cannot be deleted or renamed by user
- Excluded from income/expense reports

### Labeling a Transfer with a Category

Separately from the system "Transfer" category above, a transfer may
optionally be **labeled** with any regular (non-system) category — income-
or expense-typed — to record *why* money moved (e.g. a credit-card payment
transfer labeled `Bills:Credit Card`). This label is:

- **Never required** — uncategorized transfers behave exactly as before.
- **One shared category per transfer**, mirrored across every leg that can
  store it; it never changes balance math, transfer linkage, or loan-shape
  detection.
- **Distinct from the system "Transfer" category** — that category remains
  system-managed and is not what labels a transfer. System categories
  (`Transfer`, `Value Adjustment`) are rejected as transfer labels.
- **Opt-in for reporting** — the spending report excludes labeled transfers
  by default and folds them in only when asked.

See [`specs/transfer-categories.md`](transfer-categories.md) for the full
design.

## Default Categories

The application ships with default categories. Users can modify these.

### Default Income Categories

| Parent | Subcategories |
|--------|---------------|
| Income | Salary, Bonus, Interest, Dividends, Refunds, Other Income |

### Default Expense Categories

| Parent | Subcategories |
|--------|---------------|
| Housing | Rent/Mortgage, Utilities, Insurance, Maintenance, Property Tax |
| Transportation | Gas, Auto Insurance, Maintenance, Parking, Public Transit |
| Food | Groceries, Restaurants, Coffee, Delivery |
| Healthcare | Doctor, Dentist, Pharmacy, Insurance |
| Entertainment | Streaming, Movies, Games, Hobbies |
| Shopping | Clothing, Electronics, Home Goods |
| Personal | Haircut, Gym, Subscriptions |
| Financial | Bank Fees, Interest Paid, Taxes |
| Education | Tuition, Books, Courses |
| Gifts | Given, Charity |
| Travel | Flights, Hotels, Vacation |
| Miscellaneous | Other |

## Validation Rules

1. `name` must be unique within the same parent (or among top-level if no parent)
2. `name` cannot be empty
3. `type` must match parent's type (for subcategories)
4. Cannot delete category with existing transactions (reassign first)
5. Cannot delete parent with existing children

## Operations

### Create Category

Required: name, type
Optional: parent_id

If parent_id provided, type is inherited from parent.

### Edit Category

- `name` can be changed
- `type` can only be changed if no transactions use this category
- `parent_id` can be changed (move category) if depth rules are satisfied

### Delete Category

1. Check for transactions using this category
2. If transactions exist, prompt user to reassign to another category
3. If parent, must delete or reassign children first
4. System categories (Transfer) cannot be deleted

### Merge Categories

Reassign all transactions from one category to another, then delete the source.

## Usage in Transactions

- A transaction can have one category (or be split across multiple)
- When entering a transaction, category can be:
  - Selected from list
  - Auto-populated from payee default
  - Created on-the-fly

## Reports Integration

Categories drive spending reports:
- Spending by Category (monthly/yearly)
- Income by Category
- Category trends over time

Reports can show:
- Parent categories only (summarized)
- All categories (detailed)
- Specific category drill-down
