# Implementation Plan: CLI Cobra Migration

This document defines the order in which the remaining ~50 legacy `--flag` verbs in `internal/cli/legacy.go` are migrated to Cobra noun-verb subcommands per `specs/cli-router.md`. Each item is one PR following a red-green (test-first) pattern. Mark items as complete with `[x]` as they are finished.

Specs:
- `specs/cli-router.md` — CLI router design (the scaffold + `theme` + `version` batch is already implemented; this plan covers the remaining migration batches it left out of scope).
- `specs/cli.md` — current legacy-flag-shaped CLI reference; will be rewritten in Phase 13 once the migration is complete.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered to:

1. Pilot with `db` (Phase 1) — four verbs, two Cobra shapes (positional path vs. no-args), zero coupling to other nouns. Establishes the per-verb pattern at low risk.
2. Move through nouns in roughly increasing size and dependency depth: `account` → `transaction` → `transfer` → `scheduled` → `reconcile` → `security` → `price`.
3. `investment` (Phase 9) is the largest single noun group (12 verbs); it lands after the per-verb pattern is rote.
4. Top-level residuals (`import`, `export`) and the only 1→2 verb split (`report`) come last among the migration phases (Phases 10–11).
5. Phase 12 deletes all legacy plumbing in one focused phase.
6. Phase 13 rewrites `specs/cli.md` and `README.md` once, against the final shape.

## Per-item Shape

Every migration item follows the same four-step pattern:

1. **RED** — write tests in `internal/cli/<noun>_<verb>_test.go` invoking `executeWith([]string{"<noun>", "<verb>", …}, stdout, stderr)`. Tests cover the same scenarios as the existing legacy tests (`TestRun_<LegacyVerb>*`) for that handler. Required-flag-missing cases assert on Cobra's error format (`required flag(s) "x" not set`); semantic errors (invalid value, duplicate name, etc.) preserve the legacy handler's error message verbatim.
2. **GREEN** — create `internal/cli/<noun>_<verb>.go` containing:
    - a per-verb options struct (`<verb>Options`),
    - `new<Noun><Verb>Cmd()` returning a `*cobra.Command` using `MarkFlagRequired` for required flags,
    - the handler renamed from `run<LegacyVerb>` → `run<Noun><Verb>` with signature changed from `*cliOptions` → `*<verb>Options`. Body unchanged except for the field-access rewrite.
   The first verb in each noun group also creates `internal/cli/<noun>.go` (parent command), registers it on the root in `root.go`, adds `"<noun>"` to `cobraSubcommands`, and adds a `--help` smoke test asserting every verb in the group is listed.
3. **CLEANUP** — delete the legacy verb's branch from `parseArgs` (`internal/cli/args.go`) and its dispatch line from `RunLegacy` (`internal/cli/legacy.go`); delete the now-unused fields from `cliOptions`; delete the migrated `TestRun_<LegacyVerb>*` tests from `commands_test.go`. Build and full test suite green.
4. **DOCS** — update the relevant section of `README.md`'s `## CLI Reference` to show the new noun-verb shape replacing the legacy flag form. Skip if no README change applies (e.g., prep PRs).

Prep items (the first item of each phase) skip RED. They are pure `git mv`-style splits of the noun's handlers from `commands.go` into per-verb files. Function names and signatures are preserved verbatim. Build and full test suite green.

The final cleanup phase (Phase 12) deletes whatever residual plumbing is left after every verb has been migrated.

---

## Phase 1: db

- [x] **CM-001 — Phase prep: split `db` handlers into per-verb files** (completed 2026-05-06)
  - GREEN: create `internal/cli/db_create.go`, `db_backup.go`, `db_restore.go`, `db_list_backups.go`. Each contains exactly one of the legacy handlers (`runCreateDB`, `runBackup`, `runRestore`, `runListBackups`) cut verbatim from `commands.go`. No tests added.

