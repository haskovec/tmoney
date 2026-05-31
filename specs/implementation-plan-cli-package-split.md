# Implementation Plan: CLI Package Split

This document defines the order in which `internal/cli` (137 files in one
package) is split into per-noun subpackages plus a shared `cmdutil` hub and a
`clitest` test-support package, per [`specs/cli-package-split.md`](cli-package-split.md).
Each item is one PR. Items follow a red-green pattern (relocate tests, get them
green against the new package boundary, preserving coverage 1:1). Mark items
`[x]` as they are finished.

Specs:
- [`specs/cli-package-split.md`](cli-package-split.md) — the design (target layout, hub inventory, format.go distribution, conventions).
- [`specs/cli.md`](cli.md) / [`specs/cli-router.md`](cli-router.md) — current CLI reference + router history. **Unaffected**: this is a pure reorganization; no command, flag, output, or exit code changes.

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

Ordered "most important first" (per request), within the hard dependency
constraint that the hub + harness must exist before any noun can move:

1. **Foundation (Phase 1)** — `cmdutil`, `clitest`, and the exported test
   harness. Blocking: nothing moves until these land. Highest priority by
   definition.
2. **Pilot (Phase 2)** — `account`. Small (4 verbs) but *representative*: it
   exercises the full recipe (printer extraction from format.go, the `NewCmd`
   rename, the domain-import alias, external `_test` + `clitest`). Proving the
   complete pattern once before repeating it.
3. **Highest value + complexity (Phase 3)** — `investment` (13 verbs, biggest
   folder-shrink, the 8-function portfolio-printer cluster), then `security`
   (7), `price` (5). Done early while the pattern is fresh — the deliberate
   reversal of the spec's "investment last."
4. **Standard 4-verb nouns (Phase 4)** — `transaction`, `transfer`,
   `scheduled`, `reconcile`, `db`.
5. **Small / special (Phase 5)** — `report` (2), `theme` (2 + `testdata/` + the
   only non-domain collision, `internal/tui/theme`).
6. **Cleanup (Phase 6)** — delete shims, collapse `format.go`, tidy `root.go`,
   remove `.tdb` cruft.

---

## Design refinements from inventory (supersedes parts of the spec's Test Architecture)

A fan-out inventory + completeness audit (0 orphaned files; 73/73 source,
64/64 test accounted for) surfaced four facts that refine the spec. **These are
authoritative where they differ from `cli-package-split.md`.**

### R1 — Package-name collisions (affects 9 of 11 nouns)

`internal/cli/<noun>` collides with the same-named domain package
`internal/<noun>`. Every moved source **and** test file that imports its own
domain package must alias it. Standard alias convention for this plan:
`<noun>dom` (e.g. `accountdom`, `pricedom`), and `tuitheme` for theme.

| Noun | Domain import | Collides? | Alias |
|---|---|---|---|
| db | `internal/db` | yes | `dbpkg` |
| account | `internal/account` | yes | `accountdom` |
| transaction | `internal/transaction` | yes | `transactiondom` |
| scheduled | `internal/scheduled` | yes | `scheduleddom` |
| security | `internal/security` | yes | `securitydom` |
| price | `internal/price` | yes | `pricedom` |
| investment | `internal/investment` | yes | `investmentdom` |
| report | `internal/report` | yes | `reportdom` |
| theme | `internal/tui/theme` | yes | `tuitheme` |
| transfer | (none named `transfer`) | **no** | — |
| reconcile | `internal/reconciliation` | **no** | — |

> **Alternative considered:** name the CLI packages `<noun>cmd` (e.g.
> `internal/cli/accountcmd`, `package accountcmd`) to avoid all aliasing.
> Trade-off: zero aliases, but folder names diverge from the approved
> `internal/cli/<noun>` shape. This plan keeps `internal/cli/<noun>` + aliasing
> (Go-standard, e.g. `gh`); flip to `<noun>cmd` only if the aliasing churn is
> judged worse than the rename.

### R2 — `clitest` must NOT import `cli`; fixtures and driver are separate

The spec put fixtures **and** the `Exec` driver in `clitest` importing `cli`.
That cycles: the **6 white-box test files** below must stay `package <noun>`
(internal), and an internal test importing a cli-importing `clitest` forms
`<noun>(test) → clitest → cli → <noun>`. Resolution:

- **`internal/cli/clitest`** holds **only fixtures** (`CreateTestDBWithSecurity`,
  `CreateTestDBWithSecurityAndPrices`, `CreateInvestmentTestDB`,
  `CreateCorporateActionTestDB`, `PtrMoney`). It imports only domain packages
  (`db`, `app`, `account`, `security`, `price`, `types`) + `testing`. **Never
  imports `cli`.** Usable by both internal and external tests, cycle-free.
