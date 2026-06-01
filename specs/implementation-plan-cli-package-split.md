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

- [x] **PS-001 — Create `internal/cli/cmdutil`**
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

- [x] **PS-002 — Create `internal/cli/clitest` (fixtures only, cli-free)**
  - GREEN: new package `clitest` (regular `.go`, imports `testing`) with exported
    fixtures moved from `testutil_test.go`: `CreateTestDBWithSecurity`,
    `CreateTestDBWithSecurityAndPrices`, `CreateInvestmentTestDB`,
    `CreateCorporateActionTestDB`, `PtrMoney`. Preserve the create→close→return-path
    pattern and exact signatures. **`clitest` must not import `cli`.** Leave
    shim wrappers in `testutil_test.go` (`func createInvestmentTestDB(t,b) string
    { return clitest.CreateInvestmentTestDB(t,b) }`, etc.) so existing
    `package cli` tests compile unchanged.
  - TESTS: added `clitest/fixtures_test.go` — 5 smoke tests that reopen each
    fixture's DB and assert the rows persisted (+ `PtrMoney` round-trip). Existing
    `package cli` tests pass unchanged via shims. Full suite green.
  - NOTE: moving the fixtures out of a `_test.go` file dropped the errcheck
    `_test\.go` exclusion, so the four `database.Close()` calls are now checked
    with `t.Fatalf` (matches the fixtures' own error-handling style; a failed
    flush would make the reopen tests flaky). Behavior is otherwise identical.

- [x] **PS-003 — Export the test harness from `cli`**
  - GREEN: renamed `executeWith` → exported `ExecuteWith(args []string, stdout,
    stderr io.Writer) error` in `root.go`; kept a package-`cli` `executeWith`
    shim delegating to it (removed in PS-015). Added `SwapTUILauncher(fn
    func(string) error) (restore func())` next to `tuiLauncher`, exporting the
    launcher-swap seam; rewrote `root_test.go`'s `stubLaunchers` to delegate to
    it (the ~54 `*_Help`/launch tests that call `stubLaunchers` are unchanged).
    `Execute()` now calls `ExecuteWith`.
  - TESTS: added `harness_export_test.go` (`package cli_test`, the first
    external test package) proving the cycle-free recipe every noun PR relies
    on — `cli.ExecuteWith` drives the full root by argv, and `cli.SwapTUILauncher`
    intercepts launches with LIFO-safe restore (never invoking the real TUI).
    Build + full suite green (`go test ./...`: 5595 passed); `golangci-lint`
    clean.

## Phase 2: Pilot

- [x] **PS-004 — `internal/cli/account`** (alias `accountdom`)
  - GREEN: `git mv`'d the 5 source files into `internal/cli/account/`
    (`account.go`, `add.go`, `list.go`, `show.go`, `balance.go`), changed the
    package clause to `account`, aliased `internal/account` as `accountdom`, and
    renamed `newAccountCmd`→exported `account.NewCmd()` (verb ctors stay
    unexported). Lifted `printAccountsTable`/`printAccountDetails`/
    `printBalancesTable` out of `format.go` into a new
    `internal/cli/account/format.go` (unexported, calling `cmdutil.FormatMoney`);
    deleted them from the residual `format.go` (−113 lines). Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the
    `--file` guard to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `FormatMoney`/`RequireFile`. Rewired `root.go` to import
    `internal/cli/account` and call `account.NewCmd()`.
  - TESTS: `add_test.go`, `list_test.go`, `show_test.go`, `balance_test.go` →
    external `package account_test`, importing `cli` (`ExecuteWith`,
    `SwapTUILauncher`) + aliased `accountdom`; mechanically repointed
    `executeWith`→`cli.ExecuteWith` and `stubLaunchers(t)` (the help tests
    discarded its tui-calls slice)→`cli.SwapTUILauncher`. No fixtures, no
    white-box. Build + full suite green (`go test ./...`: 5595 passed, the
    PS-003 baseline — coverage 1:1); `go vet`, `gofmt`, `golangci-lint` clean.
  - This PR establishes the alias + printer-lift + external-`_test` recipe every
    noun PR that follows copies.

## Phase 3: Highest value + complexity

