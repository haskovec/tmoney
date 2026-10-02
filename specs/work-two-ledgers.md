# Work list: two ledgers, and the defects around them

**Date:** 2026-09-27
**Status:** DONE. Every item in the order table has shipped; each item keeps its problem statement and as-built notes.
**Source:** Code review of the tree on 2026-09-27. Not a pull-request diff.
**Decisions:** Design interview on 2026-09-27. It added W3a and W11, split W5 into W5a to W5d, and changed W2, W3, W4, W6, W7, W8, W9, and W10. Each changed item has a **Decision** line. W12 came later, from the review of PR #46, W13 from the smoke test of W8 phase 1, and W14 from VL-405 in the view-layer plan.

Use this file as the queue. Do one work item at a time. Do not start an item whose **Needs** line is still open. When an item ships, change its status line to the commit, and do not delete the problem statement. The next reader needs to know why the code looks the way it does.

Line numbers below are positions on 2026-09-27. Trust the function name if a line has moved.

## Words

Use these words the same way in every item.

| Word | Meaning |
| --- | --- |
| Register ledger | Rows in `transactions`. Checking, savings, credit card, cash, loan, asset, and HSA cash use this ledger. |
| Investment ledger | Rows in `investment_transactions`, plus `investment_lots` and `investment_positions`. Account types `investment` and `hsa_investment` use this ledger. |
| Register balance | `opening_balance` plus non-void amounts in the register ledger. This is what `account_balances` computes today. |
| Investment value | Cash in the investment ledger plus the market value of open holdings. `investment.ValuationService.GetAccountValuation` already computes this (`TotalValue`, `CashBalance`, `MarketValue`). |

`account.Type.IsInvestmentType()` is the gate. It is true only for `investment` and `hsa_investment`. HSA cash (`hsa`) stays on the register ledger.

## Already done — do not repeat

The review in `specs/code-quality-review.md` (2026-07-23) is stale on these points. They shipped:

| Topic | Where it lives now |
| --- | --- |
| One database transaction for a multi-row write | `db.DB.WithTx` in `internal/db/tx.go` |
| One owner for cash transfers | `transfer.Service` in `internal/transfer` |
| Split of the large service files | `specs/design-service-decomposition.md` (complete 2026-08-07) |
| Modal dialog registry | `specs/design-tui-decomposition.md` (slices 4a, part of 4c, 4d) |
| Undo cleared on database switch | `App.switchDatabase` calls `undoManager.Clear()` (`internal/tui/file_dialog.go`) |

`specs/design-tui-view-layer.md` is still a proposal. Its queue is `specs/implementation-plan-tui-view-layer.md`. Item W8 below points to that plan. Do not start it from the old review text.

## Order

Do the items in the table order. Each item is one branch and one pull request. W5 is four pull requests (W5a to W5d).