- **The full-root driver is exported from `cli`** as
  `ExecuteWith(args []string, stdout, stderr io.Writer) error` (rename of the
  unexported `executeWith`), plus a launcher stub seam
  `SwapTUILauncher(fn) (restore func())` (replacing the `stubLaunchers`
  test helper that mutates the unexported `tuiLauncher`).
- **External `package <noun>_test`** files (black-box) import `cli` (for
  `ExecuteWith` + `SwapTUILauncher`) and `clitest` (fixtures). Allowed: an
  external test package may import packages that import the package under test.

### R3 — White-box test files that stay `package <noun>` internal

Go allows one internal + one external test package per directory. These files
poke unexported symbols and **cannot** be `package <noun>_test`; isolate the
white-box test functions into an internal file and keep the rest external:

| File | Unexported symbol(s) | Owner noun |
|---|---|---|
| `transfer_resolve_test.go` | `resolveTransferPair`, `resolvedTransfer` fields, `*errTransferLineSplit` | transfer |
| `price_update_test.go` | mutates `registerPriceProviders` hook var | price |
| `wal_test.go` | `walToThemeTOML`, `ReadWalColors` | theme |
| `theme_wal_subcmd_test.go` | `walCachePath` | theme |
| `report_spending_test.go` | `parseYearMonth` (`TestParseYearMonth` only) | report |
| `format_test.go` | `printHelp`, `printVersion` (→ residual), `formatMoney` (→ cmdutil) | residual + cmdutil |

Internal white-box tests do **not** use `ExecuteWith` (cycle). They test the
unexported function directly; where one must run a command (e.g.
`price_update_test`), it builds a **partial** root inline (a throwaway
`*cobra.Command` with the persistent `--file` flag + the noun's own
`NewCmd()`), importing only cobra + its own package + `clitest`/`cmdutil`.

### R4 — `format.go` / `format_test.go` split seams

`format.go` (30.8K) does **not** move atomically:
- `formatMoney` → `cmdutil` (the one cross-noun formatter).
- The ~20 `print*` table helpers → their owning noun (each leaves with its noun PR).
- `printHelp` (line 526) + `printVersion` (line 520) → **carved out, stay residual** (printVersion reads the residual `Version`/`BuildTime`/`GitCommit` vars). Move them to a new residual file (e.g. `roothelp.go`) in Phase 1.
- `format_test.go` splits: `TestFormatMoney` → `cmdutil`; `TestPrintHelp*` + `TestPrintVersion` → residual.

Also noted (out of scope, flag only): `printHelp` still hardcodes stale legacy
`--flag` help text (`--create`, `--backup`, …); `price.tdb`/`security.tdb`
(1 MB each) are untracked, gitignored cruft from manual runs — ensure the moves
don't `git add` them.

---

## Per-item Shape

Files are already one-per-verb (from the Cobra migration), so there is **no
"prep split" step** — each noun is a single move PR:

1. **MOVE (source)** — `git mv` the noun's source files into `internal/cli/<noun>/`;
   change the package clause to `<noun>`; alias the domain import per R1; lift
   the noun's `print*` helpers out of `format.go` into the package (unexported);
   rename the parent constructor `new<Noun>Cmd` → exported `<noun>.NewCmd`;
   rewire the `cmd.AddCommand(new<Noun>Cmd())` call in `root.go` to
   `<noun>.NewCmd()`; replace local `formatMoney`/`openServices`/
   `autoBackupAfterModification`/`--file` guard with `cmdutil.FormatMoney`/
   `cmdutil.OpenServices`/`cmdutil.AutoBackupAfterModification`/
   `cmdutil.RequireFile`. Production build green.
2. **TESTS (relocate + repoint)** — move the noun's `*_test.go` into the folder.
   Black-box files → `package <noun>_test`, importing `cli` (`ExecuteWith`,
   `SwapTUILauncher`) + `clitest` (fixtures) + aliased domain pkgs; mechanically
   replace `executeWith(args, out, err)` → `cli.ExecuteWith(...)`,
   `createX(...)` → `clitest.CreateX(...)`, `stubLaunchers(t)` →
   `cli.SwapTUILauncher(...)`. White-box files (R3) → `package <noun>` internal,
   testing unexported symbols directly. Relocate the noun's **test-local**
   helpers with their files (see each item). Full suite green; coverage 1:1.
3. **CLEANUP** — remove any now-dead `format.go` entries for the noun; for
   `price`/`security`, delete the stray `.tdb` and confirm tests use `t.TempDir()`.