- [x] **CM-002 — `tmoney db create <path>`** (completed 2026-05-06)
  - RED: tests in `internal/cli/db_create_test.go` cover the scenarios currently in `TestRun_CreateDB`, `TestRun_CreateDBWithEqualsFormat`, `TestRun_CreateDBAlreadyExists`, `TestRun_CreateDBAddsExtension`, `TestRun_CreateThenListAccounts`. The "missing path" case asserts on Cobra's `accepts 1 arg(s)` error. Add `TestDBCmd_Help` smoke test asserting `executeWith([]string{"db", "--help"}, …)` lists all four verbs.
  - GREEN: create `internal/cli/db.go` (parent `db` command). Create `db_create.go` with `dbCreateOptions{ path string }`, `newDBCreateCmd()` (`Use: "create <path>"`, `Args: cobra.ExactArgs(1)`), and `runDBCreate`. Register `db` on the root in `root.go`; add `"db"` to `cobraSubcommands`.
  - CLEANUP: delete `--create` parsing in `parseArgs`; delete the `if opts.createDB != ""` branch in `legacy.go`; delete `createDB` field from `cliOptions`; delete migrated tests from `commands_test.go`.
  - DOCS: README "Database" section under CLI Reference — replace `tmoney --create <path>` with `tmoney db create <path>`.

- [x] **CM-003 — `tmoney db backup`** (completed 2026-05-06)
  - RED: tests in `internal/cli/db_backup_test.go` covering the scenarios in `TestRun_Backup*`.
  - GREEN: `db_backup.go` with `dbBackupOptions{ file string }`, `newDBBackupCmd()` (`Args: cobra.NoArgs`, `MarkFlagRequired("file")` on the persistent `--file` if applicable per existing semantics), and `runDBBackup`.
  - CLEANUP: delete `--backup` plumbing as in CM-002.
  - DOCS: README — replace `tmoney --backup` with `tmoney db backup`.

- [x] **CM-004 — `tmoney db restore <path>`** (completed 2026-05-06)
  - RED: tests in `internal/cli/db_restore_test.go` covering `TestRun_Restore*`.
  - GREEN: `db_restore.go` with `dbRestoreOptions{ file, backupPath string }`, `newDBRestoreCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--restore` plumbing.
  - DOCS: README.

- [x] **CM-005 — `tmoney db list-backups`** (completed 2026-05-06)
  - RED: tests in `internal/cli/db_list_backups_test.go` covering `TestRun_ListBackups*`.
  - GREEN: `db_list_backups.go` with `dbListBackupsOptions{ file string }`, `newDBListBackupsCmd()` (`Args: cobra.NoArgs`).
  - CLEANUP: delete `--list-backups` plumbing.
  - DOCS: README.

## Phase 2: account

- [x] **CM-006 — Phase prep: split `account` handlers into per-verb files** (completed 2026-05-06)
  - GREEN: create `account_add.go`, `account_list.go`, `account_show.go`, `account_balance.go`. Each contains exactly one of `runAddAccount`, `runListAccounts`, `runAccountDetails`, `runBalance`, cut verbatim.

- [x] **CM-007 — `tmoney account add`** (completed 2026-05-06)
  - RED: tests in `internal/cli/account_add_test.go` cover the scenarios in `TestRun_AddAccount*` (missing required flags, invalid type, duplicate name, credit-limit on non-credit-card, etc.). Required-flag-missing cases assert on Cobra's error format.
  - GREEN: create `account.go` (parent). `account_add.go` with `accountAddOptions{ file, name, accountType, currency, openingBal, openingDate, institution, accountNumber, notes, creditLimit, interestRate string }`, `newAccountAddCmd()` (flags via `StringVar`, `MarkFlagRequired("name")`, `MarkFlagRequired("type")`), and `runAccountAdd`. Add `"account"` to `cobraSubcommands`. Add `TestAccountCmd_Help` smoke test.
  - CLEANUP: delete `--add-account` plumbing and the `addAccount bool` field from `cliOptions` (the shared `acct*` fields are still referenced by security verbs and stay until those migrate).
  - DOCS: README "Accounts" section — replace `tmoney --add-account …` with `tmoney account add …`.