| ID | Status | Title | Needs | Size |
| --- | --- | --- | --- | --- |
| W1 | done (#46) | Corporate Actions help | — | small |
| W2 | done (#47) | Reload Reconciliation and Corporate Actions | — | small |
| W3a | done (#48) | Merger ratio must mean target shares per source share | — | small |
| W3 | done (#49) | Merger cash and ratio must be decimal | W3a | small |
| W4 | done (#50) | Share-transfer edit must keep both legs | — | medium |
| W5a | done (#51) | Close refuses an investment account that is not empty | — | medium |
| W5b | done (#52) | Delete counts the ledger of the account type | — | small |
| W6 | done (#53) | Refuse reconcile on an investment account | — | small |
| W5c | done (#54) | One display figure for account list, show, and balance | — | medium |
| W5d | done (#55) | Net worth by currency, with row errors | W5c | medium |
| W11 | done (#56) | As-of net worth leaves out accounts not yet open | W5d | small |
| W7 | done (#57) | Constructor must not write | — | medium |
| W10 | done (#58) | Correct `docs/ARCHITECTURE.md` | W5c | small |
| W12 | done (#60) | Corporate Actions keys must reach the view | — | small |
| W8 | done (#59, #61) | One view table in the TUI | W1, W2, and W12 | large |
| W13 | done (#62) | The Prices detail hint must show | — | small |
| W14 | done | Each investment load guards the service it calls | — | small |
| W9 | done (#63) | Stop exporting repositories from `app.Services` | W5c | large |

The data-safety fixes (W3a, W4, W5a, W5b, W6) go before the display work (W5c, W5d). W6 does not need W5. Its error text does not name a balance.

## W1 — Corporate Actions help

**Status:** done in PR #46.
**Also:** This is phase 0 of `specs/implementation-plan-tui-view-layer.md` (VL-001 and VL-002). Ship it here. Then mark VL-001 and VL-002 with this item's commit.

### Problem

The Corporate Actions view has keys. The `?` overlay does not list them. The status bar does list them. The destructive key is `d` (reverse the action, after a confirm). A user who presses `?` cannot discover it.

`viewShortcutSections` (`internal/tui/help_overlay.go`) has a case for ten views. It has no case for `ViewCorporateActions`. The function then appends Dialogs and Mouse only. The view's own keys are absent.

`TestViewShortcutSections` (`internal/tui/help_overlay_test.go`) lists six views. It does not list `ViewCorporateActions`, `ViewPrices`, `ViewInvestmentRegister`, `ViewPortfolio`, or `ViewAmortization`. A missing case stays green.

The status bar already states the contract (`internal/tui/app_view.go`, `ViewCorporateActions` arm):

```text
↑↓ navigate  / filter  enter details  d delete  esc back
```

The key handler is `handleCorporateActionViewKeys` (`internal/tui/corporate_action_history.go`). It also binds `g` / `G`, `pgup` / `pgdown`, and `home` / `end`.

### Fix

1. Add `corporateActionShortcuts()`. Match the status-bar line, and add `g` / `G` and page keys for parity with the amortization section.
2. Add the `ViewCorporateActions` arm in `viewShortcutSections`.
3. Replace `TestViewShortcutSections` with one loop over every `View` constant. For each constant, the result must contain a section that is not Global, Navigation, Dialogs, or Mouse. `View(999)` may stay the unknown fallback. Do not hard-code a count of views in the test. A new view with no arm must fail this test.
4. Get the constants from the `View` const block in `app.go` with `go/ast`, as VL-001 says. Fail if the set is empty. Put the enumerator in a shared test helper, because VL-101 uses it again. Do not add a sentinel constant to production code.

### Done when

- `?` on Corporate Actions shows `/`, Enter, `d`, and Esc.
- The new test fails if the `ViewCorporateActions` arm is removed.

### Do not

- Do not build the view table (W8) in this change.
- Do not change the keys.

## W2 — Reload Reconciliation and Corporate Actions

**Status:** done in PR #47.
**Decision:** A reload keeps the check marks that are still candidates. It drops the others, and it recalculates the cleared total.

### Problem

`reloadCurrentView` (`internal/tui/app_helpers.go`) reloads the sidebar and then the active view. The switch has no arm for `ViewReconciliation` or `ViewCorporateActions`.

Undo, redo, and Esc-back call this function. A save or an undo on those views leaves the table on screen stale. Corporate Actions does reload on its own success path (`refreshAfterCorporateAction`). Undo does not use that path.

`specs/design-tui-view-layer.md` records the nil reload as an open question and tells phase 1 to preserve it. That preservation is the bug. Fix it here. When W8 builds the table, copy the fixed arms.

The only reconciliation loader, `loadReconciliationData` (`internal/tui/reconciliation_view.go`), always builds an empty `checkedIDs` map, and it computes the cleared total with no checked rows. The check marks live only in memory until Finish. A reload through that loader, as it is, erases the user's work.

### Fix

1. `ViewReconciliation`: when a session is on screen, reload through `loadReconciliationData` with that session and account. If no session is on screen, reload nothing extra. Do not start a session.
2. Keep the check marks. `loadReconciliationData` takes the set of checked ids to keep. The start path passes none. The reload path passes the current set. The loader keeps each id that is still in the new candidate list, drops the rest, and computes the cleared total from the kept ids.
3. `ViewCorporateActions`: call `loadCorporateActionViewData`.
4. Add a test that sets `currentView` to each of those two values, runs the command returned by `reloadCurrentView`, and asserts the matching loaded message (or a non-nil load command). Follow the style of the existing view-load tests.
5. Add a test for the check marks: check two candidate rows, remove one of them from the candidates, and reload. One mark stays. The cleared total equals the total for that one row.

### Done when

- Undo on Corporate Actions refreshes the history table.
- Undo on Reconciliation refreshes the candidate table when a session is open, and the check marks on rows that still exist stay.
- The test names both views. A deleted arm fails the test.

### Do not

- Do not change focus behavior. Amortization and Corporate Actions focus the table at different times. W8 records why. Leave that alone.

## W3a — Merger ratio must mean target shares per source share

**Status:** done in PR #48.
**Decision:** The ratio is target shares per source share. That is the market convention, and both user interfaces already say it. The service math is wrong. A migration inverts the ratio on stored rows.

### Problem

The merger ratio has two opposite meanings in the tree:

| Place | Meaning of ratio `2.0` |
| --- | --- |
| TUI field comment (`internal/tui/corporate_action_merger_dialog.go`, Exchange Ratio field) | 1 source share becomes 2 target shares |
| CLI flag help (`internal/cli/investment/merge.go`) | "Shares of target received per source share" |
| Service math (`mergerProcessLots` and `mergerProcessPositions`, `internal/investment/corporate_action_merger.go`) | `newShares = oldShares × (1/ratio)`: 100 source shares become 50 target shares |
| Tests (`internal/investment/corporate_action_service_test.go`) | "Target position: 100/2 = 50 shares" |

The CLI example `--exchange-ratio 0.5` thus gives twice the shares that the help promises. The confirm screen (`corporate_action_merger_confirm.go`) shows the current shares and the ratio. It does not show the result, so the user cannot see the error before commit.

Merger reversal is not supported (`UnsupportedReversalError` in `corporate_action_reversal.go`). Only two readers parse a stored merger row: the TUI history text (`corporate_action_history.go`) and `tmoney investment actions` (`internal/cli/investment/actions.go`). Thus the stored ratio feeds display only. After the math changes, an old row shows its ratio with the new meaning, which is wrong.

The file comment at the top of `corporate_action_merger.go` says the code pays cash-in-lieu for a fractional share. No code does this. Fractional target shares stay in the account.

### Fix

1. Lot path and position path: `newShares = oldShares × ratio`. `newCostPerShare = oldCostPerShare ÷ ratio`, with decimal division. The total cost basis of each lot and position does not change.
2. Cash consideration stays per **source** share: `CashPerShare × old shares`. Do not change it.
3. W3a keeps the `float64` fields. W3 replaces them. Do not do W3's work here.
4. Add a numbered migration after the last one in `internal/db/migrations/`. For each `corporate_actions` row with `action_type = 'merger'`, set `exchange_ratio` in `parameters` to `1 / exchange_ratio`. Keep every other key as it is (for example, with `json_merge_patch`). The bundled DuckDB driver has the `json_*` functions (checked on 2026-09-27). A non-terminating inverse such as `1/3` is acceptable, because the value feeds display only.
5. Correct the cash-in-lieu comment. Say that fractional target shares stay.
6. Update the existing merger tests to the new direction. Do not delete them.

### Tests

| Case | Assert |
| --- | --- |
| Ratio 2, 100 source shares at 10.00 | 200 target shares at 5.00. Total cost basis is 1,000.00 before and after. |
| Ratio 0.5, lot path and position path | Both paths give the same shares and cost basis. |
| Cash 5.00 per share, ratio 2, 100 source shares | Cash on the ledger is 500.00. |
| Migration, merger rows with ratio 2 and ratio 0.5 | After the migration: 0.5 and 2. Other keys unchanged. Split and spin-off rows unchanged. |

### Done when

- The service math, the TUI comment, the CLI help, and the CLI example all agree.
- The migration test passes on a file that has old merger rows.

### Do not

- Do not add merger reversal.
- Do not add cash-in-lieu.

## W3 — Merger cash and ratio must be decimal

**Status:** done in PR #49, as a cleanup. See the correction below.
**Needs:** W3a.
**Decision:** New rows write bare JSON numbers with exact digits, for example `{"exchange_ratio":1.1,"cash_per_share":0.1}`. The on-disk shape does not change, so old rows, new rows, and older binaries can all read the file.

### Problem

`MergerParams.CashPerShare` is a `float64` (`internal/investment/corporate_action.go`). `Merger` converts it with `types.NewMoneyFromFloat` (`internal/investment/corporate_action_merger.go`, both the lot path and the position path). Binary floating point cannot hold 0.10 exactly. The cash row on the investment ledger can be wrong by a fraction of a cent.

`ExchangeRatio` is also a `float64`. `Merger` does `alpacadecimal.NewFromFloat(params.ExchangeRatio)` and `NewFromFloat(1.0 / params.ExchangeRatio)`. That ratio is not money, but it scales share counts. A ratio such as `2/3` is already inexact before the decimal library sees it.

**Correction (2026-09-27, found while building W3).** The two paragraphs above overstate the damage. `NewFromFloat` and `NewMoneyFromFloat` write the shortest decimal that maps back to the same float. Thus a typed value with 15 or fewer significant digits, such as `0.10` or `1.1`, comes back exact. A longer value comes back different in memory, but storage rounds shares to 8 places and cost to 4, so the stored rows are the same. The W3 exact-decimal test passes on the old float path too. The real float damage was `NewFromFloat(1.0 / params.ExchangeRatio)`, and W3a removed it. W3 shipped as a cleanup: it keeps floats off the money path so that this stays true for any input.

The float starts at the input, not only at storage. The CLI parses both flags with `strconv.ParseFloat` (`internal/cli/investment/merge.go`). The TUI dialog does the same (`internal/tui/corporate_action_merger_dialog.go`), and the confirm data carries `float64` (`corporate_action_merger_confirm.go`). The history text in the TUI and the CLI formats the fields with `%.2f`.

Stored corporate actions keep these fields as JSON numbers (`ToJSON` / the parser tests in `corporate_action_jsonv2_test.go`). Old rows must still load.

`types.Money` has no JSON methods. Do not add them in this item. That would change the JSON shape of every struct that holds `Money`.

`alpacadecimal.Decimal.UnmarshalJSON` reads the raw token. It accepts `1.5` and `"1.5"`, and both are exact. Go wrote the old rows in the shortest round-trip form, so an old `0.1` is the text `0.1` on disk.

### Fix

1. Change both `MergerParams` fields to a small local decimal type in `internal/investment`. It wraps `alpacadecimal.Decimal`. `MarshalJSON` writes the decimal text as a bare number. `UnmarshalJSON` uses the decimal library, so an old number and a quoted string both load exactly. It has an `IsZero` method, so `omitzero` still drops a zero cash field.
2. Parse the CLI flags and the TUI fields from text directly into a decimal. Do not call `strconv.ParseFloat` on the merger path.
3. Cash: build `types.Money` from the decimal with `types.NewMoneyFromDecimal`. Do not call `NewMoneyFromFloat`.
4. Ratio: multiply shares by the decimal ratio. Divide cost per share by the decimal ratio (W3a direction). Do not form `1.0 / ratio` in `float64`.
5. Keep `Validate`: ratio must be positive, cash must not be negative.
6. History text in the TUI and the CLI: format the decimal. Do not convert it to `float64` for `%.2f`.
7. `TestCorporateActionParamsSurviveJSONV2` must still pass. v1 and v2 must write the same bytes.
8. Add a test that posts cash-per-share `0.10` and ratio `1.1`. Assert the ledger cash and the resulting shares are the exact decimal results (100 source shares give 110 target shares and 10.00 cash). Assert the JSON of a new action loads, and that an old JSON number loads.

### Done when

- No production call in the merger path uses `NewMoneyFromFloat`, `NewFromFloat`, or `strconv.ParseFloat`.
- An old corporate-action row with `"cash_per_share": 1.5` still parses.
- A new row writes bare numbers.
- The new test locks 0.10 cash. (It is a regression lock. It also passes on the old float path; see the correction.)

### Do not

- Do not rewrite split or spin-off parameters in this change unless they use the same `NewMoneyFromFloat` path for a money amount. If they do, fix that call in the same change and name it in the commit. Do not open a second float cleanup.

## W4 — Share-transfer edit must keep both legs

**Status:** done in PR #50. See "As built" at the end of this item.
**Decision:** An edit that starts from the destination leg is refused. The error names the source account.

### Problem

`EditService.UpdateTransferShares` (`internal/investment/edit.go`) edits a share move. The screen calls it from `internal/tui/investment_transfer_shares_dialog.go`.

The function checks `TransferID.Valid`, then uses `TransferAccountID` as follows:

- The closed-account check runs only when `TransferAccountID.Valid` is true.
- The lookup always calls `ListByAccount(srcOld.TransferAccountID.ID)`. An invalid id is the zero UUID. The list does not contain the other leg.
- When the other leg is nil, the function still reverses the source, deletes the source, and calls `TransferShares` to write a new pair. The old destination row stays. That account keeps the old shares and receives the new shares.

`investment.Repository.ListByTransferID` already exists. The counterpart service uses it. This edit does not.

The function also calls `healInOwnTx` on the **new** source and destination accounts before it reverses the **old** rows. An edit that changes accounts repairs the wrong accounts first. Reverse then runs on stale lots. The heal also uses the new security for both accounts, so an edit that changes the security does not heal the old security.

The function assumes that its argument is the source leg. Both legs have the type `transfer_shares`. The source leg has a negative `TotalAmount`, and the destination leg has a positive one (`TransferShares`, `internal/investment/transfer_shares.go`). `reverseTransferShares` (`internal/investment/reverse.go`) uses that sign to find the direction. The dialog always uses the register's account as the source, and it fills "To" from `editTxn.TransferAccountID`. Thus an edit that starts from the destination account's register writes a new transfer in the **opposite** direction, even when the user changes nothing. The result is an "insufficient shares" error, or shares that move back silently if the account holds more of that security.

`internal/transfer/write.go` names this function as the example of an edit that gets the account check wrong. Cash transfers were fixed. Share transfers were not. Cash and shares stay in different owners (`transfer.Service` owns cash only; `ShareTransferError` says so). Do not move share transfers into `transfer.Service` in this item.

### Fix

1. After the `TransferID` check, require `TransferAccountID.Valid`. If it is not, return an error. Write nothing.
2. Require that the argument is the source leg: `TotalAmount` is negative, the same rule as `reverseTransferShares`. If it is not, return an error that names the source account (the leg's `TransferAccountID`). Write nothing. A zero-basis leg has no sign. Refuse it with the same error. See "Recorded, not scheduled".
3. The TUI checks the leg before it opens the edit dialog. For a destination leg it shows a notification: edit this transfer from the source account, with the account name. It does not open the dialog. Keep the service check.
4. Load the other leg with `ListByTransferID`. Require exactly one other row, and require its account id to equal `TransferAccountID`. If the row is missing, return an error. Write nothing. Do not reverse the source.
5. Run the closed-account check on the old source, the old destination, the new source, and the new destination. A missing destination is an error, not a skipped check.
6. Heal the old accounts and the new accounts before reverse. Healing is its own committed transaction today (`healInOwnTx`). Keep that. Do the reverse, both deletes, and `TransferShares` in the existing `runInTx`. If `TransferShares` fails, the transaction rolls the reverse back.
7. Heal every account/security pair: (old source, old security), (old destination, old security), (new source, new security), (new destination, new security). Skip duplicates.

### Tests

Add them next to `TestInvestmentService_UpdateTransferShares_RejectsClosedOldDestination` (`internal/investment/closed_account_guard_test.go`).

| Case | Assert |
| --- | --- |
| Happy edit, same accounts | Both old row ids are gone. One new pair exists. Share counts match the edit. |
| Other leg deleted before the edit | Error. Source row still exists. Source position is unchanged. |
| `TransferID` set, `TransferAccountID` not set | Error. No new pair. |
| Edit that moves the destination account | Old destination position is reversed. New destination holds the shares. Old destination does not. |
| Edit started from the destination leg | Error names the source account. Both legs and both positions are unchanged. |
| TUI edit key on a destination leg | Notification shown. No dialog opens. |

The fault style in `internal/investment/investment_service_tx_test.go` is the pattern for "a failed write leaves no partial state". Use it if the happy-path test cannot see a mid-function failure. The missing-leg case is the one that matters. It must not depend on a fault injector.

### Done when

- A missing other leg returns an error and leaves both ledgers unchanged.
- An edit from the destination leg returns an error and leaves both ledgers unchanged.
- The closed-destination test still passes.
- No production path lists every row of an account to find one transfer leg.

### Do not

- Do not change `transfer.Service.Update`. That path already rewrites cash legs in place and keeps `transfer_id`.
- Do not add account-to-account moves to the cash editor. `transfer.Edit` omits accounts on purpose.
- Do not add a "From" field to the share-transfer dialog. Editing from both legs is a separate feature.

### As built (PR #50)

Where the build departs from the fix list above:

1. A zero-basis leg gets its own reason (`BrokenShareTransferError`: no cost basis, so the direction is unknown), not the destination-leg error of step 2. Both legs of such a transfer have no sign, so "edit it from the source account" would send the user in a circle.
2. `DeleteTransaction` (`internal/investment/delete.go`) had the same scan: it listed the named account's rows to find the paired leg. It now uses `ListByTransferID`, and it checks each other leg's own account for the freeze rule, because the pointer on the row can be missing or wrong. A leg with no pair still deletes, so a broken transfer can be cleaned up. The "Done when" line (no production path scans an account for a leg) required this.
3. A `transfer_cash` row is refused as "not a share transfer". The old check looked only at `TransferID`.
4. The tests are in `internal/investment/update_transfer_shares_test.go`, not in `closed_account_guard_test.go`, which is about the freeze rule.
5. The comment on `transfer.Edit` (`internal/transfer/write.go`) no longer says that this function gets the account check wrong.

## W5 — One balance for display, close, and delete

**This is the main problem. It ships as four items: W5a, W5b, W5c, and W5d.** This section holds the shared problem and constraint. Each slice below has its own fix, tests, and done line.

### Problem

`account_balances` (recreated in `internal/db/migrations/034_account_type_hsa_investment.sql`) adds only register rows:

```sql
a.opening_balance + SUM(register amount where status != 'void')
```

`account.Service.GetBalance` and `GetAllBalances` (`internal/account/account_service.go`) read that view and nothing else.

Callers that treat the result as "what this account is worth", and the callers that enforce a rule:

| Caller | File | Slice |
| --- | --- | --- |
| Close rule | `account.Service.Close` | W5a |
| `tmoney account close` | `internal/cli/account/close.go` via `Account.Close` | W5a |
| TUI close dialog | `internal/tui/close_account_dialog.go`, through `undo.NewCloseAccountCommand` (`internal/undo/account.go`) | W5a |
| Delete rule | `account.Repository.Delete` | W5b |
| `tmoney account delete` | `internal/cli/account/delete.go` | W5b (refusal), W5c (preview figure) |
| TUI delete | `internal/tui/account_dialog.go`, through `undo.NewDeleteAccountCommand` | W5b |
| `tmoney account list` | `internal/cli/account/list.go` | W5c |
| `tmoney account show` | `internal/cli/account/show.go` | W5c |
| `tmoney account balance` | `internal/cli/account/balance.go`, printed by `printBalancesTable` | W5c |
| Sidebar balance map | `internal/tui/app_sidebar.go` (loaded, never read) | W5c removes it |
| Net-worth report | `report.Service.netWorthAsOf` | W5d |
| TUI dashboard and Reports view | `internal/tui/dashboard_view.go`, `internal/tui/reports_view.go` | W5d |
| `tmoney report net-worth` | `internal/cli/report/format.go` (hard-codes `"USD"` for the three totals) | W5d |

These callers stay on the register balance, and no slice changes them:

| Caller | Why it stays |
| --- | --- |
| Loan amortization when no schedule exists (`internal/tui/amortization_view.go`, `GetBalance`, then negate) | A loan is not an investment account. |
| Loan projection (`internal/cli/loan/projection.go`) | Same. |
| Register header (`internal/tui/register_view.go`, `loadRegisterData`) | The sidebar sends an investment account to the investment register (`app_sidebar.go`). This view sees register accounts only. |

`Close` treats a zero register balance as an empty account. An investment account with cash or shares has a zero register balance. Close succeeds. The account freezes. Sells and withdrawals are then refused by the closed-account guards. The user must reopen the account to move the shares and the cash.

`latestTransactionDate` reads `MAX(date)` from `transactions` only. The close date can be earlier than the last investment row.

`Repository.Delete` counts `transactions` and scheduled rows. It does not count `investment_transactions`, lots, or positions. `CountLedgerRows(id, true)` already counts those three, and the type-change path uses it. Delete does not. `investment_transactions.account_id`, `investment_lots.account_id`, `investment_positions.account_id`, and `reconciliation_sessions.account_id` all reference `accounts(id)`. A delete that passes the Go check then fails inside the database, and the user sees a driver error. The preview shows a zero balance and no warning.

`printBalancesTable` (`internal/cli/account/format.go`) adds every account into one total and formats that total as USD. An EUR account and a USD account are summed. The label is still USD. `report.Service.netWorthAsOf` has the same add, with no currency check.

The screen net-worth report does call the valuer for investment accounts (`internal/report/report_service.go`). On error it keeps the register balance and sets no flag. The comment says "fall through". The number is then the opening balance, and the screen does not say the valuation failed.

The as-of investment value is not historical. `GetAccountValuation(id, asOf)` reads the cash with `cashBalanceOf` (`internal/investment/queries.go`), which ignores `asOf`. The shares are the current shares. Only the price is as of the date. The comment on `AccountValuation` (`valuation.go`) confirms that `TotalValue` prices today's positions.

### Constraint

`investment` imports `account`. `account.Service` must not import `investment`. A balance method on `account.Service` that calls `ValuationService` is an import cycle.

`report.Service` already holds an `InvestmentValuer` port (`GetAccountValuation`). That port is the seam for display. Do not give `account.Service` a valuer. The close rule does not need a price. It gets its own narrow port (W5a).

Do not add investment rows to the `account_balances` view. A buy is not a bank deposit. The view is the register balance. Keep it.

### Do not (all slices)

- Do not build investment reconciliation here. That is W6, and W6 only refuses it.
- Do not add a currency-exchange table. Do not invent an exchange rate.
- Do not change the valuation math.

## W5a — Close refuses an investment account that is not empty

**Status:** done in PR #51. `ValuationService.LedgerState` implements the port. With no port wired, `Close` returns `ErrNoInvestmentLedger`. Added in review: register rows can still land on an investment account (`transaction add`, import, a posted schedule), so `Close` also refuses while their non-void total, without the opening balance, is not zero (`HasBalanceError.RegisterRows`).
**Decision:** The close rule stays on `account.Service.Close`, with a narrow port. The CLI, the TUI, and undo all go through that one door. The port returns cash and "has holdings" only. It does not return a price.

### Fix

1. In `internal/account`, define a port and an option. `account.NewService` takes options.

   ```go
   type InvestmentLedger interface {
       LedgerState(id types.ID) (cash types.Money, hasHoldings bool, err error)
   }
   func WithInvestmentLedger(l InvestmentLedger) ServiceOption
   ```

   `security.WithLotChecker` and `security.WithPositionChecker` are the pattern.
2. Implement the port on the investment side. `cash` must come from the same code that feeds `AccountValuation.CashBalance` (`cashBalanceOf`), so close and display agree. `hasHoldings` is true when any open lot (lot-tracked account) or position (not lot-tracked) has a non-zero share count. A missing price does not matter here.
3. `internal/app` wires it. Build the investment read side before `account.Service` so that no setter is needed.
4. `Close`, for an investment account:
   - If no port is wired, return an error. Do not use the register balance. The rule fails closed.
   - If `cash` is not zero or `hasHoldings` is true, return `HasBalanceError`. Add `HoldsShares bool` to it. `Balance` is the cash.
   - The error text names the cause: the account holds shares, the account has cash of an amount, or both.
5. `Close`, for a register account: no change.
6. `latestTransactionDate` is the later of `MAX(date)` on `transactions` and `MAX(date)` on `investment_transactions`.
7. The CLI close message (`internal/cli/account/close.go`) and the TUI dialog message (`closeAccountErrorMessage`, `internal/tui/close_account_dialog.go`) name the cause from the error. Do not print "the account balance must be zero" for an account that holds shares.

### Tests

Put the service tests in `internal/account` (with a fake port) and in `internal/app` (with the real wiring). Put the CLI assertions next to `internal/cli/account/close_test.go`.

| Case | Assert |
| --- | --- |
| Brokerage with shares, cash zero, price present | `HasBalanceError`, `HoldsShares` true. |
| Brokerage with shares and no price | `HasBalanceError`, `HoldsShares` true. No price is needed. |
| Brokerage with cash, no shares | `HasBalanceError`, `Balance` is the cash. |
| Empty brokerage (no cash, no shares) | Close succeeds. |
| Close date before the last investment row | `InvalidCloseDateError`. |
| Investment account, no port wired | Error. The account stays open. |
| `undo.CloseAccountCommand` on a brokerage with shares | Error. This proves that the TUI path uses the same door. |
| Checking account with zero balance | Close succeeds, as today. |

### Done when

- A brokerage with shares cannot be closed from the CLI, the TUI, or redo. The output says why.
- `account_balances` is unchanged.

## W5b — Delete counts the ledger of the account type

**Status:** done in PR #52. See "As built" at the end of this item.
**Decision:** An active reconciliation refuses the delete. Completed sessions are removed with the account. An investment account reports `investment transactions`.

### Problem

See the W5 problem. Two more facts:

- `CancelReconciliation` deletes the session row. Thus only two kinds of session can stay: an active one, or a completed one.
- No command or screen removes a completed session. `reconciliation.Repository.DeleteByAccountID` exists, but only tests call it. If Delete refused every session, an empty account with one old completed reconcile could never be deleted.

### Fix

1. Register account: keep the current `transactions` and scheduled checks.
2. Investment account: refuse when the investment ledger has rows (`CountLedgerRows(id, true)`). The dependent name is `investment transactions`. The count is the number of `investment_transactions` rows only. `CountLedgerRows` adds lots and positions to that number, so one buy would show as three. If no transaction row exists but a lot or a position does, refuse with the dependent name `investment holdings` and that count.
3. Both types: keep the scheduled-transaction check.
4. Reconciliation sessions:
   - An active session refuses the delete. The dependent name is `active reconciliation`. The CLI says: finish it (`tmoney reconcile finish`) or cancel it in the TUI.
   - Completed sessions only: delete them and the account in one database transaction (`db.DB.WithTx`).
5. Undo of a delete recreates the account. It does not recreate the removed completed sessions. Say this in the comment on `DeleteAccountCommand`.
6. The CLI "close it instead" sentence (`internal/cli/account/delete.go`) applies to `transactions` and `investment transactions`.
7. The CLI preview warns when the ledger or an active session blocks the delete, the same way it warns about scheduled references today.
8. Remove `reconciliation.Repository.DeleteByAccountID` and its tests. It is dead.

### Tests

| Case | Assert |
| --- | --- |
| Delete of a brokerage that has a buy | `HasDependentsError`, `investment transactions`, count 1. The account row remains. No driver error. |
| Delete of an empty brokerage | The account row is gone. |
| Register account, no transactions, one completed session | The account row and the session are gone. |
| Account with an active session | `HasDependentsError`, `active reconciliation`. The account and the session remain. |
| CLI delete of a brokerage that has a buy | Output says "close it instead". |

### Done when

- No account delete, from the CLI or the TUI, can end in a foreign-key driver error.

### As built (PR #52)

1. `Repository.DeleteBlocker` checks both ledgers for every account type, not only the ledger of the type. Eight foreign keys point at `accounts`, including `transfer_account_id` on both ledgers. A new dependent name, `transfer references`, covers rows in other accounts that name this account, such as the surviving leg of a half-deleted transfer. Without it, the "Done when" line does not hold.
2. The completed sessions and the account are deleted in two commits, not in one transaction (fix step 4). DuckDB checks a foreign key against rows deleted earlier in the same transaction, so deleting the sessions and then the account in one transaction always fails. Each step checks the blockers first, so a refused delete writes nothing. If the second step fails, the account stays and its completed sessions are gone.
3. `account.Service.DeleteBlocker` gives the CLI preview the same check that the delete uses.

## W6 — Refuse reconcile on an investment account

**Status:** done in PR #53. The service error is `InvestmentAccountError`, the TUI guard is `refuseInvestmentReconcile`, which the menu calls before its closed check and the register `r` key reaches through `showStartReconciliationDialog`, and the migration is 036.
**Decision:** A migration deletes the reconciliation sessions that already exist on investment accounts.

### Problem

The Reconcile menu action (`internal/tui/app_menu.go`, `MenuActionReconcileAccount`) runs for any selected account that is not closed. `reconciliation.Service.StartReconciliation` checks that the account exists and is open. It does not check the type.

`GetCandidateTransactions` and `CalculateClearedTotal` (`internal/reconciliation/reconciliation_service.go`) read only `transactions`. Investment cash is invisible. A user can finish a reconcile against a brokerage statement and mark nothing, or mark an empty register, and believe the account matches the statement.

The investment ledger also has no `void` status (`transfer.StatusFromRegular` returns `UnrepresentableStatusError` for void). A reconcile design for brokerage cash is a separate project. This item only stops the wrong one.

`StartReconciliation` replaces an active session. The only cancel path is inside the Reconciliation view (`CancelReconciliation`, called from `internal/tui/reconciliation_view.go`). The CLI has no `reconcile cancel`. After the new guard, an active session that a user started on a brokerage before this item has no path to cancel. Under the W5b rule, that session also blocks a delete. A session on an investment account never matched anything, because the ledger-change guard keeps the register ledger of that account empty.

### Fix

1. At the start of `StartReconciliation`, if `acct.Type.IsInvestmentType()`, return a sentinel error. Text: the account is an investment account, and reconcile applies to the register ledger only.
2. The TUI menu path shows that error as a notification and does not open the statement dialog. Check the type before `showStartReconciliationDialog`, so the user does not fill in a balance that will be rejected. Keep the service check.
3. The CLI entry is `tmoney reconcile start` (`internal/cli/reconcile/start.go`). It calls the service, so it gets the same error. Add a CLI test.
4. Add a numbered migration that deletes every `reconciliation_sessions` row whose account type is `investment` or `hsa_investment`.
5. Test: start reconcile on an investment account, expect the sentinel. Start reconcile on a checking account, still succeed. Migration test: a session on a brokerage is gone, a session on a checking account stays.

### Done when

- The menu does not open the dialog for a brokerage account.
- The service test fails if the type check is removed.
- No reconciliation session remains on an investment account after the migration.

### Do not

- Do not teach `CalculateClearedTotal` to add investment cash in this item.
- Do not add a `void` status to `investment_transactions`.

## W5c — One display figure for account list, show, and balance

**Status:** done in PR #54. As built: `AccountFigure` also carries the account `Type`, which `TotalsByCurrency` needs to split assets from liabilities; `AccountFigures` takes the account list the command already has; `account show` prints its rows before it returns a valuation error, as the list commands do; the delete preview shows a valuation error in its line and does not fail. Removing `GetAllBalances` also removed a test in `tests/integration`. Added in review: `portfolio_holdings` lists active accounts only, so a closed account's holdings are read from its own lots or positions; before that, a closed brokerage holding shares was valued at its cash alone.
**Decision:** The display figure lives on `report.Service`. In a list, a row that cannot be valued shows "error". Its currency has no total, and the command exits non-zero.

### Fix

1. Extend `report.ValuationResult` with `CashBalance`. The adapter in `internal/app/registry.go` fills it from `AccountValuation.CashBalance`.
2. Add `AccountFigure(id)` and `AccountFigures(...)` to `report.Service`. The result shape:

   ```text
   AccountID   types.ID
   Displayed   types.Money   // register balance, or investment TotalValue
   Cash        types.Money   // register balance, or investment CashBalance
   Currency    string
   Estimated   bool          // true when a holding has no price
   Err         error         // set when the valuation failed; the money fields are then not valid
   ```

3. A register account: `Displayed` and `Cash` are the register balance from `GetBalance`.
4. An investment account: `Displayed` is `TotalValue`. `Cash` is `CashBalance`. `Estimated` is `HasMissingPrices`. A valuation error goes into `Err` (in `AccountFigures`) or is returned (in `AccountFigure`). Never substitute the register balance.
5. Add one grouping helper in `internal/report` that makes one total per currency from a set of figures. A currency that has a row with `Err` has no total ("not available"). W5d uses the same helper.
6. `account list`, `account show`, and `account balance` print `Displayed`. When `Estimated` is true, prefix the figure with `~`, as the dashboard does. A row with `Err` prints "error" in the balance column. The error text goes to stderr, and the command exits non-zero after it prints every row.
7. `account balance` prints one total per currency with that currency code. It does not add across currencies. It does not label a total USD unless the currency is USD.
8. `account show` for an investment account prints "Cash" and "Total Value". It does not print "Cleared Balance". The investment ledger has no cleared state.
9. The CLI delete preview prints `Displayed`.
10. Remove the sidebar balance load (`app_sidebar.go`). Then remove `GetAllBalances` if only tests call it. Check the service layer and the repository layer.

### Tests

Put the service tests in `internal/report`, next to `report_service_test.go`, or in `internal/app` next to `networth_investment_test.go`. Put the CLI assertions next to `internal/cli/account/balance_test.go`.

| Case | Assert |
| --- | --- |
| Brokerage with a deposit and no shares | `account show` and `account balance` print the cash. `GetBalance` may still return 0. The new function must not. |
| Brokerage with shares and no price | `Estimated` is true. `Displayed` is not zero. The CLI prints `~`. |
| USD checking plus EUR savings | Two totals. Neither is a sum of both. The string `USD` is not the label of the EUR total. |
| Valuer error on one brokerage in `account list` | Every row prints. The brokerage row prints "error". The USD total is "not available". Exit status is non-zero. The opening balance is not shown. |

### Done when

- The CLI callers in the W5 table show the new figure.
- The sidebar no longer loads a balance map.
- `go test ./internal/account ./internal/report ./internal/cli/account ./internal/app` passes.

## W5d — Net worth by currency, with row errors

**Status:** done in PR #55. As built: `CurrencyTotal` also has `AssetsAvailable` and `LiabilitiesAvailable`, so a failed brokerage (an asset) does not hide a correct liabilities total; the as-of note (`NetWorth.InvestmentAsOfApproximate`) is printed by the CLI only, because the TUI Reports view always shows today; the valuer runs after the register query's rows are closed; `report net-worth` exits non-zero after printing a failed row, as the W5c commands do; money in the CLI and the TUI goes through one formatter, `types.Money.Format` (decimal rounding, half away from zero; `$`, `€`, `£`, else the currency code), added in review after the TUI's own `%.2f` path printed some amounts a cent low; in review the dashboard also got per-currency totals on shared rows, the account currency on its total-return and holdings lines, `~` on an estimated assets total, and name widths measured from the amount.
**Needs:** W5c.
**Decision:** The net-worth report uses the same rule as the list. A failed row carries its error. Only the total of that currency is "not available".

### Fix

1. `report.NetWorth`: remove `TotalAssets`, `TotalLiabilities`, and `NetWorth`. Add `Totals []CurrencyTotal`, sorted by currency code. Each `CurrencyTotal` has `Currency`, `Assets`, `Liabilities`, `NetWorth`, `Available`, and `Estimated`. When the old fields are gone, the compiler finds every caller that adds across currencies.
2. `report.AccountBalance` gets `Currency` and `Err`.
3. `netWorthAsOf` reads `a.currency`. An investment account uses the valuer. A valuer error goes into the row's `Err`, and that currency's total is not available. Delete the fall-through. Use the W5c grouping helper.
4. Callers: the TUI dashboard (`dashboard_view.go`), the TUI Reports view (`reports_view.go`), and the CLI (`internal/cli/report/format.go`, where the three totals hard-code `"USD"`). Print one line per currency, with the currency code. Print "not available" when `Available` is false. With one currency, the output looks as it does today, plus the real currency code.
5. As-of subtitle: when the as-of date is before today and the report has an investment account, the CLI and the TUI Reports view say: investment accounts show current cash and shares, priced as of the date. Do not change the valuation math.

### Tests

| Case | Assert |
| --- | --- |
| USD checking plus EUR savings | Two `CurrencyTotal` entries. Neither is a sum of both. |
| Valuer returns an error for one USD brokerage | The row has `Err`. The USD total is not available. An EUR total still shows. The opening balance is not shown. |
| As-of date one year ago, with a brokerage | The subtitle is present. |
| As-of date today | No subtitle. |

### Done when

- No caller adds money across currencies.
- The dashboard shows the other rows when one brokerage fails.

## W11 — As-of net worth leaves out accounts not yet open

**Status:** done in PR #56. The filter is `opening_date <= asOf` in the base query, so it holds with `--include-closed` too.
**Needs:** W5d.

### Problem

The `netWorthAsOf` query (`internal/report/report_service.go`) filters closed accounts. It has no `opening_date` filter. Net worth as of 2020 includes an account opened in 2025, with its full opening balance.

### Fix

1. Leave out an account when the as-of date is before its `opening_date`. Apply the rule to register and investment accounts.
2. Keep the closed-account rule as it is.

### Tests

| Case | Assert |
| --- | --- |
| Account opened after the as-of date | Not in the report. Not in any total. |
| Account opened on the as-of date | In the report. |
| Investment account opened after the as-of date | Not in the report. The valuer is not called for it. |

## W7 — Constructor must not write

**Status:** done in PR #57. As built: `clitest.OpenSvc` also runs `Prepare`, as a real CLI open does; a test fixture, `clitest.DamagedHealFile`, forces a real heal failure (a lot with more shares consumed than it held), so no test hook was added. The TUI alert logs the full error to the app log. Added in review: the alert is a sticky status-bar slot (`StatusBar.SetSticky`), because the due-count refresh after every open clears the queued notifications; it lasts while that file is open.
**Decision:** `Prepare` runs all five steps and returns every failure. The CLI and the TUI show the failure loudly and continue.

### Problem

`app.NewServices` (`internal/app/registry.go`) builds the graph and then writes:

| Call | Error handling |
| --- | --- |
| `categorySvc.EnsurePaycheckCategories()` | ignored |
| `categorySvc.EnsureValueAdjustmentCategory()` | the collision flag is kept; the error is ignored |
| `investmentSvc.HealAllAccounts()` | ignored |
| `scheduledSvc.HealNextDates()` | ignored |
| `scheduledSvc.HealTransferCategories()` | ignored |

`tmoney account list` opens services and therefore writes. A failed heal leaves positions wrong. The user sees a normal list.

`HealAllAccounts` (`internal/investment/rebuild.go`) also swallows the error of each account (`// Skip the account; don't break startup.`). It returns an error only when the account list fails. A `Prepare` that only forwards the return value would almost never see the failure this item is about.

`cmdutil.OpenServices` (`internal/cli/cmdutil/services.go`) also runs `Scheduled.AutoPost` and ignores its error.

A fatal `Prepare` error would stop every CLI command, including `tmoney investment rebuild-positions` (the repair tool), backup, and export. Writes do not depend on the startup heal. Each share operation heals its own account first (`healInOwnTx`, `syncPositionAndLots`) and fails loudly if that heal fails. The startup heal protects reads.

The writes exist for a reason. Old files need paycheck categories, the Value Adjustment category, position repair, schedule-date repair, and removal of categories on transfer schedules that cannot store one. Keep the writes. Move them out of the constructor. Surface the errors.

`NewServices` is called from production in `internal/cli/cmdutil/services.go` and `internal/tui/app.go` (`newTUIServices`, which both `NewApp` and `switchDatabase` use), and from many tests. Tests that create categories by calling `NewServices` depend on the seed. Find them before changing the constructor (`EnsurePaycheck` and `ValueAdjustment` tests construct their own service; other tests may only see the categories because `NewServices` seeded them).

### Fix

1. `NewServices` only wires fields. It does not call Ensure or Heal.
2. Add `(*Services).Prepare() error` in `internal/app`. It runs the five calls in the same order as today. `HealTransferCategories` stays after `SetTransferPort`.
3. `Prepare` runs all five steps, even after a failure, and returns `errors.Join` of every failure. Say that in the comment. The steps do not depend on each other's data. A joined error hides nothing, and the other repairs still run.
4. `HealAllAccounts` continues to the next account after a failure. It returns a joined error that names each failed account. It still returns the count of healed accounts.
5. `cmdutil.OpenServices` calls `Prepare`. On error it prints `warning: startup repair failed:` and the error to stderr, and the command continues. An `AutoPost` error gets the same treatment.
6. The TUI calls `Prepare` in both open paths (`NewApp` and `switchDatabase`). On error it shows an alert notification, not a timed toast.
7. Keep `ValueAdjustmentUserCollision` on `Services`. `Prepare` sets it. The TUI notice stays.
8. Update tests that opened a database only through `NewServices` and then expected seeded categories. Those tests call `Prepare`, or they call `EnsurePaycheckCategories` themselves. Do not weaken an assertion to make `NewServices` pass.

### Done when

- A unit test constructs `NewServices` on an empty database and finds no paycheck category until `Prepare`.
- A forced failure of one account in `HealAllAccounts` (a stub, if the real heal cannot fail on a small file) is in the error that `Prepare` returns, and the other accounts still heal.
- `OpenServices` prints that error to stderr, and the command still runs.
- `tmoney investment rebuild-positions` runs when the startup heal fails.

### Do not

- Do not remove the heals.
- Do not start W9 (repository fields) in this change. `Prepare` uses the services, not the exported repos. Leave the struct fields alone.

## W10 — Correct `docs/ARCHITECTURE.md`

**Status:** done in PR #58. Correction: the problem list above says `investment` depends on "the transfer owner". It does not; `transfer` imports `investment`. The document shows the real direction. Left for later items: the TUI views table (W8) and the `AccountRepo` example (W9).
**Needs:** W5c. Ship it as one pull request after W5c.

### Problem

`docs/ARCHITECTURE.md` describes a program this tree no longer is.

- The package tree omits `internal/transfer`, `internal/transferlink`, `internal/loan`, and `internal/applog`.
- The feature-slice table says `investment` depends only on `account`. It also depends on prices, corporate actions, and the transfer owner outside the package.
- The "Transfer Between Accounts" diagram says the path is `transaction.Service.CreateTransfer` and `transaction.TransferRepository`. Cash transfers go through `transfer.Service`.
- The diagram says balances are recalculated and stored. Balances are computed on read.
- `account_balances` is described as the current balance per account. It is the register balance only. This document must not claim the view is the investment value.

A change made from this document will put transfer code back on `transaction.Service`.

### Fix

1. Update the tree, the slice table, and the transfer diagram to match `internal/transfer` and `db.WithTx`.
2. Call `account_balances` the register balance. Point investment value at `ValuationService`.
3. Name `report.Service.AccountFigure` / `AccountFigures` as the functions that the CLI must call for display. Name the `account.InvestmentLedger` port as the close rule for an investment account.
4. Do not rewrite the whole document. Fix the sections that are false.

### Done when

- A reader who has not seen the code can name `transfer.Service` as the cash-transfer owner.
- The document does not mention `TransferRepository` or `CreateTransfer` as the current path.

## W12 — Corporate Actions keys must reach the view

**Status:** done in PR #60. As built: the list arm of the view handler and `closeCorporateActionView`, which nothing else called, are removed, so the global Esc arm is the one back path; `switchView` ends a filter entry when it leaves the view, so the view never comes back still capturing keys; the help Esc line reads "Close details or filter entry, else back".
**Source:** Review of PR #46 (W1), 2026-09-27.

### Problem

`handleKeyPress` (`internal/tui/app.go`) matches the global keys before it calls the view handler. The Corporate Actions view has no exception. Two things break:

1. **Filter typing.** After `/`, `handleCorporateActionViewKeys` (`internal/tui/corporate_action_history.go`) adds each typed character to `corporateActionViewFilter`. But the global keys run first: `1` to `5` change the view, `?` opens help, and Esc leaves the view. A user cannot type a digit into the filter. The investment register has the guard that this view lacks: while `investmentRegister.filterSearching` is true, every key goes to `handleInvestmentRegisterKeys`.
2. **Esc.** The global Esc arm makes two exceptions: Prices detail mode, and an active investment-register filter. Corporate Actions is not one of them. Thus Esc always calls `switchView(previousView)`. It does not close the details panel, and it does not end filter entry. The three Esc arms in `handleCorporateActionViewKeys` never run.

W1 made the help line say "Back", because that is what Esc does today (`corporateActionShortcuts`).

### Fix

1. Add an early guard in `handleKeyPress`, next to the investment-register guard: while `currentView == ViewCorporateActions` and `corporateActions.filterEditing` is true, send every key to `handleCorporateActionViewKeys`.
2. In the global Esc arm, add an exception: on Corporate Actions with the details panel open (`corporateActions.detail != nil`), send Esc to the view handler. The handler closes the panel, and the view stays.
3. The Esc arm in the view handler for the list (`closeCorporateActionView`, then `switchView(previousView)`) must give the same result as the global arm. The global arm also returns `reloadCurrentView()`. Keep one path. Do not have two different "back" behaviors.
4. Change the help Esc line to match the new behavior (close details, end filter entry, or back), and update `TestCorporateActionShortcuts`.
5. The view-layer plan says that three pre-switch branches in `handleKeyPress` do not move. The new guard is a fourth. Update that rule in `specs/implementation-plan-tui-view-layer.md`.

### Tests

| Case | Assert |
| --- | --- |
| Type `/`, then `1`, `2`, `?` | The filter text is `12?`. The view is still Corporate Actions. No help overlay. |
| Esc while typing the filter | Filter entry ends. The view is still Corporate Actions. |
| Esc with the details panel open | The panel closes. The view is still Corporate Actions. |
| Esc on the list | The view goes back, as today. |

### Do not

- Do not change key routing for another view.
- Do not change the keys themselves.

## W8 — One view table in the TUI

**Status:** done. Phase 3 (the two file splits) is done in PR #59, and phase 1 (the view table) in PR #61. The plan and the design record where the build differs. Phase 1 found one old bug and left it alone, because the phase changes no behavior: the Prices detail hint never shows. W13 fixes it.
**Needs:** W1, W2, and W12. The table copies the fixed key routing and help text.
**Decision:** The queue for this work is `specs/implementation-plan-tui-view-layer.md`. W8 is phases 1 and 3 of that plan (VL-101 to VL-111, and VL-201 to VL-209). Phases 2 and 4 stay only in the plan. Mark progress in the plan, not here. Change this status line when both phases are done.

### Problem

`*App` has 420 methods (count of `func (a *App)` under `internal/tui` on 2026-09-27). The list of views is written out by hand in:

- `internal/tui/app.go` (`View.String`, and the key switch)
- `internal/tui/app_view.go` (content and status bar)
- `internal/tui/app_menu.go` (`switchView`)
- `internal/tui/app_helpers.go` (reload, refresh, active table)
- `internal/tui/help_overlay.go` (W1)

W1 and W2 are missing arms in two of those lists. The next view will miss an arm the same way.

Two files are still too large to navigate:

| File | Lines |
| --- | --- |
| `internal/tui/price_view.go` | 1231 |
| `internal/tui/investment_register_view.go` | 1073 |

### Corrections that the plan already carries

1. Phase 0 of the plan is W1. Do not do it again.
2. VL-108 is corrected for W2: the Reconciliation and Corporate Actions entries call the loaders that W2 added. No entry has a nil `reload`.
3. Do not make a view a `tea.Model`. The design explains why (Bubble Tea v2 `Update` replaces the root model).
4. Do not create a new package. The design explains why (tests construct `App` literals).
5. The file splits (phase 3) can land before the view table. They do not need W1.

### Done when

The exit criteria of plan phases 1 and 3 are met. `price_view.go` and `investment_register_view.go` are each at or under the line counts in that plan (about 450 and about 500, split across the named files).

### Do not

- Do not move dialog `open` / `submit` / `close` off `App` for every view. The design prices that and does not require it.
- Do not start from `specs/code-quality-review.md` item 4. That item mixes four jobs. The design splits them.

## W13 — The Prices detail hint must show

**Status:** done in PR #62. As built: the refresh is `refreshKeyHints` (`internal/tui/app_helpers.go`), deferred at the top of `Update`; it skips an App with no status bar, as some tests build. A pty smoke run differs from the build before only in the status line of the Prices history.
**Source:** The smoke test of W8 phase 1 (`specs/implementation-plan-tui-view-layer.md`, VL-111), 2026-09-28. The bug is older than W8: `main` before W8 has it too.

### Problem

The Prices view has two modes, the list and one security's history, and each has its own status-bar key hints. The history hints name the keys that work only there: `enter edit`, `n new`, `d delete`, `i import`. They never show.

`updateStatusBar` (`internal/tui/app_helpers.go`) sets the hints. Only a view switch, a database switch, and the start of the app call it. The mode changes without a view switch, in three places: Enter on the list, Esc in the history, and a load that arrives in history mode (the drill from Securities opens Prices first and loads the history after). So the status bar keeps the hints of the mode that was on screen at the last view switch, which is almost always the list.

### Fix

`Update` refreshes the key hints after every message, with a deferred call at its top. The hints then follow any state that they read, in every path, now and for a later view with modes. `updateStatusBar` stays the one place that sets the context (the view name and the file), which changes only with the view or the file.

### Tests

| Case | Assert |
| --- | --- |
| The list loads | The hints contain `enter view history`. |
| Enter on the list | The mode is history. The hints contain `i import`. |
| Esc in the history | The mode is list. The hints contain `enter view history`. |
| A load arrives in history mode | The hints contain `i import`. |

### Do not

- Do not change the hint text.
- Do not refresh the hints in the render path. `View` must not write state that it does not need for the mouse.

## W14 — Each investment load guards the service it calls

**Status:** done.
**Source:** VL-405 in `specs/implementation-plan-tui-view-layer.md` (the Portfolio move), 2026-10-02. The bug is older: it came with the extraction of the valuation service (`ba24049`), when some nil checks were not updated.

### Problem

`app.Services` has two investment fields: `Investment`, the write side, and `InvestmentValuation`, the read model. Four loads checked one and called the other:

- `loadDashboardData` checked `Investment` and called only `InvestmentValuation`.
- `loadInvestmentRegisterData` checked `Investment` and called both, with no guard on the valuation call.
- The Portfolio view's `load` and `loadLotDetail` checked the investment service and called only the valuation service.

Production always wires both services, so a user cannot hit it. But an `App` with only the investment service wired panicked in all four loads, and an `App` with only the valuation service skipped work it could do. Tests build both kinds of `App`.

### Fix

Each load checks the service it calls. The investment register's load has two blocks, the cash balance under `Investment` and the valuation under `InvestmentValuation`, as its other loads are already split. The Portfolio deps lose `investments`, which existed only for the wrong check. The lot-detail error now names the valuation service.

The other 17 nil checks of the two services in `internal/tui` guard the service they call, and were left alone. The CLI does not nil-check them.

### Tests

`internal/tui/investment_services_nil_test.go`, against one lot-tracking investment account:

| Case | Assert |
| --- | --- |
| Each of the four loads, with no valuation service | No panic. |
| Each of the four loads, with no investment service | The valuation (or the lot detail) still loads. |

All eight cases fail on the code before the fix.

## W9 — Stop exporting repositories from `app.Services`

**Status:** done in PR #63. As built:

- The first commit removed five fields, not six: `SplitRepo` had one reader, `export`, so it moved in its own commit. Export now takes the account, transaction, payee, and category services; its split provider's method is `GetSplits`, the name `transaction.Service` already has.
- New pass-throughs with no rule of their own: `transaction.Service.Search` (the CLI search; the dead-code phase of `specs/design-service-decomposition.md` had removed it when it had no caller), and `ValuationService.GetTransaction`, `ListTransactions`, and `ListOpenLots`. The import store is `imexport.NewServiceTransactionStore`, over the transaction and payee services.
- One caller could not move without its rule: the merger preview read `LotRepo` and `PositionRepo` and skipped a position only when its account had open lots. The merger skips a position when its account has ever held the security in lots, and a lot-tracked account that sold out keeps a position row with shares. `CorporateActionService.MergerHoldings` returns the holdings by the merger's rule, and the preview reads it. That fix is its own commit, before the field deletion.
- Tests that must read or write stored state directly build their own repository on the database; `clitest.OpenSvcDB` returns the database next to the services, because a second open of the same file in one process is not safe.
- `TestServices_ExportsNoRepository` fails on a field whose type is a repository, and `TestNewServices` checks the services only.
**Needs:** W5c.
**Decision:** Delete the repository fields. Do not unexport them, and do not add an `apptest` package. A repository holds only the database handle, so a test builds its own with `<pkg>.NewRepository(db)`.

### Problem

`app.Services` exports both services and repositories (`internal/app/registry.go`, the block under "Repositories (exposed for direct use by CLI/TUI when needed)"). A caller can skip a service rule. `TestNewServices` locks the nil-check for those fields, which makes the leak look intentional.

W5c removes the worst caller (`GetBalance` used as investment value). After that, the remaining direct repo reads should move onto service methods, and the fields should go.

Production reads outside `internal/app` on 2026-09-27 (44 in all; about 36 test files use the fields):

| Uses | Repositories |
| --- | --- |
| 0 | `SplitRepo`, `ScheduledTxnRepo`, `ReconciliationRepo`, `PriceRepo`, `TransactionLotRepo`, `CorporateActionRepo` |
| 1 to 4 | `PositionRepo` 1, `SecurityRepo` 2, `TransactionRepo` 4, `LotRepo` 4 |
| 7 to 10 | `PayeeRepo` 7, `InvestmentRepo` 7, `AccountRepo` 9, `CategoryRepo` 10 |

### Fix

1. First commit: delete the six fields that no production code reads. Tests that used them build their own repository from the same `*db.DB`.
2. Then one repository per commit, smallest first. Search production code (not `_test.go`) for `svc.<Name>Repo` and `a.services.<Name>Repo`. For each call, either name the service method that already implements the rule, or add one method. Do not add a method that is a raw `SELECT` with no rule unless the read is truly rule-free (a label lookup by id). Put rule-free reads on the service anyway so the next caller does not grow a new repo use. Then delete the field.
3. Update `TestNewServices` so it no longer treats a public repo as part of the contract.

Stop if a caller cannot move without a new rule you do not understand. Leave that field in place and write the reason under this heading.

### Done when

- `app.Services` has no repository field.
- No production file outside `internal/app` reads a repository off `Services`.
- `go test ./...` passes. This will touch many test fixtures. Do not "fix" them by exporting a new global.

### Do not

- Do not build a unit-of-work framework. `specs/design-service-decomposition.md` §10 records why. That decision stands.
- Do not delete a field in the same commit that changes its behavior.

## Recorded, not scheduled

These are true. They are not work items. Do not "fix" them while you are in a nearby file.

| Fact | Why it stays |
| --- | --- |
| `investment_transactions.status` has no `void` | `transfer.StatusFromRegular` returns an error instead of writing `pending`. A void status is a migration and a product decision. |
| `TransferShares` is one long function | `specs/design-service-decomposition.md` §10 declined to split it. Revisit only if a bug lands inside it. W4 does not. |
| No CI workflow | `.github` contains only `dependabot.yml`. A view-list test (W1) and `go test` on pull requests would lock the list. Add CI only if you are already changing GitHub config. Do not block W1 on it. |
| `*App` method count will not fall in W8 | The view-layer design says the count stays flat. Do not treat a flat count as failure. |
| A zero-basis share transfer has no direction | `reverseTransferShares` and the W4 guard use the sign of `TotalAmount`. A leg with zero cost basis has no sign. `reverseTransferShares` treats it as a destination leg, and W4 refuses to edit it. Spin-off allocation is strictly between 0 and 100 percent, so this case needs a zero-cost buy or opening position. A fix needs a stored direction, which is a migration. |
| As-of investment value is not historical | `cashBalanceOf` ignores the date, and the shares are current. W5d labels this. A true replay of cash and shares as of a date is a separate project. |
| Merger cash-in-lieu is not built | Fractional target shares stay in the account. W3a corrects the comment that said otherwise. The user can sell the fraction to record the cash. |