4. **DOCS** — none per noun (behavior unchanged). A final docs pass (Phase 6)
   updates `docs/ARCHITECTURE.md`'s package overview.

Foundation items (Phase 1) introduce shared code with **package-`cli` shims**
(`func formatMoney(...) string { return cmdutil.FormatMoney(...) }`, etc.) so
unmoved nouns and the residual `format.go` printers keep compiling; the shims
are deleted in Phase 6.

---

## Phase 1: Foundation

- [ ] **PS-001 — Create `internal/cli/cmdutil`**
  - GREEN: new package `cmdutil` with exported `FormatMoney` (body moved from
    `format.go:23`), `OpenServices` + `AutoBackupAfterModification` (bodies moved
    from `helpers.go`), and a new `RequireFile(file string) error` folding the
    `--file is required to specify a database` guard. Leave package-`cli` shims
    (`formatMoney`, `openServices`, `autoBackupAfterModification`) delegating to
    `cmdutil` so all 50+ existing call sites compile unchanged. Carve `printHelp`
    + `printVersion` out of `format.go` into a residual `roothelp.go` (they stay
    `package cli`). `cmdutil` imports `app`, `backup`, `config`, `db`, `types`.
  - TESTS: move `TestFormatMoney` from `format_test.go` into
    `cmdutil/format_test.go` (`package cmdutil`) calling `FormatMoney`. Leave
    `TestPrintHelp*`/`TestPrintVersion` in `format_test.go` (now exercising the
    residual `roothelp.go`). Build + full suite green.
  - NOTE: `RequireFile` adoption is per-noun (each noun swaps its guard during
    its MOVE). `export.go` uses a divergent message (`--export requires --file`)
    — keep its bespoke guard or give `RequireFile` an optional message; decide
    in PS-016.

- [ ] **PS-002 — Create `internal/cli/clitest` (fixtures only, cli-free)**
  - GREEN: new package `clitest` (regular `.go`, imports `testing`) with exported
    fixtures moved from `testutil_test.go`: `CreateTestDBWithSecurity`,
    `CreateTestDBWithSecurityAndPrices`, `CreateInvestmentTestDB`,
    `CreateCorporateActionTestDB`, `PtrMoney`. Preserve the create→close→return-path
    pattern and exact signatures. **`clitest` must not import `cli`.** Leave
    shim wrappers in `testutil_test.go` (`func createInvestmentTestDB(t,b) string
    { return clitest.CreateInvestmentTestDB(t,b) }`, etc.) so existing
    `package cli` tests compile unchanged.
  - TESTS: none new; existing tests pass via shims. Full suite green.

- [ ] **PS-003 — Export the test harness from `cli`**
  - GREEN: rename `executeWith` → exported `ExecuteWith(args []string, stdout,
    stderr io.Writer) error` in `root.go`; keep a package-`cli` `executeWith`
    shim delegating to it. Add `SwapTUILauncher(fn func(string) error) (restore
    func())` exporting the launcher-swap seam; rewrite `root_test.go`'s
    `stubLaunchers` to call it. Build + full suite green.

## Phase 2: Pilot

- [ ] **PS-004 — `internal/cli/account`** (alias `accountdom`)
  - Source: `account.go` (`newAccountCmd`→`NewCmd`), `account_add.go`,
    `account_list.go`, `account_show.go`, `account_balance.go`. Printers from
    `format.go`: `printAccountsTable`, `printAccountDetails`, `printBalancesTable`
    (→ unexported, call `cmdutil.FormatMoney`).
  - Tests: `account_add_test.go`, `account_list_test.go`, `account_show_test.go`,
    `account_balance_test.go` → `package account_test` (no fixtures, no
    white-box). Repoint to `cli.ExecuteWith`.
  - Rewire `root.go:61` → `account.NewCmd()`. This PR establishes the alias +
    printer-lift + external-test recipe for all that follow.

## Phase 3: Highest value + complexity