- [x] **CM-008 — `tmoney account list`** (completed 2026-05-06)
  - RED: tests covering `TestRun_ListAccounts*` including `--include-closed`.
  - GREEN: `account_list.go` with `accountListOptions{ file string; includeClosed bool }`, `newAccountListCmd()` (`Args: cobra.NoArgs`, local `--include-closed` flag).
  - CLEANUP: delete `--list-accounts` plumbing. Keep `--include-closed` legacy parsing (still used by `--report net-worth`); retire when reports migrate.
  - DOCS: README.

- [x] **CM-009 — `tmoney account show <name>`** (completed 2026-05-06)
  - RED: tests covering `TestRun_Account*` (account-details).
  - GREEN: `account_show.go` with `accountShowOptions{ file, name string }`, `newAccountShowCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--account` plumbing (the `accountName` field on `cliOptions` is also used by other verbs as a filter — only delete the standalone-show usage; field stays until those verbs migrate).
  - DOCS: README.

- [x] **CM-010 — `tmoney account balance`** (completed 2026-05-07)
  - RED: tests covering `TestRun_Balance*`.
  - GREEN: `account_balance.go` with `accountBalanceOptions{ file string }`, `newAccountBalanceCmd()` (`Args: cobra.NoArgs`).
  - CLEANUP: delete `--balance` plumbing.
  - DOCS: README.

## Phase 3: transaction

- [x] **CM-011 — Phase prep: split `transaction` handlers into per-verb files** (completed 2026-05-07)
  - GREEN: create `transaction_add.go`, `transaction_list.go`, `transaction_void.go`, `transaction_search.go` containing `runAddTransaction`, `runTransactions`, `runVoidTransaction`, `runSearch` respectively, cut verbatim.

- [x] **CM-012 — `tmoney transaction add`** (completed 2026-05-07)
  - RED: tests covering `TestRun_AddTransaction*` (missing required, invalid amount, invalid date, payee auto-create, category resolution, etc.).
  - GREEN: create `transaction.go` (parent). `transaction_add.go` with `transactionAddOptions` (account, amount, payee, category, date, memo, check, status), `newTransactionAddCmd()` with `MarkFlagRequired("account")`, `MarkFlagRequired("amount")`. Register on root; add `"transaction"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--add-transaction` and `tx*` fields from `cliOptions`. (`tx*` fields are still referenced by `--transfer`, `--void`, `--add-scheduled`, `--post-scheduled`, `--search`, etc., so they remain until those verbs migrate.)
  - DOCS: README "Transactions" section.

- [x] **CM-013 — `tmoney transaction list`** (completed 2026-05-07)
  - RED: tests covering `TestRun_Transactions*` (account filter, limit, date range, status filter, invalid dates).
  - GREEN: `transaction_list.go` with `transactionListOptions{ file, account, fromDate, toDate, status string; limit int }`, `newTransactionListCmd()` (`MarkFlagRequired("account")`).
  - CLEANUP: delete `--transactions`, `--limit`, `--from`/`--to`/`--status` parsing branches that are exclusive to this verb.
  - DOCS: README.

- [x] **CM-014 — `tmoney transaction void <id>`** (completed 2026-05-07)
  - RED: tests covering `TestRun_VoidTransaction*`.
  - GREEN: `transaction_void.go` with `transactionVoidOptions{ file, txnID string }`, `newTransactionVoidCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--void` plumbing.
  - DOCS: README.

- [x] **CM-015 — `tmoney transaction search <term>`** (completed 2026-05-07)
  - RED: tests covering `TestRun_Search*` (account filter, category filter, min/max amount, date range).
  - GREEN: `transaction_search.go` with `transactionSearchOptions{ file, term, account, category, fromDate, toDate, minAmount, maxAmount string }`, `newTransactionSearchCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--search`, `--min`, `--max` plumbing; reduce `--from`/`--to`/`--category`/`--account` parsing branches.
  - DOCS: README.

## Phase 4: transfer

- [x] **CM-016 — Phase prep: split `transfer` handlers** (completed 2026-05-07)
  - GREEN: create `transfer_add.go`, `transfer_link.go` containing `runTransfer`, `runLinkTransfers` cut verbatim.