- [x] **PS-005 — `internal/cli/investment`** (alias `investmentdom`; 13 verbs)
  - DONE: `git mv`'d the 14 source files into `internal/cli/investment/`
    (`investment.go` + `buy/sell/dividend/reinvest/fee/deposit/withdraw/transfer/
    split/merge/spin_off/portfolio/rebuild_positions.go`), changed the package
    clause to `investment`, aliased `internal/investment` as `investmentdom`, and
    renamed `newInvestmentCmd`→exported `investment.NewCmd()` (verb ctors stay
    unexported). Lifted the 8-function portfolio-printer cluster
    (`printPortfolioSummary`, `printPortfolioWithLots`, `printAccountTotals`,
    `printClosedPositions`, `partitionHoldings`, `formatGainLoss`,
    `formatReturnPct`, `formatFeesPaid`) out of `format.go` into
    `internal/cli/investment/format.go` (unexported, calling
    `cmdutil.FormatMoney`); deleted them + the now-unused `app`/`investment`
    imports from the residual `format.go` (−248 lines). Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the
    `--file` guard to `cmdutil.*`/`cmdutil.RequireFile`. Rewired `root.go` to
    import `internal/cli/investment` and call `investment.NewCmd()`.
  - DONE (tests): 13 `*_test.go` → external `package investment_test`, importing
    `cli` (`ExecuteWith`, `SwapTUILauncher`) + `clitest` (fixtures) + aliased
    `investmentdom` (portfolio only); mechanically repointed
    `executeWith`→`cli.ExecuteWith`, `create{Investment,CorporateAction}TestDB`/
    `ptrMoney`→`clitest.*`, and `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(...)`. The 4 portfolio-local DB helpers
    (`createPortfolioCmdTestDB*`) traveled with `portfolio_test.go`. No white-box
    files (all 13 external). Removed the now-orphaned `ptrMoney` +
    `createCorporateActionTestDB` shims (and the `types` import) from
    `testutil_test.go` — investment was their only consumer; the remaining
    fixture shims stay (still used by security/price/`workflow_test.go`).
  - VERIFIED: `go build ./...`, `go vet ./...`, `go fix ./...`, `gofmt -l` all
    clean; `go test ./...` green (investment pkg 100% relocated coverage);
    `golangci-lint run` clean.
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

