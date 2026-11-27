# TMoney Code Review & Remediation Checklist

**Date:** 2026-11-27
**Baseline:** `fe541c1` (`go vet ./...` and `golangci-lint run ./...` pass with 0 issues).
**Goal:** Work through verified correctness bugs in strict criticality order, followed by architectural and idiomatic Go improvements. Check off each item (`- [x]`) and link the PR/commit as it is resolved.

---

## Critical — Data Loss, Ledger Corruption, or Wrong Financial Math

- [x] **CRIT-01: Non-transactional `DELETE + INSERT` in `payee.Repository.Update` & `category.Repository.Update`** *(fixed on `fix/category-payee-inplace-update`: migration `038`, true in-place `UPDATE`s, `runInTx` in services, `payee` default-category verification, and `category.Repository.Delete` guards for `payees` & `scheduled_split_items`)*
  - **Files:** `internal/payee/payee_repository.go:149-201`, `internal/category/category_repository.go:252-332`, `internal/category/category_service.go:649-654`
  - **Issue:** Both `Update` methods execute `DELETE` then `INSERT` without a transaction. A failed `INSERT` (e.g., invalid `default_category_id` FK in `SetDefaultCategory`) commits the `DELETE` and permanently destroys the row. Worse, because `categories(id)` still has 6 inbound FKs and `payees(id)` has 3 inbound FKs, editing/renaming *any* referenced category or payee (or merging a category with referenced children) fails with a DuckDB FK violation.
  - **DuckDB Note:** This workaround originally existed because older DuckDB versions turned `UPDATE` on an indexed column into an internal `DELETE+INSERT` that tripped inbound FKs. See the [DuckDB `UPDATE` Probe Findings](#duckdb-update-probe-findings) section below for live verification against the pinned DuckDB version before fixing.

- [x] **CRIT-02: `MergeCategories` / `MergePayees` fail when called via `InTx(tx)`** *(scheduled_split_items fixed in PR #143; `ErrMergeInTx` guard, two-tx `WithTx` serialization, and explicit column lists in `MergePayees` fixed on `fix/merge-in-tx-guard`)*
  - **Files:** `internal/category/category_service.go:607-680`, `internal/payee/payee_service.go:327-418`
  - **Issue:** ~~`MergeCategories` never updated `scheduled_split_items.category_id`~~ *(fixed in PR #143)*. Remaining: calling `InTx(tx).MergeCategories(...)` or `InTx(tx).MergePayees(...)` runs the trailing `DELETE` inside `tx`, hitting DuckDB's same-tx FK delete restriction.

- [ ] **CRIT-03: `payee.Repository.Delete` permanently destroys `payee_aliases` before failing on `scheduled_transactions` FK**
  - **Files:** `internal/payee/payee_repository.go:205-244`
  - **Issue:** Checks only `transactions` for dependents, deletes all `payee_aliases` outside a transaction, then runs `DELETE FROM payees`. When the payee is referenced by `scheduled_transactions`, all aliases are permanently lost before the FK error fires.

- [ ] **CRIT-04: `investment.Service.TransferShares` from non-lot account to lot-tracked account destroys shares (or panics)**
  - **Files:** `internal/investment/transfer_shares.go:125-142, 220-228`
  - **Issue:** When `srcAcct.TrackLots == false`, `srcLots` is `nil` and callers pass `lotAllocations == nil`. The source position is reduced, both ledger rows are inserted, and the `for i, alloc := range lotAllocations` loop runs 0 times — destination receives 0 lots (or panics on `srcLots[i]` if allocations were passed).

- [ ] **CRIT-05: Reversing a multi-lot share transfer deletes only ONE destination lot, duplicating the rest**
  - **Files:** `internal/investment/reverse.go:162-175`, `internal/investment/lot_repository.go:181-191`
  - **Issue:** `TransferShares` creates one destination lot per source lot allocation (all sharing `source_transaction_id = dstTxn.ID`), but `reverseTransferShares` calls `GetBySourceTransaction` (`QueryRow`), checking and deleting only a single lot on edit/delete.

- [ ] **CRIT-06: Deleting/editing a full sell on a non-lot account wipes out cost basis (`AverageCostPerShare = $0`); reversing buys allows negative shares/basis**
  - **Files:** `internal/investment/trades.go:206-209`, `internal/investment/reverse.go:73-96, 127-136, 180-202`
  - **Issue:** Selling to 0 shares deletes the `investment_positions` row; reversing that sell recreates it with `AverageCostPerShare = $0`. Reversing a buy/transfer-in after partial sells allows `newShares < 0` and negative `AverageCostPerShare`.

- [ ] **CRIT-07: `Sell` & `FeeLiquidation` with `commission > 0` use the Buy formula in `SmartCompute`**
  - **Files:** `internal/investment/computation.go:13-28, 40-78`, `internal/investment/trades.go:142-148`, `internal/investment/trades_income.go:124-131`, `internal/investment/total_return.go:81-128`
  - **Issue:** `ComputeTotalAmount` always adds commission (`shares*price + commission`) and `ComputePricePerShare` always subtracts it (`(total - commission) / shares`). For sells, net proceeds = `shares*price - commission`; giving price credits `+$20` too much cash on a `$10` commission, and giving net proceeds computes the wrong execution price and double-subtracts commission in `TotalReturn`.

- [ ] **CRIT-08: `realizedGainLotTracked` mixes post-split / post-spinoff `lot.CostPerShare` with pre-action sells & junctions**
  - **Files:** `internal/investment/total_return.go:100-131`, `internal/investment/corporate_action_split.go:88-106`, `internal/investment/corporate_action_spinoff.go:84-91`
  - **Issue:** Splits and spin-offs mutate open lots' `CostPerShare` in place while prior sells and `investment_transaction_lots` junctions stay at pre-action scale, inflating historical realized gains. `adjustLots` also scales `OriginalShares` while leaving pre-split `j.Shares` unscaled, breaking `Shares == OriginalShares - sum(junctions)`.

- [ ] **CRIT-09: `SpinOff` misallocates cost basis on fractional shares and drops basis completely when `wholeShares == 0`**
  - **Files:** `internal/investment/corporate_action_spinoff.go:84-131, 146-207`
  - **Issue:** Divides full fractional spin-off cost basis by floored `wholeShares` instead of `rawSpinOffShares` (assigning `$0` basis to cash-in-lieu fractional shares and inflating whole-share basis). When `wholeShares == 0`, parent basis is reduced while no spin-off lot/position is created, permanently dropping that basis. Lot-tracked accounts also floor per lot instead of per account.

- [ ] **CRIT-10: `transaction.Service.VoidTransaction` on a split-line counterpart corrupts parent/counterpart pairing**
  - **Files:** `internal/transaction/service_status.go:101-128`
  - **Issue:** Zeroes and voids the counterpart row without touching the parent split (or checking parent reconciled/closed status), leaving an active parent split pointing to a `$0` void row that later edits overwrite with non-zero amounts.

- [ ] **CRIT-11: `transaction.Service.UpdateSplit` breaks when converting between categorized and transfer-line splits**
  - **Files:** `internal/transaction/service_splits.go:148-215`
  - **Issue:** Both `wasTransfer && !isTransfer` (orphans counterpart) and `!wasTransfer && isTransfer` (skips `TransferID` minting, routing checks, and counterpart creation) fall through directly to `splitRepo.Update`. Also never checks `split.TransactionID == existing.TransactionID`.

- [ ] **CRIT-12: `transaction.planSplitReplacement` corrupts split/counterpart pairing when re-targeting with preserved `TransferID`**
  - **Files:** `internal/transaction/service_split_plan.go:59-103`
  - **Issue:** `matchByTransferID` matches an old transfer split without verifying `o.split.TransferAccountID.ID == ns.TransferAccountID.ID`, updating `transaction_splits.transfer_account_id` to Account C while leaving the counterpart row in Account B.

- [ ] **CRIT-13: `security.Repository.Delete` fails to check investment & corporate-action tables, allowing orphaned references**
  - **Files:** `internal/security/security_repository.go:332-364`, `internal/db/migrations/010_drop_security_fks.sql`
  - **Issue:** Migration 010 dropped all DB FKs referencing `securities(id)`, relying on app checks, but `Delete` only checks `security_prices` — missing `investment_transactions`, `investment_lots`, `investment_positions`, and `corporate_actions`.

- [ ] **CRIT-14: `undo.AutoPostCommand.Execute()` is a no-op — Redo does nothing and wedges subsequent Undo**
  - **Files:** `internal/undo/auto_post.go:50-52`, `internal/undo/manager.go:68-94`
  - **Issue:** Redoing an undone auto-post returns `nil` without re-creating transactions/transfers or advancing schedules, moves the command back onto `undoStack`, and causes the next Undo to fail with `NotFoundError` and wedge the stack.

- [ ] **CRIT-15: `undo.DeleteTransactionCommand.Undo()` corrupts DB when undoing deletion of a split-line counterpart**
  - **Files:** `internal/undo/transaction.go:100-143`, `internal/transaction/service_transfer_line.go:265-280`
  - **Issue:** `Delete` removes both the counterpart and the parent's `transaction_splits` row, but `Undo` only recreates the counterpart, leaving parent splits unbalanced and the counterpart orphaned (`MalformedPairError`).

---

## High — Serious Functional Bugs, Races, and Financial/Invariant Violations

### Core Ledger, Transfers & Schema

- [ ] **HIGH-01: `accounts.interest_rate` is `DECIMAL(5, 4)` (max `9.9999`), rejecting any loan APR ≥ 10%**
  - **Files:** `internal/db/migrations/034_account_type_hsa_investment.sql:68`, `internal/types/validation.go:172-179`
  - **Issue:** Domain validation permits `0..100%`, `loan` math expects percentage points (e.g., `12.5`), but `DECIMAL(5, 4)` overflows at `10.0%`. Widen to `DECIMAL(7, 4)`.

- [ ] **HIGH-02: `transfer.Service` skips closed-account checks on 5 verbs; `SetStatus`/`SetLegStatus` un-void transfers; `Restore` overwrites live/reconciled transfers**
  - **Files:** `internal/transfer/write.go:151-362`
  - **Issue:** `Delete`, `Void`, `SetStatus`, `SetLegStatus`, and `Restore` never check `IsClosed()`. `SetStatus`/`SetLegStatus` allow changing a voided `$0` transfer to `cleared` (and `SetLegStatus` can leave legs mismatched `cleared`/`void`). `Restore` never checks that legs are actually `void`, distinct (`legs[0].RowID != legs[1].RowID`), or valid/non-zero/equal-and-opposite.

- [ ] **HIGH-03: `transfer.Service.assembleSplitLine` hardcodes `CategoryID.Valid = true` with `NilID` for uncategorized splits**
  - **Files:** `internal/transfer/read.go:268, 303`
  - **Issue:** Every uncategorized split-line transfer returns `CategoryID{ID: NilID, Valid: true}` instead of `Valid: false`.

- [ ] **HIGH-04: `transfer.Service.LinkExisting` & `transferlink` allow orphaning transfers, linking void/split/closed/mismatched rows, and assigning system categories**
  - **Files:** `internal/transfer/write.go:415-452`, `internal/transferlink/transferlink.go:206-254`
  - **Issue:** Doesn't verify `!fromRow.IsTransfer() && !toRow.IsTransfer()`, `!IsVoid()`, equal-and-opposite non-zero amounts, zero splits, open accounts, or `ValidateTransferCategory`.

- [ ] **HIGH-05: Cross-currency transfers and transfer-linking have zero currency validation**
  - **Files:** `internal/transfer/guards.go:49-94`, `internal/transaction/service_transfer_line.go:32-68`, `internal/transferlink/transferlink.go:131-158`
  - **Issue:** Moving `100` from USD to JPY writes `-$100` and `+¥100` at 1:1; `transferlink.FindUnlinked` matches `-$100 USD` and `+€100 EUR` as a "Clean" auto-link pair.

- [ ] **HIGH-06: `transfer.Service.ReplaceWithTransfer` allows replacing a reconciled investment transaction (and reads outside tx)**
  - **Files:** `internal/transfer/create.go:135-168`

- [ ] **HIGH-07: `transaction.Service.Update` reverse-cascade is non-transactional and violates split/parent invariants**
  - **Files:** `internal/transaction/service_crud.go:51-137`
  - **Issue:** Updates `parentSplit` and `transaction` in separate autocommitted statements; doesn't check parent closed/void; allows setting `CategoryID` on a split transaction (double counting in reports); doesn't propagate date/account edits on a split parent to transfer-line counterparts.

- [ ] **HIGH-08: Split-line counterparts bypass target `OpeningDate` and closed-account freeze; `ValidateTransferCategory` never enforced on splits; `AddSplit` broken for transfer lines**
  - **Files:** `internal/transaction/service_transfer_line.go:32-68, 159-339`, `internal/transaction/service_splits.go:21-131, 285-366`
  - **Issue:** Regular targets skip `ValidateTransactionDate`; edit/delete cascades skip `ensureAccountOpen` and `IsVoid`; `ValidateTransferCategory` is never called in `internal/transaction/`; `CreateWithSplits`/`ReplaceSplits` allow splits on transfers; `AddSplit` skips all transfer-line logic and persists before checking split total mismatch.

- [ ] **HIGH-09: `transaction.Repository.GetByBankReferenceID` swallows all DB errors as not-found**
  - **Files:** `internal/transaction/transaction_repository.go:482-484`
  - **Issue:** Returns `(nil, nil)` on *any* error, not only `sql.ErrNoRows`, risking duplicate OFX imports on DB errors.

- [ ] **HIGH-10: `account.Repository.DeleteBlocker` misses `transaction_splits.transfer_account_id`; `CountLedgerRows` misses `reconciliation_sessions`**
  - **Files:** `internal/account/account_repository.go:413-418, 458-476`, `internal/transfer/ledger_move.go:145-154`
  - **Issue:** Deleting an account can leave dangling `transaction_splits.transfer_account_id`; converting a 0-txn account with an `in_progress` reconciliation session to investment strands that session permanently.

- [ ] **HIGH-11: Nil-pointer panics on nil optional dependencies/inputs**
  - **Files:** `internal/transaction/service_validation.go:24-32` (`s.accountRepo == nil`), `internal/transfer/ledger_move.go:217-220` (`plan == nil`)

### Investments & Prices

- [ ] **HIGH-12: Duplicate `LotID` entries in `lotAllocations` bypass share validation and corrupt lot balances**
  - **Files:** `internal/investment/trades.go:237-285`, `internal/investment/trades_income.go:231-280`, `internal/investment/transfer_shares.go:96-196`
  - **Issue:** Validates each allocation against a fresh `GetByID` copy and writes them sequentially, allowing selling 16 shares from a 10-share lot via `[{L1, 8}, {L1, 8}]`.

- [ ] **HIGH-13: `EnableLots` and `DisableLots` are not atomic (missing `runInTx`)**
  - **Files:** `internal/investment/backfill.go:213-220, 337-360`
  - **Issue:** Mid-operation failure leaves partial lots committed while `TrackLots` stays `false` (blocking retry), or destroys all tax lots before `RebuildPositions` fails.

- [ ] **HIGH-14: Same-day replay ordering breaks after `UpdateBuy` (`CreatedAt` changes)**
  - **Files:** `internal/investment/rebuild.go:82-88, 262-267`, `internal/investment/backfill.go:118-123`, `internal/investment/total_return.go:281-286`, `internal/investment/performance.go:159-166`
  - **Issue:** `UpdateBuy` deletes and inserts a new row with fresh `CreatedAt`. Same-day sorts use `CreatedAt ASC` only, so a same-day sell now replays *before* the edited buy and fails with insufficient shares. Order share-additions before share-removals on the same date.

- [ ] **HIGH-15: Stale `investment_positions` rows on lot-tracking accounts break `security.Service.Hide`, `SharesBySecurity`, and `Split.adjustPositions`**
  - **Files:** `internal/investment/rebuild.go:90-121`, `internal/security/security_service.go:206-214`, `internal/investment/queries.go:101-181`, `internal/investment/corporate_action_split.go:143-151`
  - **Issue:** `RebuildPositions` populates `investment_positions` even when `TrackLots == true`, but lot sells/transfers/mergers never clear it — leaving phantom shares once all lots are closed.

- [ ] **HIGH-16: Non-lot `Split` miscalculates a 2nd split and back-dated splits after a sell**
  - **Files:** `internal/investment/corporate_action_split.go:148-173`, `internal/investment/queries.go:202-227`
  - **Issue:** `netSharesHeldAsOf` sums raw transactions without earlier splits (two 2:1 splits on 10 shares yield 30 instead of 40); back-dated splits spread remaining cost over post-split shares incorrectly.

- [ ] **HIGH-17: `reverseSpinOff` inflates untouched lot-tracked positions and deletes non-transaction prices; `Merger`/`SpinOff` lack `source != target` & date-scoping**
  - **Files:** `internal/investment/corporate_action_reversal.go:125-205`, `internal/investment/corporate_action_merger.go:25-28, 132-169`, `internal/investment/corporate_action_spinoff.go:21-28, 84-131`

- [ ] **HIGH-18: Cash `Update*` methods do not verify old transaction type (and edit methods reset `Status` to `pending`)**
  - **Files:** `internal/investment/edit.go:65-404`
  - **Issue:** Passing a `buy`/`sell`/`exchange` ID to `UpdateDividend`/`UpdateDeposit`/etc. deletes the row without reversing lots/positions. Also `healInOwnTx` only heals the *new* `(accountID, securityID)` when account/security changes, and every `Update*` resets cleared status to `pending`.

- [ ] **HIGH-19: `performance.go` calculation bugs (post-spinoff buys cut, wrong annualization gate / `+Inf`, `solveXIRR` bracket overflow to `NaN`)**
  - **Files:** `internal/investment/performance.go:124, 137-141, 217-226, 289-315`

- [ ] **HIGH-20: `SmartCompute` accepts `<= 0` total/price and `< 0` commission; `listEverHeldSecurities` omits `TransactionTypeExchange`**
  - **Files:** `internal/investment/computation.go:40-78`, `internal/investment/total_return.go:357-385`, `internal/investment/valuation_service.go:163-174`

- [ ] **HIGH-21: `price.Service.RefreshPrices` checks `GetCurrentPrice(today)` instead of `GetBySecurityAndDate(quote.Date)`**
  - **Files:** `internal/price/refresh.go:140-170`
  - **Issue:** Overwrites yesterday's manual/transaction price with `SourceAPI` whenever today already has a price on file.

### Scheduled, Reconciliation & Reports

- [ ] **HIGH-22: `reconciliation.Service.CalculateClearedTotal` & `FinishReconciliation` do not check `txn.AccountID == accountID`, `status != 'void'`, or closed accounts**
  - **Files:** `internal/reconciliation/reconciliation_service.go:218-222, 238-302, 383-417`
  - **Issue:** Passing another account's transaction ID folds its amount into `clearedTotal` and permanently marks it `reconciled`; `FinishReconciliation` can also mutate a closed account.

- [ ] **HIGH-23: `scheduled` `postOccurrenceSingle` bypasses `transaction.Service.Create`**
  - **Files:** `internal/scheduled/post_occurrence.go:64-85`
  - **Issue:** Writes directly via `txnRepo.Create`, bypassing `opening_date`, zero-amount, and non-investment-account guards.

- [ ] **HIGH-24: Variable-amount estimation includes `void` transactions (`$0`) and writes unrounded sub-cent amounts (`-100.3333`)**
  - **Files:** `internal/scheduled/estimate.go:54-68`, `internal/scheduled/auto_post.go:256-286`
  - **Issue:** Voided `$0` rows drag down the average, and unrounded `Div()` writes 4-decimal amounts so bank reconciliation can never hit `$0.00`.

- [ ] **HIGH-25: `scheduled.Repository.ListDue` uses DuckDB `CURRENT_DATE` (UTC) instead of local `types.Today()`**
  - **Files:** `internal/scheduled/scheduled_repository.go:256`

- [ ] **HIGH-26: `scheduled.Service.AutoPost` drops `AutoPostSummary` on a mid-batch hard error**
  - **Files:** `internal/scheduled/auto_post.go:121-219`
  - **Issue:** Earlier candidates already committed in separate transactions are never reported or pushed onto the undo stack, and remaining candidates are skipped.

- [ ] **HIGH-27: `report.SpendingByCategory*` mixes currencies and binds raw local `time.Time` (dropping boundary days)**
  - **Files:** `internal/report/report_service.go:226-306`, `internal/report/report.go:47-65`
  - **Issue:** Sums USD and JPY together without grouping by `a.currency`; binds raw `time.Time` against `DATE` columns (timezone shift drops start or end boundary day outside UTC).

- [ ] **HIGH-28: `NetWorthReport` double-counts future-dated register↔investment transfers and disagrees with `AccountFigures`**
  - **Files:** `internal/report/report_service.go:92-175`, `internal/report/account_figures.go:52-121`, `internal/investment/queries.go:31-45`
  - **Issue:** With `post_lead_days > 0`, `registerBalancesAsOf(today)` excludes the future register leg while `cashBalanceOf` includes the future investment leg, inflating/deflating Net Worth.

### Import/Export & Undo

- [ ] **HIGH-29: `imexport.ImportService.Preview` mutates the database during dry-run**
  - **Files:** `internal/imexport/import_service.go:186-187, 369-381`, `internal/imexport/adapters.go:62-63`
  - **Issue:** Calls `ResolveOrCreate` during preview, inserting unseen payees into `payees` before confirmation or on Cancel.

- [ ] **HIGH-30: `imexport.WriteCSV` defers `writer.Flush()` after `return writer.Error()`**
  - **Files:** `internal/imexport/csv.go:240-269`
  - **Issue:** Silently discards flush errors on buffers < 4 KiB or final flush failures.

- [ ] **HIGH-31: `imexport.ParseCSV` greedy split folding double-counts splits; exporting transfer-line splits breaks CSV round-trip**
  - **Files:** `internal/imexport/csv.go:114-176, 220-234`, `internal/imexport/export_service.go:251-269`

- [ ] **HIGH-32: `imexport.ParseOFX` slices `content` with `strings.ToUpper` byte offsets (Unicode corruption/panic) and uses 64 KiB `bufio.Scanner` limit**
  - **Files:** `internal/imexport/ofx.go:17-27, 116-192`

- [ ] **HIGH-33: `imexport.ImportService.Execute` bugs (`ImportActionMatch` on transfers/reconciled, wrong `Updated` count, dropped splits, always returns `nil`)**
  - **Files:** `internal/imexport/import_service.go:277-347, 376-389, 428-447`

- [ ] **HIGH-34: `undo.VoidTransferCommand.Execute()` appends to `c.snapshots` without resetting → Undo after Redo always fails**
  - **Files:** `internal/undo/transfer.go:188-205`

- [ ] **HIGH-35: `undo.DeleteTransferCommand` fails on voided transfers; `EditTransferCommand` & `DeleteTransferCommand` lose per-leg statuses**
  - **Files:** `internal/undo/transfer.go:88-160`

- [ ] **HIGH-36: Non-atomic multi-step Undo/Redo commands (`PostScheduledTransfer`, `PostScheduledTransactionWithEdits`, `AutoPost`, `DeletePayee`, `CompoundCommand.Undo`)**
  - **Files:** `internal/undo/scheduled_transaction.go:183-255, 325-335`, `internal/undo/auto_post.go:56-100`, `internal/undo/payee.go:121-135`, `internal/undo/command.go:49-56`

### CLI & TUI

- [ ] **HIGH-37: Data races mutating live `*scheduled.Transaction` and `*importDialogState` inside Bubbletea `tea.Cmd` goroutines**
  - **Files:** `internal/tui/scheduled_dialog_app.go:58,64,351,427`, `internal/tui/scheduled_dialog_submit.go:227-274,402-404`, `internal/tui/scheduled_transfer_dialog.go:395-419`, `internal/tui/paycheck_wizard_app.go:283-284`, `internal/tui/loan_wizard_submit.go:428-431`, `internal/tui/import_dialog.go:358-457`
  - **Issue:** Mutates live UI-referenced structs on background goroutines while `View()` reads them; failed saves also corrupt in-memory display state.

- [ ] **HIGH-38: Missing `fileOf` tags and per-view request IDs allow cross-DB state pollution & out-of-order async overwrites**
  - **Files:** `internal/tui/refresh_prices.go:19-22,85-100`, `internal/tui/create_category_dialog.go:135-137,277-327`, `internal/tui/import_dialog.go:78-83`, `internal/tui/link_transfers_dialog.go:22-26`, `internal/tui/account_dialog.go:42-45`, `internal/tui/app.go:606-608`, `internal/tui/reconciliation_view.go:300-317,598`, `internal/tui/portfolio_view.go:93-150,653-666`

- [ ] **HIGH-39: `reloadAfterRestore` fails to close open dialogs; `switchDatabase` & `reloadAfterRestore` leave stale view state**
  - **Files:** `internal/tui/backup_dialog.go:222-250`, `internal/tui/file_dialog.go:536-553`

- [ ] **HIGH-40: Nil-pointer panic in `submitPriceDialog` when background price refresh finishes while dialog is open**
  - **Files:** `internal/tui/price_view.go:299-301`, `internal/tui/price_dialog.go:115,208`

- [ ] **HIGH-41: Unreachable `Esc` handler in `ViewPortfolio` breaks exiting Lot Detail mode**
  - **Files:** `internal/tui/app.go:552-587`, `internal/tui/portfolio_view.go:576-592`

- [ ] **HIGH-42: TUI investment editing bugs (pre-reverse `ListOpenLots` fails edits; cleared status reset to pending; silent corruption on hidden securities, closed accounts, unlisted currencies, `%.2f` truncation)**
  - **Files:** `internal/tui/investment_sell_dialog.go:105,142-166,300-376`, `internal/tui/investment_transfer_shares_dialog.go:148-184,335-352`, `internal/tui/investment_buy_dialog.go:132,162-163`, `internal/tui/investment_dividend_dialog.go:139,162-163`, `internal/tui/investment_fee_liquidation_dialog.go:83,112-113`, `internal/tui/account_dialog.go:255`, `internal/tui/price_dialog.go:75`, `internal/tui/security_view.go:530-538`

- [ ] **HIGH-43: `tmoney reconcile mark` is a no-op that persists nothing**
  - **Files:** `internal/cli/reconcile/mark.go:63-92`, `internal/cli/reconcile/finish.go:71-88`

- [ ] **HIGH-44: `--category "Parent:Subcategory"` is broken in `transaction add/edit` and `scheduled add/edit`**
  - **Files:** `internal/cli/transaction/edit.go:245-260`, `internal/cli/scheduled/edit.go:250-265`

- [ ] **HIGH-45: Tilde (`~/...`) paths broken in `launchTUI`, `tmoney db backup/restore`, and TUI import dialogs**
  - **Files:** `internal/cli/tui.go:56-71,84`, `internal/cli/db/backup.go:50,56`, `internal/cli/db/restore.go:52,60`, `internal/tui/price_import_dialog.go:80`, `internal/tui/import_dialog.go:424`

---

## Medium — Edge-Case Correctness, Precision, and UX Bugs

- [ ] **MED-01: `portfolio_holdings` SQL view returns `NULL` instead of `0` when no `investment_positions` row exists**
  - **Files:** `internal/db/migrations/034_account_type_hsa_investment.sql:270-280` (`COALESCE` is inside scalar subquery rather than wrapping it)
- [ ] **MED-02: `db.DB.SetMetadata` and `db.DB.Reindex` run outside `txMu` / `WithTx`; `db.Create` leaks `conn` if `reconnect()` fails before `old.Close()`**
  - **Files:** `internal/db/connection.go:303-306, 923-945`, `internal/db/reindex.go:37-78`
- [ ] **MED-03: `types` edge cases (`Quantity.Scan` missing `int64`, `Nullable*.Scan` sets `Valid = true` on scan error, `MaxLength`/`MinLength` count bytes instead of runes, `ServiceValidationError` missing `Unwrap()`)**
  - **Files:** `internal/types/types.go:390-427, 662-776`, `internal/types/validation.go:87-100, 303-309`
- [x] **MED-04: `category.Repository.Update` allows self-parent cycles, 3-level hierarchies, parent/child type mismatch; `Service.Update` allows setting `IsSystem = true`; `Delete` misses `scheduled_split_items` & `payees.default_category_id` guard** *(fixed on PR #143 alongside CRIT-01)*
  - **Files:** `internal/category/category_repository.go:288-303, 337-420`, `internal/category/category_service.go:494-509`
- [ ] **MED-05: `payee.Service` auto-create methods skip `validatePayee`**
  - **Files:** `internal/payee/payee_service.go:103-145, 187-204`
- [ ] **MED-06: `transaction.Repository.Search` misses split categories & truncates 4th decimal place via `CAST(? AS DECIMAL)`**
  - **Files:** `internal/transaction/transaction_repository.go:570-606`
- [ ] **MED-07: `transaction.Service.RestoreVoidedTransaction` skips closed-account & amount/status validation; `transfer.Service.Recreate` allows duplicate live legs**
  - **Files:** `internal/transaction/service_status.go:133-153`, `internal/transfer/write.go:369-402`
- [ ] **MED-08: `account.Service` missing `WithTx` on `Update`/`Close`, unguarded `OpeningDate`/`Active`/`ClosedDate` mutations, `GetBalance` wraps `sql.ErrNoRows` instead of `NotFoundError`**
  - **Files:** `internal/account/account_service.go:85-93, 171-217`, `internal/transfer/ledger_move.go:255`
- [ ] **MED-09: Split ratios use `float64` (`NewFromFloat`) and ~15 investment sites use reciprocal-first `a.Mul(1 / b)` instead of `a.Div(b)`**
  - **Files:** `internal/investment/corporate_action_split.go:28-29,239-240`, `internal/investment/corporate_action.go:292-295`, `internal/investment/computation.go:18-19`, `internal/investment/position.go:69`, `internal/investment/holdings.go:104-105,227-228`
- [ ] **MED-10: `CorporateActionService.DeleteAction` ignores later corporate actions; `ParseSplitRatio` accepts trailing garbage & 1:1 no-ops; `SetClearedStatus` uses full-row `Update`**
  - **Files:** `internal/investment/corporate_action_reversal.go:219-244`, `internal/investment/corporate_action.go:194-243`, `internal/investment/investment_service.go:143-157`
- [ ] **MED-11: `price` edge cases (`BulkImport` non-transactional, `YahooProvider` unescaped ticker / missing `Regular.End > 0` / intraday leak in `FetchQuoteOn`, timezone `NotFutureDate` rejects Asian/Australian closed quotes, `ListIncomeOnlyTransactionPrices` consecutive fallback, `security.Service.Resolve` swallows DB errors)**
  - **Files:** `internal/price/price_service.go:122-137`, `internal/price/yahoo_provider.go:113,219,252`, `internal/price/refresh.go:161-169`, `internal/investment/price_cleanup.go:58-73`, `internal/security/security_service.go:118-130`
- [ ] **MED-12: `reconciliation.StartReconciliation` deletes active session before validation & outside tx; `ReopenSession` allows two concurrent `in_progress` sessions**
  - **Files:** `internal/reconciliation/reconciliation_service.go:108-126, 363-377`, `internal/undo/reconciliation.go:75-83`
- [ ] **MED-13: `scheduled` validation & tx gaps (`Update` skips `ensureNoClosedAccounts`, `Delete` non-transactional across splits+parent, `Create`/`Update` allow unsupported `inv↔inv` categories & system categories on transfer splits, `Validate` skips `SecondaryDayOfMonth`/`Interval` on semimonthly/zero `EndDate`, `FindLoanSchedule` returns completed schedules)**
  - **Files:** `internal/scheduled/scheduled_service.go:151-233`, `internal/scheduled/validation.go:34-106`, `internal/scheduled/scheduled.go:475-476,530-564,690-712`, `internal/scheduled/loan_projection.go:19-33`
- [ ] **MED-14: `loan.Project` can emit payment #1 before `nextDate` & breaks on `dayOfMonth == 0`; `SplitPayment`/`BuildLoanSnapshot` lack `owed <= 0` / `apr < 0` guards**
  - **Files:** `internal/loan/loan.go:93-115, 163-195, 237-252`, `internal/scheduled/loan_build.go:51-109`
- [ ] **MED-15: `undo` edge cases (transfer Undo/Redo mints new IDs breaking stacked commands; `Merge*` commands wedge `Manager.Undo()`; `DeleteTransactionCommand.Undo()` applies payee default category onto uncategorized txns)**
  - **Files:** `internal/undo/transfer.go:40-55,154-160`, `internal/undo/category.go:148-152`, `internal/undo/payee.go:169-173`, `internal/undo/transaction.go:134-139`
- [ ] **MED-16: `imexport` edge cases (`TransferAccount` ignored on import; UTF-8 BOM breaks CSV headers; `ParseOFX` doesn't unescape XML entities; `ParseQIF` zeroes bad splits & misparses multi-section QIF; `WriteQIF` allows newline injection; `ExportService` uses `Float64()` & loses multi-account QIF ownership; `Matcher` matches void txns & punctuation-only payees)**
  - **Files:** `internal/imexport/import_service.go:393-450`, `internal/imexport/csv.go:42-86`, `internal/imexport/ofx.go:53-54,159-199`, `internal/imexport/qif.go:127-142,247-295,330-382`, `internal/imexport/export_service.go:116-156,215,266`, `internal/imexport/matcher.go:125-129,346-361`
- [ ] **MED-17: `config` & `applog` hardening (`config.Save` uses fixed `.tmp` without `Sync()` and world-readable perms; `config.go` imports `"testing"` with unsynchronized `saveDisabled`; `applog.Append` ignores `Close()` error and allows newline injection)**
  - **Files:** `internal/config/config.go:8,18,126-174`, `internal/applog/applog.go:36-49`
- [ ] **MED-18: `chart` & `termimg` edge cases (`chart.Pie` allocates before checking `w <= 0 || h <= 0` and produces `NaN` on zero-sum slices; `termimg.Placement` can return `cols == 0`; `Probe` busy-spins on PTY EOF)**
  - **Files:** `internal/chart/pie.go:28-31,109-138`, `internal/termimg/termimg.go:165-171`, `internal/termimg/probe_unix.go:38-55`
- [ ] **MED-19: TUI & CLI usability/state bugs (empty dir traps file browser; `applyScheduledDueCount` wipes statusbar notifications; Dashboard `"in 0 days"` UTC-midnight bug; stuck in `ViewReconciliation` on load error; `price_import_dialog` hardcodes `"USD"`; closed-account transfer names vanish in Register/Reconciliation; `portfolio_view.go:495` nil `valuation` deref; `switchDatabase` keeps `prevDB` exclusively locked and skips its quit backup; `cmdutil.OpenServices` writes auto-post to `os.Stdout` and mutates DB on read-only CLI commands)**
  - **Files:** `internal/tui/file_dialog.go:489-502,640-643`, `internal/tui/scheduled_view.go:465-511`, `internal/tui/dashboard_view.go:684,693`, `internal/tui/reconciliation_view.go:233,519-521`, `internal/tui/register_view.go:104`, `internal/tui/price_import_dialog.go:102`, `internal/tui/portfolio_view.go:495`, `internal/cli/cmdutil/services.go:47-53`

---

## Low — Minor Bugs & Cleanup

- [ ] **LOW-01:** `category.Service.seedCategory` skips children if parent already exists (`internal/category/category_service.go:253-288`).
- [ ] **LOW-02:** `payee.Repository.FindPayeeByPattern` matches in alphabetical `pattern` order instead of specificity (`internal/payee/payee_repository.go:403-438`).
- [ ] **LOW-03:** `transfer.Service.Delete` allows deleting void transfers & `Void` omits `"**VOID**"` memo, diverging from `transaction.Service` (`internal/transfer/write.go:263-264,341-362`).
- [ ] **LOW-04:** `transaction.Split.Validate` doesn't validate `PaycheckSection` enum; `transferlink.Service` has unused `db` field & dead rollback code.
- [ ] **LOW-05:** `backup.enforceRetention` aborts loop on first `os.Remove` error (`internal/backup/backup.go:550-557`).
- [ ] **LOW-06:** `report.SpendingByCategory` doesn't deterministically sort `Subcategories` (`internal/report/report_service.go:391-403`).
- [ ] **LOW-07:** Swallowed DB errors in `investment/holdings.go:266-280`, `queries.go:71-74`, `auto_price.go:30-38`; unescaped SQL `LIKE` wildcards in `security_service.go:247-257`; stale `UnsupportedReversalError` text; dead `CanHide()`/`CanDelete()` methods.
- [ ] **LOW-08:** TUI/CLI polish: `menubar.go` uses `len(label)` instead of `lipgloss.Width`; `corporate_action_history.go:390` unclamped `a.height - 10`; `MenuActionDashboard` skips reload; `afterAccountDialogSave` doesn't reload investment views; `ViewSecurities` search resets on reload; `afterLoanWizardSave` always prints `"Loan created."`; `cli/price/import.go` drops `hiddenSkipped` from summary; `cli/export.go` stale error message & deferred `Close()` ignoring error; `cli/report/spending.go` doesn't reject conflicting period flags.

---

## Architectural & Idiomatic Go Improvements

- [ ] **ARCH-01: Thread `context.Context` end-to-end (`db.Queryer`, `WithTx`, repositories, services, `price.Provider`)**
  - Enables cancellation and deadlines for long queries (`transferlink`, reports), HTTP price refreshes (`time.Sleep` + `http.Client`), and TUI background `tea.Cmd`s on view/DB switch or SIGHUP.
- [ ] **ARCH-02: Replace `WHERE CAST(id AS VARCHAR) = ?` with `WHERE id = CAST(? AS UUID)` & fix N+1 queries**
  - Unblocks DuckDB ART index lookups across repositories; batch `transferlink.FindUnlinked` (currently N+1 + O(N²)) and `ValuationService.GetAccountValuation` (currently 10+ scans of `investment_transactions` + duplicate `GetCurrentPrice` per holding).
- [ ] **ARCH-03: Enforce uniform `WithTx` & read-inside-tx discipline across all services and multi-step undo commands**
  - Add `InTx` to `account.Service` (and move raw SQL out of service into `account.Repository`); move pre-write reads inside `runInTx`; execute/undo multi-step undo commands inside a single `WithTx` and remove irreversible `Merge*` operations from `undo.Command`.
- [ ] **ARCH-04: Single source of truth for lot-tracking vs position-tracking + replay-based investment edits**
  - Stop persisting stale `investment_positions` for `TrackLots == true` accounts (or gate every reader/writer uniformly on `acct.TrackLots`); extract shared `validateAndConsumeLots` helper; shift investment edits to mutate-event + replay-from-history inside `WithTx`.
- [ ] **ARCH-05: Finish decomposing `tui.App` and enforce uniform view/async contracts**
  - Add `onKey(msg) (handled bool, cmd tea.Cmd)`, `reset()`, and `onLeave()` to every view in `internal/tui/views.go`; unify `resetForNewDatabase()`; require `fileOf` + per-view `reqID` on async messages; value-copy structs before `tea.Cmd` mutations; wire investment/security/price/corporate-action TUI actions into `undo.Manager`.
- [ ] **ARCH-06: Idiomatic Go error inspection, types, and package hygiene**
  - Replace bare `err == sql.ErrNoRows` and direct type assertions (`err.(*dberrors.NotFoundError)`, etc.) with `errors.Is`/`errors.As`.
  - Remove `"testing"` import from production `internal/config/config.go`; deduplicate XDG path resolution with `filepath.IsAbs` check and `0700`/`0600` permissions.
  - Change `report.Service` date APIs from `time.Time` to `types.Date`.
  - Consolidate the 4 CLI category resolvers onto `internal/cli/category/resolve.go`, deduplicate `addMonths`/`lastDayOf` between `loan` and `scheduled`, and centralize `db.ExpandHome` at CLI/TUI/backup boundaries.
  - Pass explicit `io.Writer`s (Cobra's `cmd.ErrOrStderr()`) into `cmdutil.OpenServices` / `AutoBackupAfterModification`; replace positional `INSERT INTO ... SELECT *` in `payee.Service.MergePayees` with explicit column lists.

---

## DuckDB `UPDATE` Probe Findings

Verified empirically against the pinned DuckDB version (**DuckDB `v2.0.0`** via `github.com/duckdb/duckdb-go/v2 v2.20000.0`) using TMoney's full migrated schema (probe test run and removed after confirming behavior).

| Scenario on FK-referenced row | Result in DuckDB v2.0.0 | Takeaway |
|---|---|---|
| `UPDATE categories SET updated_at = ...` (non-indexed, non-FK col) | **PASS** | True in-place update works |
| `UPDATE categories SET name = ...` (with `idx_categories_name_parent`) | **FAIL** (FK violation) | Secondary index still forces `DELETE+INSERT` rewrite |
| `UPDATE categories SET parent_id = ...` (with secondary indexes) | **FAIL** (FK violation) | Blocked by both secondary index and outbound FK |
| `DELETE + INSERT` inside `WithTx` on referenced category | **FAIL** (FK violation) | Cannot wrap `DELETE+INSERT` in a tx for referenced rows |
| `UPDATE payees SET notes = ...` (non-indexed, non-FK col) | **PASS** | True in-place update works |
| `UPDATE payees SET name = ...` (with `idx_payees_name`) | **FAIL** (FK violation) | Secondary index still forces `DELETE+INSERT` rewrite |
| `UPDATE payees SET default_category_id = ...` (with `idx_payees_name`) | **FAIL** (FK violation) | Blocked by outbound FK enforcement index |
| **After dropping `idx_categories_*` & `idx_payees_name`:** `UPDATE categories SET name = ...` | **PASS** | Dropping secondary indexes unblocks renaming referenced categories (same as migration 021 for `accounts`) |
| **After dropping `idx_categories_*` & `idx_payees_name`:** `UPDATE payees SET name = ...` | **PASS** | Dropping `idx_payees_name` unblocks renaming referenced payees |
| **After dropping secondary indexes:** `UPDATE categories SET parent_id = ...` | **FAIL** (FK violation) | Outbound FK (`parent_id REFERENCES categories(id)`) still forces `DELETE+INSERT` rewrite |
| **After dropping secondary indexes:** `UPDATE payees SET default_category_id = ...` | **FAIL** (FK violation) | Outbound FK (`default_category_id REFERENCES categories(id)`) still forces `DELETE+INSERT` rewrite |
| Same-tx dereference all child FKs + `DELETE FROM payees` | **FAIL** (FK violation) | Split-tx trailing delete workaround in `MergeCategories`/`MergePayees` is **still required** in DuckDB v2.0.0 |
| `accounts`: `CREATE INDEX` + `UPDATE name` on referenced account (before & after `reconnect()`) | **FAIL** (FK violation) | `reconnect()` does not avoid indexed-column `DELETE+INSERT` FK errors |
| `UPDATE accounts SET interest_rate = 10.0` (`DECIMAL(5, 4)`) | **FAIL** (out of range) | Confirms **HIGH-01** (`DECIMAL(5,4)` overflows at `10.0%`) |

### Recommended Fix Path for CRIT-01 (Given DuckDB v2.0.0 Behavior)

1. Add a schema migration (following the precedent of migrations `010`, `013`, `016`, `021`, `026`) that:
   - Drops the low-value secondary indexes on these small tables: `idx_categories_name_parent`, `idx_categories_parent`, `idx_categories_type`, and `idx_payees_name`.
   - Drops the two outbound FK constraints on parent-referenced tables — `categories.parent_id REFERENCES categories(id)` and `payees.default_category_id REFERENCES categories(id)` — while enforcing referential integrity at the repository/service layer (verifying `default_category_id` exists in `payee.Repository.Create`/`Update` and guarding `payees.default_category_id` in `category.Repository.Delete`).
2. Replace `DELETE + INSERT` in both `category.Repository.Update` and `payee.Repository.Update` with a single true in-place `UPDATE` statement (`SET name = ?, ... WHERE id = CAST(? AS UUID)`).
3. Keep the inbound FKs from child tables (`transactions`, `transaction_splits`, `payee_aliases`, `scheduled_transactions`, `scheduled_split_items`) intact — in-place `UPDATE` on `categories` and `payees` will now pass cleanly even when referenced, and no `Update` failure can ever delete an existing row.