- [x] **CM-017 — `tmoney transfer add`** (completed 2026-05-07)
  - RED: tests covering `TestRun_Transfer*`.
  - GREEN: create `transfer.go` (parent). `transfer_add.go` with `transferAddOptions{ file, fromAccount, toAccount, amount, date, memo string }`, `newTransferAddCmd()` (`MarkFlagRequired("from")`, `MarkFlagRequired("to")`, `MarkFlagRequired("amount")`). Register on root; add `"transfer"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--transfer`, `--from`, `--to` (transfer-only usage) plumbing.
  - DOCS: README.

- [x] **CM-018 — `tmoney transfer link`** (completed 2026-05-07)
  - RED: tests covering `TestRun_LinkTransfers*` (dry-run preview, --confirm, --max-days override, ambiguous pairs).
  - GREEN: `transfer_link.go` with `transferLinkOptions{ file string; confirm bool; maxDays int }`, `newTransferLinkCmd()`.
  - CLEANUP: delete `--link-transfers`, `--max-days`, `--confirm` (link-only usage) plumbing.
  - DOCS: README "Link Transfers" section.

## Phase 5: scheduled

- [x] **CM-019 — Phase prep: split `scheduled` handlers** (completed 2026-05-07)
  - GREEN: create `scheduled_add.go`, `scheduled_list.go`, `scheduled_post.go`, `scheduled_skip.go` containing `runAddScheduled`, `runScheduled`, `runPostScheduled`, `runSkipScheduled` cut verbatim.

- [x] **CM-020 — `tmoney scheduled add`** (completed 2026-05-07)
  - RED: tests covering `TestRun_AddScheduled*`.
  - GREEN: create `scheduled.go` (parent). `scheduled_add.go` with `scheduledAddOptions` (account, amount, payee, category, frequency, day, occurrences, endDate, autoPost, leadDays, etc.), `newScheduledAddCmd()` with the appropriate `MarkFlagRequired` calls. Register on root; add `"scheduled"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--add-scheduled` and `st*` fields from `cliOptions`.
  - DOCS: README "Scheduled Transactions" section.

- [x] **CM-021 — `tmoney scheduled list`** (completed 2026-05-07)
  - RED: tests covering `TestRun_Scheduled*` (--due, --account filter).
  - GREEN: `scheduled_list.go` with `scheduledListOptions{ file, account string; due bool }`, `newScheduledListCmd()`.
  - CLEANUP: delete `--scheduled`, `--due` plumbing.
  - DOCS: README.

- [x] **CM-022 — `tmoney scheduled post <id>`** (completed 2026-05-07)
  - RED: tests covering `TestRun_PostScheduled*` (with --amount and --date overrides).
  - GREEN: `scheduled_post.go` with `scheduledPostOptions{ file, id, amount, date string }`, `newScheduledPostCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--post-scheduled` plumbing.
  - DOCS: README.

- [x] **CM-023 — `tmoney scheduled skip <id>`** (completed 2026-05-08)
  - RED: tests covering `TestRun_SkipScheduled*`.
  - GREEN: `scheduled_skip.go` with `scheduledSkipOptions{ file, id string }`, `newScheduledSkipCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--skip-scheduled` plumbing.
  - DOCS: README.

## Phase 6: reconcile

- [x] **CM-024 — Phase prep: split `reconcile` handlers** (completed 2026-05-08)
  - GREEN: create `reconcile_start.go`, `reconcile_mark.go`, `reconcile_finish.go`, `reconcile_status.go` containing `runStartReconcile`, `runMarkReconciled`, `runFinishReconcile`, `runReconcileStatus` cut verbatim.

- [x] **CM-025 — `tmoney reconcile start`** (completed 2026-05-08)
  - RED: tests covering `TestRun_StartReconcile*` (statement-date, statement-balance).
  - GREEN: create `reconcile.go` (parent). `reconcile_start.go` with `reconcileStartOptions{ file, account, statementDate, statementBalance string }`, `newReconcileStartCmd()` with required flags. Register on root; add `"reconcile"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--start-reconcile`, `--statement-date`, `--statement-balance` plumbing.
  - DOCS: README "Reconciliation" section.