- [x] **PS-006 — `internal/cli/security`** (alias `securitydom`; 7 verbs)
  - DONE: `git mv`'d the 8 source files into `internal/cli/security/`
    (`security.go` + `add/list/show/edit/hide/unhide/delete.go`), changed the
    package clause to `security`, aliased `internal/security` as `securitydom` in
    the 4 files that use it (`add`, `list`, `edit`, `delete`), and renamed
    `newSecurityCmd`→exported `security.NewCmd()` (verb ctors stay unexported).
    Lifted `printSecuritiesTable`/`printSecurityDetails` out of `format.go` into
    `internal/cli/security/format.go` (unexported; uses `securitydom` — neither
    printer calls `cmdutil.FormatMoney`); deleted them + the now-unused
    `internal/security` import from the residual `format.go` (−58 lines). Swapped
    every `openServices`/`autoBackupAfterModification`/`formatMoney` call and the
    `--file` guard to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `RequireFile`. Rewired `root.go` to import `internal/cli/security` and call
    `security.NewCmd()`.
  - DONE (tests): 7 `*_test.go` → external `package security_test`, importing
    `cli` (`ExecuteWith`, `SwapTUILauncher`) + `clitest` (fixtures) + aliased
    `securitydom` (list/hide/unhide/delete) + `types` (delete); mechanically
    repointed `executeWith`→`cli.ExecuteWith`,
    `createTestDBWithSecurity(t)`→`clitest.CreateTestDBWithSecurity(t)`, and
    `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`. No
    white-box files (all 7 external). The `testutil_test.go` fixture shims stay —
    `createTestDBWithSecurity`/`createTestDBWithSecurityAndPrices` are still
    consumed by the residual `price_*_test.go` (PS-007 not yet done).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./...`, `gofmt -l` all
    clean; `go test ./...` green (5595 passed in 33 packages — the PS-003/PS-004
    baseline, coverage 1:1); `golangci-lint run ./...` clean.
  - CLEANUP: deleted stray gitignored `internal/cli/security.tdb` (1 MB); all 7
    test files use `t.TempDir()` / the `clitest` fixture (no on-disk fixtures).

- [x] **PS-007 — `internal/cli/price`** (alias `pricedom`; 5 verbs)
  - DONE: `git mv`'d the 6 source files into `internal/cli/price/`
    (`price.go` + `add/current/import/list/update.go`), changed the package
    clause to `price`, aliased `internal/price` as `pricedom` in the 3 files that
    reference it (`add`, `import`, `update`) plus the new `format.go`
    (`current.go`/`list.go` have no domain-price refs, so no alias), and renamed
    `newPriceCmd`→exported `price.NewCmd()` (verb ctors stay unexported). Lifted
    `printPricesTable` out of `format.go` into `internal/cli/price/format.go`
    (unexported, using `pricedom`); deleted it + the now-unused `internal/price`
    import from the residual `format.go`. `price_update.go`'s own
    `registerPriceProviders` hook var, `printRefreshResult`, and `displayOutcome`
    traveled with the file (not from format.go). Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the
    `--file` guard to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `FormatMoney`/`RequireFile` (`cmdutil.RequireFile`'s message is byte-identical
    to both the `fmt.Errorf` and `errors.New` guards it replaced). Rewired
    `root.go` to import `internal/cli/price` and call `price.NewCmd()`.
  - DONE (tests): `price_add/current/import/list_test.go` → external
    `package price_test`, importing `cli` (`ExecuteWith`, `SwapTUILauncher`) +
    `clitest` (`CreateTestDBWithSecurity`/`CreateTestDBWithSecurityAndPrices`);
    mechanically repointed `executeWith`→`cli.ExecuteWith`,
    `createTestDBWithSecurity[AndPrices]`→`clitest.*`, and
    `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`.
    **`price_update_test.go` is the first genuinely white-box file (R3)** — it
    mutates the unexported `registerPriceProviders`, so it stays `package price`
    internal (`update_test.go`) and runs the command via a partial inline root
    helper `execPrice` (throwaway `*cobra.Command` with the persistent `--file`
    flag + `NewCmd()`), **not** `cli.ExecuteWith` (which would cycle
    `price → cli → price`). Its 2 help tests (`TestPriceUpdate_Help`,
    `TestPriceCmd_HelpListsUpdate`) were split out to external
    `update_help_test.go` (`package price_test`) using `cli.ExecuteWith` +
    `cli.SwapTUILauncher`. Removed the now-orphaned
    `createTestDBWithSecurity`/`createTestDBWithSecurityAndPrices` shims (and the
    `security` import) from `testutil_test.go`; `createInvestmentTestDB` stays
    (still consumed by `workflow_test.go`).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./...`, `gofmt -l` all
    clean; `go test ./...` green (5595 passed in 34 packages — the PS-006 baseline
    of 5595, now in 34 pkgs since `price` is its own test package; coverage 1:1,
    all 49 price test functions preserved); `golangci-lint run ./internal/cli/...`
    clean. An adversarial 3-lens review (behavior / coverage / spec-conformance)
    confirmed no behavior change, `execPrice` is behaviorally equivalent to the
    real root for the white-box tests, and `go list -test -deps` shows the
    internal `package price` test imports zero `internal/cli` (R2/D5 cycle-free).
  - CLEANUP: deleted stray gitignored `internal/cli/price.tdb` (1 MB); all test
    files use `t.TempDir()` / the `clitest` fixture (no on-disk fixtures).

## Phase 4: Standard 4-verb nouns