- [ ] **PS-005 — `internal/cli/investment`** (alias `investmentdom`; 13 verbs)
  - Source: `investment.go` (`newInvestmentCmd`→`NewCmd`) + `investment_buy.go`,
    `_sell`, `_dividend`, `_reinvest`, `_fee`, `_deposit`, `_withdraw`,
    `_transfer`, `_split`, `_merge`, `_spin_off`, `_portfolio`,
    `_rebuild_positions`. Printer cluster from `format.go` (lines 749-992,
    8 fns, move together): `printPortfolioSummary`, `printPortfolioWithLots`,
    `printAccountTotals`, `printClosedPositions`, `partitionHoldings`,
    `formatGainLoss`, `formatReturnPct`, `formatFeesPaid`. Imports
    `internal/investment` (domain) for `Holding`/`AccountValuation`.
  - Tests: 13 files → mostly `package investment_test`. Fixtures:
    `clitest.CreateInvestmentTestDB`, `clitest.CreateCorporateActionTestDB`,
    `clitest.PtrMoney`. Move the 4 portfolio-local DB helpers
    (`createPortfolioCmdTestDB*` in `investment_portfolio_test.go`) with the file.
    **`investment_portfolio_test.go`'s two help tests** use `stubLaunchers` →
    repoint to `cli.SwapTUILauncher` (keeps them external).
  - Do NOT move `investment_transfer*` to the `transfer` noun — they are
    investment's share-transfer verb.

- [ ] **PS-006 — `internal/cli/security`** (alias `securitydom`; 7 verbs)
  - Source: `security.go` (`newSecurityCmd`→`NewCmd`) + `_add`, `_list`, `_show`,
    `_edit`, `_hide`, `_unhide`, `_delete`. Printers: `printSecuritiesTable`,
    `printSecurityDetails`.
  - Tests: 7 files → `package security_test`. Fixture:
    `clitest.CreateTestDBWithSecurity`. 4 test files import `internal/security`
    → alias `securitydom` there too.
  - CLEANUP: delete stray `internal/cli/security.tdb`; confirm `t.TempDir()` usage.

- [ ] **PS-007 — `internal/cli/price`** (alias `pricedom`; 5 verbs)
  - Source: `price.go` (`newPriceCmd`→`NewCmd`) + `_add`, `_list`, `_current`,
    `_import`, `_update`. Printer: `printPricesTable`. `price_update.go` carries
    its own `registerPriceProviders` hook var, `printRefreshResult`,
    `displayOutcome` (travel with the file, not from format.go).
  - Tests: `price_add/current/import/list_test.go` → `package price_test`
    (fixtures `clitest.CreateTestDBWithSecurity`,
    `clitest.CreateTestDBWithSecurityAndPrices`). **`price_update_test.go` →
    `package price` internal** (mutates `registerPriceProviders`); it runs
    `price update` via a partial inline root + `clitest` fixture. `*_Help` tests
    use `SwapTUILauncher`.
  - CLEANUP: delete stray `internal/cli/price.tdb`.

## Phase 4: Standard 4-verb nouns

- [ ] **PS-008 — `internal/cli/transaction`** (alias `transactiondom`)
  - Source: `transaction.go` (`newTransactionCmd`→`NewCmd`) + `_add`, `_list`,
    `_void`, `_search`. Printers: `printTransactionsTable`, `printSearchResults`.
  - Tests: 4 files → `package transaction_test` (no fixtures, no white-box).

- [ ] **PS-009 — `internal/cli/transfer`** (no alias needed)
  - Source: `transfer.go` (`newTransferCmd`→`NewCmd`) + `_add`, `_edit`,
    `_delete`, `_link`, **and the whole `transfer_resolve.go`** (`resolveTransferPair`
    + family, `resolvedTransfer`, `errTransferLineSplit`, `isNotFound`).
    `parseEditStatus` lives in `transfer_edit.go` (not resolve.go — spec note).
    No format.go printers (`transfer_link.go` has its own local
    `printLinkTransferPreview`/`writeCandidateTable`).
  - Tests: `transfer_add/edit/delete/link_test.go` → `package transfer_test`;
    **`transfer_resolve_test.go` → `package transfer` internal** (calls
    `resolveTransferPair`, reads unexported fields). Relocate test-local helpers
    `setupTransferAccounts`, `setupTransferDispatchAccounts` (in
    `transfer_add_test.go`), `openSvc`, `findInvestmentLegForTest` — note
    `findInvestmentLegForTest` is shared by edit/delete (external) **and**
    resolve (internal); duplicate it across the two test packages or hoist it
    into `clitest`.

- [ ] **PS-010 — `internal/cli/scheduled`** (alias `scheduleddom`)
  - Source: `scheduled.go` (`newScheduledCmd`→`NewCmd`) + `_add`, `_list`,
    `_post`, `_skip`. Printer: `printScheduledTransactionsTable`.
  - Tests: 4 files → `package scheduled_test` (no fixtures, no white-box).

- [ ] **PS-011 — `internal/cli/reconcile`** (no alias; domain is `reconciliation`)
  - Source: `reconcile.go` (`newReconcileCmd`→`NewCmd`) + `_start`, `_mark`,
    `_finish`, `_status`. Printer: `printReconcileStatus`. `reconcile_finish.go`
    imports `internal/reconciliation` (`DifferenceError`).
  - Tests: 4 files → `package reconcile_test` (no fixtures, no white-box).
    `workflow_test.go` also drives reconcile but **stays residual**.