- [x] **CM-026 — `tmoney reconcile mark <id>...`** (completed 2026-05-08)
  - RED: tests covering `TestRun_MarkReconciled*`. Variadic positional.
  - GREEN: `reconcile_mark.go` with `reconcileMarkOptions{ file string; ids []string }`, `newReconcileMarkCmd()` (`Args: cobra.MinimumNArgs(1)`).
  - CLEANUP: delete `--mark-reconciled` plumbing.
  - DOCS: README.

- [x] **CM-027 — `tmoney reconcile finish`** (completed 2026-05-08)
  - RED: tests covering `TestRun_FinishReconcile*` (force flag, non-zero diff).
  - GREEN: `reconcile_finish.go` with `reconcileFinishOptions{ file, account string; force bool }`, `newReconcileFinishCmd()`.
  - CLEANUP: delete `--finish-reconcile`, `--force` plumbing.
  - DOCS: README.

- [x] **CM-028 — `tmoney reconcile status`** (completed 2026-05-08)
  - RED: tests covering `TestRun_ReconcileStatus*`.
  - GREEN: `reconcile_status.go` with `reconcileStatusOptions{ file, account string }`, `newReconcileStatusCmd()` (`Args: cobra.NoArgs`, `MarkFlagRequired("account")`).
  - CLEANUP: delete `--reconcile-status` plumbing.
  - DOCS: README.

## Phase 7: security

- [x] **CM-029 — Phase prep: split `security` handlers** (completed 2026-05-08)
  - GREEN: create `security_add.go`, `security_list.go`, `security_show.go`, `security_edit.go`, `security_hide.go`, `security_unhide.go`, `security_delete.go` containing `runAddSecurity`, `runListSecurities`, `runSecurityDetail`, `runEditSecurity`, `runHideSecurity`, `runUnhideSecurity`, `runDeleteSecurity` cut verbatim.

- [ ] **CM-030 — `tmoney security add`**
  - RED: tests covering `TestRun_AddSecurity*`.
  - GREEN: create `security.go` (parent). `security_add.go` with `securityAddOptions` (ticker, asset class, name, currency, etc.), `newSecurityAddCmd()` with required flags. Register on root; add `"security"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--add-security` and `sec*` fields from `cliOptions`.
  - DOCS: README "Securities" section under CLI Reference (note: Securities currently live under TUI section in README; add CLI subsection if missing).

- [ ] **CM-031 — `tmoney security list`**
  - RED: tests covering `TestRun_ListSecurities*`.
  - GREEN: `security_list.go` with `securityListOptions{ file string; showHidden bool }`, `newSecurityListCmd()`.
  - CLEANUP: delete `--list-securities` plumbing.
  - DOCS: README.

- [ ] **CM-032 — `tmoney security show <ticker>`**
  - RED: tests covering `TestRun_SecurityDetail*`.
  - GREEN: `security_show.go` with `securityShowOptions{ file, ticker string }`, `newSecurityShowCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--security` (detail-show) plumbing.
  - DOCS: README.

- [ ] **CM-033 — `tmoney security edit <ticker>`**
  - RED: tests covering `TestRun_EditSecurity*`.
  - GREEN: `security_edit.go` with `securityEditOptions` (ticker positional, then optional fields to update), `newSecurityEditCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--edit-security` plumbing.
  - DOCS: README.

- [ ] **CM-034 — `tmoney security hide <ticker>`**
  - RED: tests covering `TestRun_HideSecurity*`.
  - GREEN: `security_hide.go` with `securityHideOptions{ file, ticker string }`, `newSecurityHideCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--hide-security` plumbing.
  - DOCS: README.

- [ ] **CM-035 — `tmoney security unhide <ticker>`**
  - RED: tests covering `TestRun_UnhideSecurity*`.
  - GREEN: `security_unhide.go` with `securityUnhideOptions{ file, ticker string }`, `newSecurityUnhideCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--unhide-security` plumbing.
  - DOCS: README.

- [ ] **CM-036 — `tmoney security delete <ticker>`**
  - RED: tests covering `TestRun_DeleteSecurity*`.
  - GREEN: `security_delete.go` with `securityDeleteOptions{ file, ticker string }`, `newSecurityDeleteCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--delete-security` plumbing.
  - DOCS: README.

## Phase 8: price