- [x] **PS-008 — `internal/cli/transaction`** (alias `transactiondom`)
  - DONE: `git mv`'d the 5 source files into `internal/cli/transaction/`
    (`transaction.go` + `add/list/void/search.go`), changed the package clause to
    `transaction`, aliased `internal/transaction` as `transactiondom` in the 3
    files that reference it (`add`, `list`, `search`) plus the new `format.go`
    (`void.go` uses only `svc` + `types`, so no domain import / no alias), and
    renamed `newTransactionCmd`→exported `transaction.NewCmd()` (verb ctors stay
    unexported). Lifted `printTransactionsTable`/`printSearchResults` out of
    `format.go` into `internal/cli/transaction/format.go` (unexported, using
    `cmdutil.FormatMoney`, `transactiondom.Transaction`, and the unaliased foreign
    `account.Account`); deleted them + the now-unused `internal/transaction`
    import from the residual `format.go`. Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the
    `--file` guard to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `FormatMoney`/`RequireFile` (`cmdutil.RequireFile`'s message is byte-identical
    to the `fmt.Errorf` guards it replaced). Rewired `root.go` to import
    `internal/cli/transaction` and call `transaction.NewCmd()`.
  - DONE (tests): `add/list/void/search_test.go` → external
    `package transaction_test`, importing `cli` (`ExecuteWith`, `SwapTUILauncher`)
    + aliased `transactiondom`; mechanically repointed
    `executeWith`→`cli.ExecuteWith`, `transaction.{NewRepository,NewTransaction,
    StatusVoid}`→`transactiondom.*`, and `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`. No
    fixtures, no white-box files (all 4 external) — matches the plan. The
    `testutil_test.go` `createInvestmentTestDB` shim stays (still consumed by the
    residual `workflow_test.go`).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./internal/cli/...`,
    `gofmt -l` all clean; `go test ./...` green (5595 passed in 35 packages — the
    PS-007 baseline of 5595, now in 35 pkgs since `transaction` is its own test
    package; coverage 1:1, all 55 transaction test functions preserved);
    `golangci-lint run ./internal/cli/...` clean. An adversarial 3-lens review
    (behavior / coverage-1:1 / spec-conformance+cycle-safety) confirmed only
    mechanical changes, no dropped/weakened tests, and that the production
    `internal/cli/transaction` package does not import `internal/cli` (R2/D5
    cycle-free).

- [x] **PS-009 — `internal/cli/transfer`** (no alias needed)
  - DONE: `git mv`'d the 6 source files into `internal/cli/transfer/`
    (`transfer.go` + `add/edit/delete/link/resolve.go`), changed the package
    clause to `transfer`, and renamed `newTransferCmd`→exported `transfer.NewCmd()`
    (verb ctors stay unexported). **No domain alias** — there is no
    `internal/transfer`; the link domain `internal/transferlink` (distinct name)
    is imported unaliased. The whole `transfer_resolve.go` family
    (`resolveTransferPair`, `resolveFromRegularLeg`, `resolveFromInvestmentLeg`,
    `findInvestmentLeg`, `refuseIfMultiLineSplit`, `investmentStatusToRegular`,
    `isNotFound`, `resolvedTransfer`, `errTransferLineSplit`) moved together;
    `parseEditStatus` stayed in `edit.go`. **No format.go printers** —
    `printLinkTransferPreview`/`writeCandidateTable` are local in `link.go`
    (residual `internal/cli/format.go` is untouched — empty diff). Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the four
    `--file` guards to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `FormatMoney`/`RequireFile` (`cmdutil.RequireFile`'s message is byte-identical
    to the `fmt.Errorf` guards it replaced). Rewired `root.go` to import
    `internal/cli/transfer` and call `transfer.NewCmd()`.
  - DONE (tests): `add/edit/delete/link_test.go` → external
    `package transfer_test`, importing `cli` (`ExecuteWith`, `SwapTUILauncher`) +
    `clitest` (shared fixtures) + domain pkgs; mechanically repointed
    `executeWith`→`cli.ExecuteWith` and `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`.
    **`resolve_test.go` stays `package transfer` internal (R3)** — it calls
    `resolveTransferPair` and reads the unexported `resolvedTransfer` fields /
    `*errTransferLineSplit`. The four cross-boundary helpers
    (`setupTransferAccounts`, `setupTransferDispatchAccounts`, `openSvc`,
    `findInvestmentLegForTest`) — each used by **both** the internal `resolve_test`
    and the external `add`/`edit`/`delete` tests — were **hoisted into
    `clitest`** (`SetupTransferAccounts`, `SetupTransferDispatchAccounts`,
    `OpenSvc`, `FindInvestmentLegForTest`) rather than duplicated, with 3 additive
    `clitest` smoke tests (matching the PS-002 precedent). `clitest.OpenSvc` opens
    via `db.Open`+`app.NewServices` (cli-free per D5/R2; behavior-equivalent for
    the schedule-free fixtures) — documented inline. Single-package helpers
    (`assertTransferLegsExist`, `assertInvTransferAmount`, `assertTransferGone`,
    `reconcileLegs`, `setupLinkScenario`) stayed local with their files.
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./internal/cli/...`,
    `gofmt -l` all clean; `go test ./...` green (5598 passed in 36 packages — the
    PS-008 baseline of 5595, now in 36 pkgs since `transfer` is its own test
    package, +3 additive `clitest` smoke tests; all 50 transfer test functions
    preserved 1:1 — add 20, edit 7, delete 7, link 9, resolve 7);
    `golangci-lint run ./internal/cli/...` clean. An adversarial 3-lens review
    (behavior / coverage-1:1 / spec-conformance+cycle-safety) returned pass on all
    three with no blockers/majors/minors: `go list -deps` confirms neither the
    production `internal/cli/transfer` nor `clitest` imports the root
    `internal/cli` (R2/D5 cycle-free), the source diffs are mechanical only, and
    coverage is exactly 1:1.