- [ ] **PS-012 — `internal/cli/db`** (alias `dbpkg`)
  - Source: `db.go` (`newDBCmd`→`NewCmd`) + `_create`, `_backup`, `_restore`,
    `_list_backups`. No format.go printers (inline tabwriter). `db_create.go`
    has no `--file` guard (creates, doesn't open) — don't add `RequireFile`.
  - Tests: 4 files → `package db_test`. `*_Help` tests use `SwapTUILauncher`.

## Phase 5: Small / special

- [ ] **PS-013 — `internal/cli/report`** (alias `reportdom`)
  - Source: `report.go` (`newReportCmd`→`NewCmd`) + `report_net_worth.go`,
    `report_spending.go`. Printers: `printNetWorthReport`, `printSpendingReport`.
    `parseYearMonth` lives in `report_spending.go` (→ unexported in `report`).
    Verb `net-worth` uses ctor `newReportNetWorthCmd` / file `report_net_worth.go`.
  - Tests: `report_net_worth_test.go` → `package report_test`.
    **`report_spending_test.go`**: split — `TestParseYearMonth` into a small
    `package report` internal file; the rest → `package report_test`. `*_Help`
    tests use `SwapTUILauncher`.

- [ ] **PS-014 — `internal/cli/theme`** (alias `tuitheme` for `internal/tui/theme`)
  - Source: `theme.go` (`newThemeCmd`→`NewCmd`) + `theme_list.go`,
    `theme_wal.go`, **and the whole `wal.go`** (generation/parse:
    `walToThemeTOML`, `ReadWalColors`/`WalColors`/`WalSpecial`/`WalColorTable`,
    `walCachePath`, `activeThemeIDFn` var). No format.go printers. Consider
    unexporting `ReadWalColors` et al. (no cross-package callers). Verb
    `generate-from-wal` → ctor `newThemeGenerateFromWalCmd`.
  - Move `testdata/wal-sample-colors.json` → `internal/cli/theme/testdata/`.
  - Tests: `theme_test.go`, `theme_list_test.go` → `package theme_test`;
    **`wal_test.go` + `theme_wal_subcmd_test.go` → `package theme` internal**
    (call `walToThemeTOML`/`ReadWalColors`/`walCachePath`). Relocate test-local
    helpers `writeUserTheme`, `writeConfigTheme`, `installSampleWalCache` with
    their files. `*_Help` tests use `SwapTUILauncher`. (No `theme_wal_test.go`
    exists — spec hint was wrong; 4 test files total.)

## Phase 6: Final cleanup

- [ ] **PS-015 — Delete shims**
  - GREEN: remove the package-`cli` shims `formatMoney`, `openServices`,
    `autoBackupAfterModification`, `executeWith`, and the `testutil_test.go`
    fixture wrappers — every caller now references `cmdutil.*` / `cli.ExecuteWith`
    / `clitest.*` directly. Delete `testutil_test.go`. Build + full suite green.

- [ ] **PS-016 — Collapse `format.go`, tidy `root.go`, remove cruft**
  - GREEN: confirm `format.go` holds nothing but already-moved code and delete it
    (only `roothelp.go`'s `printHelp`/`printVersion` remain residual). Verify
    `root.go` `newRootCmd()` calls 11 `<noun>.NewCmd()` + 3 local
    (`newVersionCmd`/`newImportCmd`/`newExportCmd`). Resolve the `RequireFile`
    message decision for `export.go`. Delete untracked `price.tdb`/`security.tdb`
    (ensure not `git add`-ed). Optionally fix `printHelp`'s stale legacy
    `--flag` text. Full suite green.

- [ ] **PS-017 — DOCS: update `docs/ARCHITECTURE.md` package overview**
  - GREEN: reflect the new `internal/cli/{cmdutil,clitest,<noun>...}` layout in
    the architecture doc's package map. Add a status note to
    `specs/cli-package-split.md` marking the split complete.

---

## Out of Scope

- **Renaming CLI packages to `<noun>cmd`** to avoid domain-import aliasing (see
  R1 alternative) — only if the team prefers it over aliasing.
- **Fixing `printHelp`'s stale legacy `--flag` help text** — flagged in PS-016 as
  optional; it predates the Cobra migration and is a separate cleanup.
- **Splitting `format.go`'s remaining printers per-verb beyond noun granularity** —
  printers move whole to their noun; no finer split.
- **Behavior/UX changes** — no new commands, flags, output, or completion work.