- [ ] **CM-037 — Phase prep: split `price` handlers**
  - GREEN: create `price_add.go`, `price_list.go`, `price_current.go`, `price_import.go`, `price_update.go` containing `runAddPrice`, `runListPrices`, `runCurrentPrice`, `runImportPrices`, `runUpdatePrices` cut verbatim. (`update_prices.go` already exists with helper code; keep that file as-is and add `price_update.go` for the new Cobra wiring; consolidate during the migration step CM-042.)

- [ ] **CM-038 — `tmoney price add`**
  - RED: tests covering `TestRun_AddPrice*`.
  - GREEN: create `price.go` (parent). `price_add.go` with `priceAddOptions` (ticker, date, value, source), `newPriceAddCmd()` with required flags. Register on root; add `"price"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--add-price` plumbing.
  - DOCS: README "Update Prices" section grows into a "Prices" CLI subsection.

- [ ] **CM-039 — `tmoney price list <ticker>`**
  - RED: tests covering `TestRun_ListPrices*`.
  - GREEN: `price_list.go` with `priceListOptions{ file, ticker string; limit int }`, `newPriceListCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--prices` plumbing.
  - DOCS: README.

- [ ] **CM-040 — `tmoney price current <ticker>`**
  - RED: tests covering `TestRun_CurrentPrice*`.
  - GREEN: `price_current.go` with `priceCurrentOptions{ file, ticker string }`, `newPriceCurrentCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--current-price` plumbing.
  - DOCS: README.

- [ ] **CM-041 — `tmoney price import <file>`**
  - RED: tests covering `TestRun_ImportPrices*`.
  - GREEN: `price_import.go` with `priceImportOptions{ file, csvPath string }`, `newPriceImportCmd()` (`Args: cobra.ExactArgs(1)`).
  - CLEANUP: delete `--import-prices` plumbing.
  - DOCS: README.

- [ ] **CM-042 — `tmoney price update`**
  - RED: tests covering `TestUpdatePrices*` from `update_prices_test.go` adapted to Cobra invocation. Variadic positional for ticker filtering.
  - GREEN: `price_update.go` with `priceUpdateOptions{ file, provider string; tickers []string }`, `newPriceUpdateCmd()` (`Args: cobra.ArbitraryArgs`). Move runtime helper logic from existing `update_prices.go` into `price_update.go`; delete `update_prices.go` once consolidated.
  - CLEANUP: delete `--update-prices` plumbing.
  - DOCS: README.

## Phase 9: investment

The largest phase: 12 verbs, all under `tmoney investment …`. Verbs cluster as trading (buy/sell/dividend/reinvest), cash flow (fee/deposit/withdraw/transfer), corporate actions (split/merge/spin-off), and reporting (portfolio).

- [ ] **CM-043 — Phase prep: split `investment` handlers**
  - GREEN: create twelve files (`investment_buy.go`, `investment_sell.go`, …) containing the corresponding `run*` handlers cut verbatim from `commands.go`.

- [ ] **CM-044 — `tmoney investment buy`**
  - RED: tests covering `TestRun_Buy*`.
  - GREEN: create `investment.go` (parent). `investment_buy.go` with `investmentBuyOptions` (account, ticker, shares, price, date, fees, etc.), `newInvestmentBuyCmd()` with required flags. Register on root; add `"investment"` to `cobraSubcommands`. Add help smoke test listing all twelve verbs.
  - CLEANUP: delete `--buy` plumbing.
  - DOCS: README "Investment" CLI subsection (add if missing).