- [x] **PS-010 — `internal/cli/scheduled`** (alias `scheduleddom`)
  - DONE: `git mv`'d the 5 source files into `internal/cli/scheduled/`
    (`scheduled.go` + `add/list/post/skip.go`), changed the package clause to
    `scheduled`, aliased `internal/scheduled` as `scheduleddom` in the files that
    reference it (`add`, `list`, plus the new `format.go`; `post.go`/`skip.go`
    touch only the foreign `internal/transaction`, left unaliased per convention),
    and renamed `newScheduledCmd`→exported `scheduled.NewCmd()` (verb ctors stay
    unexported). Lifted `printScheduledTransactionsTable` out of `format.go` into
    `internal/cli/scheduled/format.go` (unexported, using `cmdutil.FormatMoney` and
    `scheduleddom.Transaction`); deleted it + the now-unused `internal/scheduled`
    import from the residual `format.go` (−84 lines). Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the four
    `--file` guards to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `FormatMoney`/`RequireFile` (`cmdutil.RequireFile`'s message is byte-identical
    to the `fmt.Errorf` guards it replaced). Rewired `root.go` to import
    `internal/cli/scheduled` and call `scheduled.NewCmd()`.
  - DONE (tests): `add/list/post/skip_test.go` → external `package scheduled_test`,
    importing `cli` (`ExecuteWith`, `SwapTUILauncher`) + aliased `scheduleddom`
    (list/post/skip) + foreign domains (`account`/`db`/`payee`/`transaction`/`types`,
    unaliased); mechanically repointed `executeWith`→`cli.ExecuteWith` and
    `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`. No
    fixtures, no white-box files (all 4 external) — matches the plan. The
    `testutil_test.go` `createInvestmentTestDB` shim stays (still consumed by the
    residual `workflow_test.go`).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./internal/cli/...`,
    `gofmt -l` all clean; `go test ./...` green (5598 passed in 37 packages — the
    PS-009 baseline of 5598, now in 37 pkgs since `scheduled` is its own test
    package; coverage 1:1, all 46 scheduled test functions preserved — add 12,
    list 12, post 11, skip 7); `golangci-lint run ./internal/cli/...` clean. An
    adversarial 3-lens review (behavior / coverage-1:1 / spec-conformance+
    cycle-safety) returned pass on all three with no blockers/majors:
    `go list -deps` confirms the production `internal/cli/scheduled` does not
    import the root `internal/cli` (R2/D5 cycle-free), source diffs are mechanical
    only, and coverage is exactly 1:1.

- [x] **PS-011 — `internal/cli/reconcile`** (no alias; domain is `reconciliation`)
  - DONE: `git mv`'d the 5 source files into `internal/cli/reconcile/`
    (`reconcile.go` + `start/mark/finish/status.go`), changed the package clause
    to `reconcile`, and renamed `newReconcileCmd`→exported `reconcile.NewCmd()`
    (verb ctors stay unexported). **No domain alias** — the reconcile domain
    package is `internal/reconciliation` (distinct name), so `internal/reconciliation`
    (`finish.go` for `DifferenceError`, `format.go` for `Status`) and the foreign
    `internal/account` (`format.go`) are imported unaliased. Lifted
    `printReconcileStatus` out of `format.go` into `internal/cli/reconcile/format.go`
    (unexported, using `cmdutil.FormatMoney`); deleted it + the now-unused
    `internal/account` and `internal/reconciliation` imports from the residual
    `format.go` (which still compiles — `printNetWorthReport`/`printSpendingReport`
    remain, report noun not yet moved). Swapped every
    `openServices`/`autoBackupAfterModification`/`formatMoney` call and the four
    `--file` guards to `cmdutil.OpenServices`/`AutoBackupAfterModification`/
    `FormatMoney`/`RequireFile` (`cmdutil.RequireFile`'s message is byte-identical
    to the `fmt.Errorf` guards it replaced; `autoBackupAfterModification` correctly
    travels with the data-modifying `start`/`finish` only — `mark`/`status` never
    had it). Rewired `root.go` to import `internal/cli/reconcile` and call
    `reconcile.NewCmd()`.
  - DONE (tests): `start/mark/finish/status_test.go` → external
    `package reconcile_test`, importing `cli` (`ExecuteWith`, `SwapTUILauncher`) +
    foreign domains (`account`/`db`/`reconciliation`/`transaction`/`types`,
    unaliased); mechanically repointed `executeWith`→`cli.ExecuteWith` and
    `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`. No
    fixtures, no white-box files (all 4 external) — matches the plan. The
    cross-noun `workflow_test.go` **stays residual** `package cli` and still drives
    the reconcile lifecycle via `cli.ExecuteWith`, still exercising the lifted
    `printReconcileStatus` output. The `testutil_test.go` `createInvestmentTestDB`
    shim stays (still consumed by `workflow_test.go`).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./internal/cli/...`,
    `gofmt -l` all clean; `go test ./...` green (5598 passed in 38 packages — the
    PS-010 baseline of 5598, now in 38 pkgs since `reconcile` is its own test
    package; coverage 1:1, all 33 reconcile test functions preserved — start 10,
    mark 8, finish 8, status 7); `golangci-lint run ./internal/cli/...` clean. An
    adversarial 3-lens review (behavior-equivalence / coverage-1:1 / spec-conformance+
    cycle-safety) returned pass on all three with no blockers/majors/minors:
    `go list -deps` confirms the production `internal/cli/reconcile` does not import
    the root `internal/cli` (R2/D5 cycle-free — direct deps are `account`,
    `cli/cmdutil`, `reconciliation`, `types`, `cobra`), source diffs are mechanical
    only, and no residual `package cli` code references `newReconcileCmd` or
    `printReconcileStatus`.