- [ ] **CM-045 — `tmoney investment sell`** — RED/GREEN/CLEANUP/DOCS per the standard pattern; covers `TestRun_Sell*`.
- [ ] **CM-046 — `tmoney investment dividend`** — covers `TestRun_Dividend*`.
- [ ] **CM-047 — `tmoney investment reinvest`** — covers `TestRun_Reinvest*`.
- [ ] **CM-048 — `tmoney investment fee`** — covers `TestRun_InvestmentFee*`. Replaces `--investment-fee`.
- [ ] **CM-049 — `tmoney investment deposit`** — covers `TestRun_InvestDeposit*`. Replaces `--invest-deposit`.
- [ ] **CM-050 — `tmoney investment withdraw`** — covers `TestRun_InvestWithdraw*`. Replaces `--invest-withdraw`.
- [ ] **CM-051 — `tmoney investment transfer`** — covers `TestRun_TransferShares*`. Replaces `--transfer-shares`.
- [ ] **CM-052 — `tmoney investment split`** — covers `TestRun_Split*`. Replaces `--split`.
- [ ] **CM-053 — `tmoney investment merge`** — covers `TestRun_MergeSecurity*`. Replaces `--merge-security`.
- [ ] **CM-054 — `tmoney investment spin-off`** — covers `TestRun_SpinOff*`. Replaces `--spin-off`.
- [ ] **CM-055 — `tmoney investment portfolio`** — covers `TestRun_Portfolio*`. Replaces `--portfolio`. `Args: cobra.NoArgs`.

## Phase 10: import / export

These are top-level subcommands (no parent noun), since the spec calls for `tmoney import <file>` rather than nesting under a noun.

- [ ] **CM-056 — Phase prep: split `import`/`export` handlers**
  - GREEN: create `import.go`, `export.go` containing `runImport`, `runExport` cut verbatim.

- [ ] **CM-057 — `tmoney import <file>`**
  - RED: tests covering `TestRun_Import*` (CSV/QIF/OFX detection, `--account`, `--source-account`, `--confirm`, `--skip-duplicates`, `--update-duplicates`, `--format`).
  - GREEN: `import.go` with `importOptions{ file, importFile, account, sourceAccount, formatOverride string; confirm, skipDuplicates, updateDuplicates bool }`, `newImportCmd()` (`Args: cobra.ExactArgs(1)`, `MarkFlagRequired("account")`). Register on root; add `"import"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--import`, `--account` (import-only usage), `--source-account`, `--format`, `--confirm`, `--skip-duplicates`, `--update-duplicates` plumbing.
  - DOCS: README "Import Transactions" section.