- [x] **PS-012 — `internal/cli/db`** (alias `dbpkg`)
  - DONE: `git mv`'d the 5 source files into `internal/cli/db/`
    (`db.go` + `create/backup/restore/list_backups.go`), changed the package
    clause to `db`, aliased `internal/db` as `dbpkg` in the one file that
    references it (`create.go`; `backup`/`restore`/`list_backups` touch only
    `internal/backup` + stdlib, no domain-`db` ref), and renamed
    `newDBCmd`→exported `db.NewCmd()` (verb ctors stay unexported). **No
    format.go printers** — `list-backups` uses an inline `tabwriter`, so the
    residual `internal/cli/format.go` is untouched (empty diff). `db_create.go`
    correctly has **no** `--file` guard (it creates, doesn't open) — `RequireFile`
    was *not* added there. Swapped the three `--file` guards in `backup`/`restore`/
    `list_backups` to `cmdutil.RequireFile` (`cmdutil.RequireFile`'s message is
    byte-identical to the `fmt.Errorf("--file is required to specify a database")`
    guards it replaced); db verbs never used `openServices`/`formatMoney`/
    `autoBackupAfterModification`, so no other `cmdutil.*` swaps. Rewired `root.go`
    to import `internal/cli/db` and call `db.NewCmd()`.
  - DONE (tests): `create/backup/restore/list_backups_test.go` → external
    `package db_test`, importing `cli` (`ExecuteWith`, `SwapTUILauncher`) +
    aliased `dbpkg` (for the `dbpkg.Open`/`dbpkg.Create` setup/assert calls) +
    `internal/backup` (restore/list_backups); mechanically repointed
    `executeWith`→`cli.ExecuteWith`, `db.{Open,Create}`→`dbpkg.{Open,Create}`, and
    `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`. No
    fixtures, no white-box files (all 4 external) — matches the plan. The
    `testutil_test.go` `createInvestmentTestDB` shim stays (still consumed by the
    residual `workflow_test.go`).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./internal/cli/...`,
    `gofmt -l` all clean; `go test ./...` green (5598 passed in 39 packages — the
    PS-011 baseline of 5598, now in 39 pkgs since `db` is its own test package;
    coverage 1:1, all 29 db test functions preserved — create 7, backup 7,
    restore 8, list-backups 7); `golangci-lint run ./internal/cli/...` clean. An
    adversarial 3-lens review (behavior-equivalence / coverage-1:1 / spec-conformance+
    cycle-safety) returned pass on all three with no blockers/majors/minors:
    `go list -deps` confirms the production `internal/cli/db` does not import the
    root `internal/cli` (R2/D5 cycle-free — direct CLI dep is only
    `internal/cli/cmdutil`), source diffs are mechanical only, and no residual
    `package cli` code references `newDBCmd`.

## Phase 5: Small / special

- [x] **PS-013 — `internal/cli/report`** (alias `reportdom`)
  - DONE: `git mv`'d the 3 source files into `internal/cli/report/`
    (`report.go` + `net_worth.go`, `spending.go`), changed the package clause to
    `report`, aliased `internal/report` as `reportdom` in the files that reference
    it (`net_worth`, `spending`, and the new `format.go`), and renamed
    `newReportCmd`→exported `report.NewCmd()` (verb ctors `newReportNetWorthCmd`/
    `newReportSpendingCmd` stay unexported). Lifted `printNetWorthReport`/
    `printSpendingReport` out of `format.go` into `internal/cli/report/format.go`
    (unexported, using `cmdutil.FormatMoney` + `reportdom`). `parseYearMonth`
    traveled with `spending.go` (unexported, still `package report`). Swapped every
    `openServices` call and the two `--file` guards to `cmdutil.OpenServices`/
    `cmdutil.RequireFile` (`cmdutil.RequireFile`'s message is byte-identical to the
    `fmt.Errorf` guards it replaced; report verbs never used
    `formatMoney`/`autoBackupAfterModification` directly — read-only commands).
    Rewired `root.go` to import `internal/cli/report` and call `report.NewCmd()`.
  - **Subsumed PS-015/PS-016 cleanup for `format.go`.** `printNetWorthReport`/
    `printSpendingReport` were the **last two printers** in the residual
    `internal/cli/format.go`; once they moved, the residual `formatMoney` shim had
    zero callers (grep-confirmed: only `format.go` referenced it), which the active
    `unused` linter (v2 default set) flags, and removing it leaves `format.go` with
    no declarations. So PS-013 necessarily **`git rm`'d the residual `format.go`
    entirely** — it held nothing but those two printers + the `formatMoney` shim.
    This pulls forward the `format.go` deletion PS-016 had tentatively scheduled and
    the `formatMoney`-shim removal from PS-015. The other shims (`openServices`,
    `autoBackupAfterModification`, `executeWith`) **remain** (still used by
    `import.go`/`export.go`/residual tests) and are removed in PS-015. The residual
    `format_test.go` is untouched — it exercises `roothelp.go`'s `printHelp`/
    `printVersion`, never `format.go`.
  - DONE (tests): `net_worth_test.go` → external `package report_test`, importing
    `cli` (`ExecuteWith`, `SwapTUILauncher`) + foreign domains
    (`account`/`db`/`transaction`/`types`, unaliased). `spending_test.go` → external
    `package report_test` minus `TestParseYearMonth` (+ `category` import). Both
    mechanically repointed `executeWith`→`cli.ExecuteWith` and
    `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`.
    **`TestParseYearMonth` → internal `package report` file
    `spending_internal_test.go` (R3 white-box)** — it calls the unexported
    `parseYearMonth` directly with no command execution, so (unlike price's
    `update_test.go`) it needs **no** partial inline root and imports only
    `testing`; cycle-free by construction (`package report` test imports zero
    `internal/cli`). No `clitest` fixtures needed (report tests build DBs inline
    via `db.Create` + repos).
  - VERIFIED: `go fix ./...`, `go build ./...`, `go vet ./internal/cli/...`,
    `gofmt -l internal/cli/` all clean; `go test ./...` green (report pkg: 30 passed
    = all 22 report test functions preserved 1:1 — net-worth 9, spending 12 +
    `TestParseYearMonth` with its 8 subtests); `golangci-lint run ./internal/cli/...`
    clean (confirms the `formatMoney`/`format.go` orphan is fully resolved, not
    merely tolerated).

- [x] **PS-014 — `internal/cli/theme`** (alias `tuitheme` for `internal/tui/theme`)
  - DONE: `git mv`'d the 4 source files into `internal/cli/theme/`
    (`theme.go` + `list.go` (was `theme_list.go`), `generate_from_wal.go` (was
    `theme_wal.go`), and the **whole `wal.go`**), changed the package clause to
    `theme`, aliased `internal/tui/theme` as `tuitheme` in the 3 files that
    reference it (`list.go` for `DiscoverUserThemes`/`BuiltinIDs`/`LoadTheme`,
    `generate_from_wal.go` for `UserThemesDir`; `wal.go` has no domain-theme
    ref so no alias), and renamed `newThemeCmd`→exported `theme.NewCmd()` (verb
    ctors `newThemeListCmd`/`newThemeGenerateFromWalCmd` stay unexported). **No
    format.go printers** — theme never had any (residual `format.go` was already
    deleted in PS-013; this PR's diff doesn't touch it). **Unexported
    `ReadWalColors`/`WalColors`/`WalSpecial`/`WalColorTable`** →
    `readWalColors`/`walColors`/`walSpecial`/`walColorTable` (grep-verified zero
    cross-package callers — the only external reference was
    `root.go`'s `newThemeCmd()` call), so `internal/cli/theme` now exports
    exactly one symbol (`NewCmd`), matching every other noun; this is what makes
    the white-box test internal (R3). `walCachePath`/`defaultWalThemePath`/
    `walToThemeTOML`/`activeThemeIDFn` stayed unexported, travelling with their
    files. Theme verbs never used
    `openServices`/`autoBackupAfterModification`/`formatMoney`/the `--file` guard,
    so **no `cmdutil.*` swaps** were needed. Rewired `root.go` to import
    `internal/cli/theme` and call `theme.NewCmd()` (same position).
  - DONE (testdata): `git mv`'d `testdata/wal-sample-colors.json` →
    `internal/cli/theme/testdata/`; the now-empty `internal/cli/testdata/` dir
    was removed.
  - DONE (tests): `theme_test.go` (3), `list_test.go` (was `theme_list_test.go`,
    7), and the command tests carved out of `theme_wal_subcmd_test.go` into
    `generate_from_wal_test.go` (5) → external `package theme_test`, importing
    `cli` (`ExecuteWith`, `SwapTUILauncher`) + aliased `tuitheme` (only where
    `theme.Parse` is used); mechanically repointed `executeWith`→`cli.ExecuteWith`,
    `theme.Parse`→`tuitheme.Parse`, and `_, restore := stubLaunchers(t)`→
    `restore := cli.SwapTUILauncher(func(string) error { return nil })`. The
    test-local helpers `writeUserTheme`/`writeConfigTheme` (with `list_test.go`)
    and `installSampleWalCache` (with `generate_from_wal_test.go`) travelled with
    their files. **`wal_test.go` is the white-box file (R3)** — it stays
    `package theme` internal (calls unexported `readWalColors`/`walToThemeTOML`)
    and the **2 `TestWalCachePath_*` tests** (white-box `walCachePath`, carved
    out of the old `theme_wal_subcmd_test.go`) were folded into it. *Refines the
    plan's test-split hint:* the hint guessed `theme_wal_subcmd_test.go` would go
    fully internal, but its 5 `generate-from-wal` command tests are black-box and
    belong external; only its 2 `walCachePath` tests are white-box. Net: 4 test
    files → 4 test files (3 external + 1 internal), with the subcmd file's tests
    redistributed by access need (the same internal/external split price's
    PS-007 used). All 24 theme test functions preserved 1:1 (3+7+5+9).
  - VERIFIED: `go fix ./internal/cli/...`, `go build ./...`, `go vet
    ./internal/cli/...`, `gofmt -l internal/cli/` all clean; `go test ./...` green
    (5598 passed in 41 packages — the PS-013 baseline of 5598, now in 41 pkgs
    since `theme` is its own test package; coverage 1:1, all 24 theme test
    functions preserved — theme 3, list 7, generate-from-wal 5, wal 9 incl. the 2
    moved `walCachePath` tests); `golangci-lint run ./internal/cli/...` clean. An
    adversarial 3-lens review (behavior-equivalence / coverage-1:1 / spec-conformance+
    cycle-safety) returned **pass** on all three with no blockers/majors/minors:
    `go list -deps` confirms the production `internal/cli/theme` does not import
    the root `internal/cli` and the internal `package theme` test imports zero
    `internal/cli` (R2/D5 cycle-free), source diffs are mechanical only
    (`walToThemeTOML`'s TOML body + `readWalColors`'s error strings byte-identical),
    and no residual `package cli` code references `newThemeCmd`/`ReadWalColors`.
  - **This completes the noun extractions (all 11 nouns moved).** Remaining:
    PS-015 (delete shims), PS-016 (tidy `root.go` / cruft), PS-017 (docs).

## Phase 6: Final cleanup

- [ ] **PS-015 — Delete shims**
  - GREEN: remove the package-`cli` shims `openServices`,
    `autoBackupAfterModification`, `executeWith`, and the `testutil_test.go`
    fixture wrappers — every caller now references `cmdutil.*` / `cli.ExecuteWith`
    / `clitest.*` directly. Delete `testutil_test.go`. Build + full suite green.
  - NOTE: the `formatMoney` shim was **already removed in PS-013** (it became
    orphaned the moment report's printers — the last in `format.go` — moved out, and
    `unused` would have failed lint). Only the three remaining shims are left here.

- [ ] **PS-016 — Tidy `root.go`, remove cruft** (`format.go` already deleted in PS-013)
  - GREEN: the residual `format.go` was **already deleted in PS-013** (it held only
    the report printers + the `formatMoney` shim; `roothelp.go`'s `printHelp`/
    `printVersion` remain residual). Verify
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