- [ ] **CM-058 — `tmoney export <file>`**
  - RED: tests covering `TestRun_Export*`.
  - GREEN: `export.go` with `exportOptions{ file, exportFile, account, fromDate, toDate, formatOverride string }`, `newExportCmd()` (`Args: cobra.ExactArgs(1)`). Register on root; add `"export"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: delete `--export` plumbing.
  - DOCS: README "Export Transactions" section.

## Phase 11: report

`--report --report-type net-worth|spending` is the only 1→2 verb split in the migration.

- [ ] **CM-059 — Phase prep: split `report` handlers**
  - GREEN: create `report_net_worth.go`, `report_spending.go` containing `runNetWorthReport`, `runSpendingReport` cut verbatim. The dispatcher `runReport` is left in `commands.go` (it just routes by `--report-type` and gets deleted entirely in cleanup).

- [ ] **CM-060 — `tmoney report net-worth`**
  - RED: tests covering `TestRun_NetWorth*` (with `--as-of`, `--include-closed`).
  - GREEN: create `report.go` (parent). `report_net_worth.go` with `reportNetWorthOptions{ file, asOf string; includeClosed bool }`, `newReportNetWorthCmd()`. Register on root; add `"report"` to `cobraSubcommands`. Add help smoke test.
  - CLEANUP: in `parseArgs` and `legacy.go`, delete the `--report` + `--report-type net-worth` branches; delete `reportAsOf` field from `cliOptions`.
  - DOCS: README "Net worth" section under CLI Reference.

- [ ] **CM-061 — `tmoney report spending`**
  - RED: tests covering `TestRun_Spending*` (`--month`, `--year`, `--from`/`--to`).
  - GREEN: `report_spending.go` with `reportSpendingOptions{ file, month, fromDate, toDate string; year int }`, `newReportSpendingCmd()`.
  - CLEANUP: delete the residual `--report`/`--report-type` branches and `reportType`/`reportMonth`/`reportYear` fields from `cliOptions`. Delete the now-orphan `runReport` dispatcher in `commands.go`.
  - DOCS: README "Spending by Category" section.

## Phase 12: Final cleanup

After Phase 11, every legacy verb is migrated and `parseArgs`/`legacy.go` only carry residual scaffolding. This phase deletes that scaffolding.

- [ ] **CM-062 — Delete `RunLegacy` and the legacy dispatch path**
  - RED: not applicable (deletion).
  - GREEN: delete `internal/cli/legacy.go` (move `runTUI` into `internal/cli/tui.go` if it doesn't already live there). Delete the `legacyRunner` indirection and the `isLegacyInvocation` branch in `root.go` — the Cobra root command becomes the only entry point. Update `Execute()` accordingly. Confirm full test suite passes.

- [ ] **CM-063 — Delete `parseArgs` and `internal/cli/args.go`**
  - GREEN: delete `internal/cli/args.go`. Delete `internal/cli/args_test.go` (if it covers only `parseArgs`). Update any remaining references. Tests for individual verbs already cover Cobra-side parsing via `executeWith`; nothing should reference `parseArgs` after Phase 11.

- [ ] **CM-064 — Delete `cliOptions` struct**
  - GREEN: remove `cliOptions` from wherever it lives now (likely `args.go`, deleted in CM-063, but verify). Each per-verb file already has its own options struct.

- [ ] **CM-065 — Delete `commands.go` and `commands_test.go` if empty**
  - GREEN: confirm both files are empty (every handler has been moved into a per-verb file by the prep PRs; every test has been migrated by the verb PRs). Delete them. If any shared helpers remain in `commands.go` (e.g., a `loadDB` helper), move them to a `helpers.go` and delete `commands.go`. Test helpers similarly move to `testutil_test.go` if not already there.

- [ ] **CM-066 — Tidy root.go**
  - GREEN: with no legacy path remaining, delete `cobraKnownFlags`, `cobraSubcommands`, `isLegacyInvocation`, and `legacyRunner` from `root.go`. The root command's `RunE` keeps the no-args TUI launch path; everything else is just regular Cobra subcommand dispatch. Update tests in `root_test.go` to drop legacy-routing scenarios.

## Phase 13: Documentation

- [ ] **CM-067 — Rewrite `specs/cli.md`**
  - GREEN: replace `specs/cli.md`'s flag-based documentation with a noun-verb reference. Each noun group becomes a section; each verb a subsection with `Use`, required flags, optional flags, examples, and expected output (carry over the existing examples, just shifted to the new shape). Keep the "Database File Handling", "Date Formats", "Amount Formats", "Exit Codes", and "Configuration" sections. Cross-reference `specs/cli-router.md` once and note that the migration is complete.

- [ ] **CM-068 — Update `README.md` and retire `specs/cli-router.md`'s migration section**
  - GREEN: in `README.md`'s `## CLI` section, remove the "migration is opportunistic" caveat and replace with a one-paragraph statement that all verbs are now Cobra-native, plus a pointer to `specs/cli.md`. Reflow the `## CLI Reference` section so every example uses the noun-verb form (most should already be updated by the per-verb DOCS steps; this is a final pass for consistency). In `specs/cli-router.md`, add a status note at the top stating the migration is complete and move the **Migration Strategy** section to a `## History` heading at the bottom (preserved for posterity, not removed).

---

## Out of Scope

Explicitly deferred — not in this implementation plan:

- **Aliases for common verbs** (e.g., `tmoney portfolio` for `tmoney investment portfolio`). Add only if requested.
- **Shell completion installation UX.** Cobra's `tmoney completion bash|zsh|fish` is auto-provided; no installer is shipped.
- **Backward-compatibility shims for legacy flags.** The router spec explicitly chose no deprecation period; users see Cobra's default "unknown command/flag" error after migration.
- **Cobra command groups for top-level help.** The default flat list of subcommands is acceptable. If grouping by domain is wanted, it's a separate UX polish PR after Phase 13.
- **Optional follow-up: convert remaining handler-side validations to `MarkFlagRequired`.** Most "missing required flag" cases are already moved during migration; this would be a sweep over any cases that opted to keep handler-side checks for richer error messages. Worth a single follow-up PR if desired.
- **Restructuring `internal/cli/format.go`.** Output-formatting helpers stay shared across verbs; not split per-verb.
