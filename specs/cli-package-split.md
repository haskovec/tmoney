# CLI Package Split Specification

> **Status (2026-05-31): ✅ COMPLETE.** All 11 nouns are extracted into
> `internal/cli/<noun>` packages alongside the shared `cmdutil` hub and the
> cli-free `clitest` fixtures; the package-`cli` shims were removed and the
> monolithic `format.go` (plus the dead pre-Cobra `roothelp.go`) deleted. The
> split landed incrementally across PS-001 → PS-017 — see
> [`implementation-plan-cli-package-split.md`](implementation-plan-cli-package-split.md)
> for the per-PR record. The text below describes the realized layout.
>
> This spec describes the split of the single, large `internal/cli` package
> into per-noun subpackages
> plus a shared `cmdutil` hub and a `clitest` test-support package, mirroring
> the horizontal extraction already done in `internal/tui` (`widget/`,
> `dialog/`, `theme/`). The current CLI behavior/reference live in
> [`cli.md`](cli.md); the Cobra router design is in
> [`cli-router.md`](cli-router.md).
>
> **The per-PR checklist lives in
> [`implementation-plan-cli-package-split.md`](implementation-plan-cli-package-split.md).**
> A fan-out inventory + completeness audit (0 orphaned files; 73/73 source,
> 64/64 test accounted for) refined the test architecture and surfaced a
> pervasive package-name collision. Those refinements (R1–R4) are folded into
> this spec below and are authoritative; the implementation plan carries the
> same refinements as numbered design notes.

## Overview

`internal/cli` currently holds **137 files in one package** (73 source +
64 test). Every command is a self-contained Cobra leaf assembled by
`internal/cli/root.go`. The package is well-named by noun
(`account_*.go`, `investment_*.go`, …) but the sheer file count hurts
navigability, and there is no compiler-enforced boundary stopping one
command group from reaching into another's internals.

This spec splits the package into **one subpackage per noun** plus a
small shared **`cmdutil`** hub and a **`clitest`** test-support package.
The result shrinks the top-level folder to a handful of files,
distributes the 32 KB `format.go` god-file along noun lines, and turns
the "commands don't reach into each other" invariant into a hard,
compiler-enforced fact.

### Why this is safe

A dependency trace established that the coupling in `internal/cli` is a
**star, not a web**:

- **No command-to-command dependencies.** The `account` commands never
  call `investment` command code, etc. Every command depends on exactly
  two things: the shared CLI helpers, and the `app.Services` /domain
  packages.
- **`internal/cli` is a leaf.** Only `main.go` imports it, so there is
  zero risk of an *external* import cycle.
- **`transfer_resolve.go` only looks cross-cutting.** It fans out through
  `app.Services` (transaction + investment + account *repos*), never
  through other CLI commands, and is used solely by `transfer
  edit`/`delete`.

A star topology is the easy case: extract the hub, and the spokes become
independent. The one real friction the inventory found is mechanical, not
structural — the CLI subpackages collide by name with their domain packages
(R1), resolved by import aliases.

## Goals

- Shrink the top-level `internal/cli` folder to root + single-verb
  commands.
- Give each noun its own package with a compiler-enforced boundary.
- Dissolve `format.go` into the nouns that actually use each function.
- Preserve 100% of the existing (integration-style) test coverage.
- Match the in-repo precedent set by the `internal/tui` split (shared
  leaves nested under the parent; parent imports down into them).

## Non-goals

- **Per-command unit-test rewrite.** The 1144 `executeWith` integration
  tests are kept as-is (relocated + repointed, not rewritten). We are not
  converting them to construct subcommands in isolation.
- **Splitting single-verb commands into packages.** `version`, `import`,
  `export` stay in the top-level package.
- **Changing any CLI behavior, flags, output, or exit codes.** This is a
  pure reorganization.
- **Touching `internal/tui` or any domain package.**

## Design decisions

| # | Decision | Rationale |
|---|---|---|
| D1 | Per-noun packages **and** a shared hub (go further than `tui`'s horizontal-only split). | CLI commands share no mutable state (unlike `tui` views, which share one Bubbletea model), so per-noun vertical slices are clean here when per-view was not in `tui`. |
| D2 | Hub is its own leaf package, **not** the top-level `cli` package. | Top-level `cli` imports every noun to assemble root; if nouns imported `cli` back for `formatMoney`, that is a cycle. Same reason `tui` put `widget/` as a leaf. |
| D3 | Hub named `cmdutil`, nested at `internal/cli/cmdutil`. | `gh`/ecosystem standard for "shared command infrastructure"; nesting matches `tui/widget`. The hub mixes formatting + wiring, so a single role-name (`clifmt`) would be inaccurate. |
| D4 | Black-box tests move into the noun folders as external `package <noun>_test`, driving the full CLI via the **exported `cli.ExecuteWith`** + `clitest` fixtures. | External `_test` packages may import packages that import the package under test, so importing `cli` from `account_test` is cycle-free. |
| D5 | `internal/cli/clitest` holds **fixtures only** and **must not import `cli`**. | The 6 white-box test files (R3) stay `package <noun>` internal and need the fixtures; if `clitest` imported `cli` they would cycle (`<noun>(test) → clitest → cli → <noun>`). Keeping `clitest` cli-free makes it usable by both internal and external tests. |
| D6 | Incremental rollout, one noun per PR; ordered "most important first" (foundation → pilot → giants → standard → small → cleanup), per the implementation plan. | 137 files in motion; small, green, bisectable PRs minimize regression risk and merge conflicts. (This reorders the earlier "investment last" idea — the giant moves *early*, while the pattern is fresh.) |
| D7 | CLI subpackages keep the `internal/cli/<noun>` name; the colliding domain import is aliased `<noun>dom` (`tuitheme` for theme). | Preserves the approved folder shape. Alternative — naming packages `<noun>cmd` to avoid aliasing — is noted in R1 but not chosen. |

## Target layout

```
internal/cli/
  root.go  tui.go            # root + TUI launch; exports ExecuteWith + SwapTUILauncher
  roothelp.go                # printHelp + printVersion (carved out of format.go)
  version.go  import.go  export.go   # single-verb commands stay top-level
  cmdutil/          # shared hub (the widget/ analog)
    format.go       # FormatMoney
    services.go     # OpenServices, AutoBackupAfterModification, RequireFile
  clitest/          # fixtures ONLY (regular .go, imports testing + domain pkgs; NOT cli)
    fixtures.go     # CreateInvestmentTestDB, CreateTestDBWithSecurity, PtrMoney, ...
  account/    db/    transaction/    transfer/    scheduled/
  reconcile/  security/  price/  investment/  report/  theme/
```

11 noun packages + `cmdutil` + `clitest`. Dependency direction:

```
main.go → cli (root) → {account, db, …, investment, theme} → cmdutil
                     ↘ cmdutil
clitest → {db, app, account, security, price, types}   # domain only, NOT cli
<noun>_test (external) → cli (ExecuteWith, SwapTUILauncher) + clitest (fixtures)
<noun>   (internal white-box test) → clitest + cmdutil  # never cli
```

All production edges point one way; `clitest` is a cli-free leaf so both
internal and external tests can use it. Acyclic by construction.

## The `cmdutil` hub — exact inventory

The hub is **tiny**, because the trace proved `format.go` is noun-local
rendering pooled into one file, not shared infrastructure. The hub holds
only what is genuinely cross-noun:

| Symbol | Source today | Exported as | Notes |
|---|---|---|---|
| `formatMoney` | `format.go` | `cmdutil.FormatMoney` | The one formatter used by 5+ nouns directly *and* by every `print*` helper. |
| `openServices` | `helpers.go` | `cmdutil.OpenServices` | Opens DB, builds `app.Services`, updates recent files, auto-posts due schedules (keep the os.Stdout side effect). Called by 49 command files. |
| `autoBackupAfterModification` | `helpers.go` | `cmdutil.AutoBackupAfterModification` | Best-effort auto-backup after data-modifying commands. Called by 33 command files. |
| `RequireFile(file)` | *(new)* | `cmdutil.RequireFile` | Folds the `--file is required to specify a database` guard. **51 identical sites** collapse into it. `export.go` uses a divergent message (`--export requires --file`) — give `RequireFile` an optional message or keep export's bespoke guard. `db_create.go` has no guard (it creates, not opens) — don't add one. |

## `format.go` distribution

`format.go` does **not** move atomically. `formatMoney` → `cmdutil`;
`printHelp`/`printVersion` → carved out to a residual `roothelp.go` (printVersion
reads the residual `Version`/`BuildTime`/`GitCommit` vars); the remaining ~20
`print*` functions go *home* to the single noun that uses each (becoming
unexported there). This table was **verified by the inventory**:

| Function | Destination package | Caller |
|---|---|---|
| `printAccountsTable` | `account` | `account_list.go` |
| `printAccountDetails` | `account` | `account_show.go` |
| `printBalancesTable` | `account` | `account_balance.go` |
| `printTransactionsTable` | `transaction` | `transaction_list.go` |
| `printSearchResults` | `transaction` | `transaction_search.go` |
| `printScheduledTransactionsTable` | `scheduled` | `scheduled_list.go` |
| `printReconcileStatus` | `reconcile` | `reconcile_status.go` |
| `printSecuritiesTable` | `security` | `security_list.go` |
| `printSecurityDetails` | `security` | `security_show.go` |
| `printPricesTable` | `price` | `price_list.go` |
| `printNetWorthReport` | `report` | `report_net_worth.go` |
| `printSpendingReport` | `report` | `report_spending.go` |
| `printPortfolioSummary` | `investment` | `investment_portfolio.go` |
| `printPortfolioWithLots` | `investment` | `investment_portfolio.go` |
| `printAccountTotals` | `investment` | internal to portfolio printers |
| `printClosedPositions` | `investment` | internal to portfolio printers |
| `partitionHoldings` | `investment` | internal to portfolio printers |
| `formatGainLoss` | `investment` | internal to portfolio printers |
| `formatReturnPct` | `investment` | internal to portfolio printers |
| `formatFeesPaid` | `investment` | internal to portfolio printers |
| `printHelp` | residual `roothelp.go` | root help |
| `printVersion` | residual `roothelp.go` | `version.go` |

Non-`format.go` symbols/files that also relocate with their noun:

| Symbol / file | Destination | Notes |
|---|---|---|
| `parseYearMonth` | `report` | defined in `report_spending.go`; only `report_spending.go` uses it |
| `parseEditStatus` | `transfer` | defined in `transfer_edit.go` (not `transfer_resolve.go`); only `transfer_edit.go` uses it |
| `transfer_resolve.go` (whole file: `resolveTransferPair`, `resolveFromRegularLeg`, `resolveFromInvestmentLeg`, `findInvestmentLeg`, `refuseIfMultiLineSplit`, `investmentStatusToRegular`, `isNotFound`, `resolvedTransfer`, `errTransferLineSplit`) | `transfer` | used only by `transfer edit`/`delete` |
| `wal.go` (whole file: `walToThemeTOML`, `ReadWalColors`/`WalColors`/`WalSpecial`/`WalColorTable`, `walCachePath`, `activeThemeIDFn`) | `theme` | generation/parse logic; consider unexporting `ReadWalColors` et al. (no cross-pkg callers) |
| `registerPriceProviders` (hook var), `printRefreshResult`, `displayOutcome` | `price` | already in `price_update.go`; travel with the file |
| `printLinkTransferPreview`, `writeCandidateTable` | `transfer` | already in `transfer_link.go`; travel with the file |

## Per-package conventions

- **R1 — domain-package name collision (affects 9 of 11 nouns).**
  `internal/cli/<noun>` collides with the same-named domain package
  `internal/<noun>`. Every moved source **and** test file importing its own
  domain package must alias it: convention `<noun>dom` (`accountdom`,
  `transactiondom`, `scheduleddom`, `securitydom`, `pricedom`,
  `investmentdom`, `reportdom`, `dbpkg`), and `tuitheme` for
  `internal/tui/theme`. **No collision** for `transfer` (no `internal/transfer`)
  or `reconcile` (domain is `internal/reconciliation`).
  *Alternative not chosen:* name the packages `<noun>cmd` to avoid all aliasing.
- **One exported constructor per noun:** `account.NewCmd()`, `db.NewCmd()`, …
  The package qualifier removes the stutter (no `NewAccountCmd`). `root.go`
  calls `account.NewCmd()` in place of `newAccountCmd()`. Verb constructors
  move into the noun package and stay unexported.
- **Drop the redundant filename prefix:** `account_add.go` → `account/add.go`,
  `investment_buy.go` → `investment/buy.go`, etc.
- **The persistent `--file` flag is unchanged.** It stays registered on the
  root command in `cli`; subcommands read it with `cmd.Flags().GetString("file")`.
  Cobra resolves inherited flags through the runtime command tree, not via
  compile-time package coupling, so command bodies need no change.

## Test architecture (R2 + R3)

The 1144 `executeWith` calls are integration-style (drive the full root by
argv). Two test entry points, chosen by whether a test needs unexported access:

- **Black-box tests → external `package <noun>_test`.** They import `cli` for
  the **exported `ExecuteWith(args []string, stdout, stderr io.Writer) error`**
  (rename of the unexported `executeWith`) and the **exported
  `SwapTUILauncher(fn) (restore func())`** seam (replacing the `stubLaunchers`
  test helper, used by `*_Help` tests in db/price/investment/theme/report), plus
  `clitest` for fixtures. Cycle-free: an external `_test` package may import a
  package that imports the package under test.
- **`internal/cli/clitest` — fixtures only, cli-free.** Holds
  `CreateTestDBWithSecurity`, `CreateTestDBWithSecurityAndPrices`,
  `CreateInvestmentTestDB`, `CreateCorporateActionTestDB`, `PtrMoney`
  (exact current signatures preserved; create→close→return-path pattern intact).
  Imports only domain packages + `testing` (the `httptest`/`testing.T`-helper
  pattern). **Never imports `cli`**, so internal white-box tests can use it too.
- **White-box tests → internal `package <noun>` (R3).** Six test files poke
  unexported symbols and cannot be external; split each so only the white-box
  functions are internal, the rest external:

  | File | Unexported symbol(s) | Owner |
  |---|---|---|
  | `transfer_resolve_test.go` | `resolveTransferPair`, `resolvedTransfer` fields, `*errTransferLineSplit` | transfer |
  | `price_update_test.go` | mutates `registerPriceProviders` | price |
  | `wal_test.go` | `walToThemeTOML`, `ReadWalColors` | theme |
  | `theme_wal_subcmd_test.go` | `walCachePath` | theme |
  | `report_spending_test.go` | `parseYearMonth` (`TestParseYearMonth` only) | report |
  | `format_test.go` | `printHelp`/`printVersion` (→ residual), `formatMoney` (→ cmdutil) | residual + cmdutil |

  Internal white-box tests do **not** use `ExecuteWith` (that would cycle). They
  call the unexported function directly; where one must run a command (e.g.
  `price_update_test`), it builds a **partial** root inline (a throwaway
  `*cobra.Command` with the persistent `--file` flag + the noun's own
  `NewCmd()`), importing only cobra + its own package + `clitest`/`cmdutil`.
- **Test-local helpers travel with their files** (not in `clitest`): e.g.
  transfer's `setupTransferAccounts`/`setupTransferDispatchAccounts`/`openSvc`/
  `findInvestmentLegForTest`; investment's `createPortfolioCmdTestDB*`; theme's
  `writeUserTheme`/`writeConfigTheme`/`installSampleWalCache`. A helper shared by
  both an internal and an external test file in the same noun (e.g.
  `findInvestmentLegForTest`) is duplicated or hoisted into `clitest`.
- **`workflow_test.go` stays residual** — it's a cross-noun end-to-end test
  driving reconcile/investment/etc. via `ExecuteWith`; it keeps its own access
  to fixtures (`clitest`) and the root.

## What won't break (verified)

- **Star topology** — no command calls another command's code.
- **`cli` is a leaf** — only `main.go` imports it; no external cycle.
- **`transfer_resolve.go`** fans out only through `app.Services`.
- **Inherited `--file` flag** works unchanged across package boundaries.
- **Package-name collisions (R1)** are a *compile-time* concern resolved by
  import aliases — not a runtime break and not a structural blocker.

## Migration plan

The incremental, priority-ordered per-PR checklist (foundation → pilot →
giants → standard nouns → small → cleanup) lives in
[`implementation-plan-cli-package-split.md`](implementation-plan-cli-package-split.md).
Shape: Phase 1 lands `cmdutil` + `clitest` + the exported harness (with
package-`cli` shims so unmoved code compiles); each noun PR then moves source +
tests + printers, aliases the domain import, exports `NewCmd`, and rewires
`root.go`; the final phase deletes shims, collapses `format.go`, and tidies
`root.go`. Every PR keeps `go test ./...` green.

## Gotchas

- **`testdata/` is per-package in Go** (test working dir = package dir).
  Only `theme_wal_subcmd_test.go` and `wal_test.go` read
  `testdata/wal-sample-colors.json`, so it moves into `theme/testdata/`
  and no other noun needs on-disk fixtures.
- **Stray `price.tdb` / `security.tdb` (1 MB each)** sit in the package dir but
  are **untracked and gitignored** — leftovers from manual `tmoney --file …`
  runs, **not** produced by tests (the inventory confirmed every test uses
  `t.TempDir()`). They're safe to delete; just ensure the moves don't
  accidentally `git add` them.
- **`stubLaunchers` ↔ `tuiLauncher` coupling.** `stubLaunchers` (root_test.go)
  mutates the unexported `tuiLauncher` var; it's used by `*_Help` tests across
  db/price/investment/theme/report. The exported `SwapTUILauncher` seam (R2)
  replaces it so those help tests can live in their external `_test` packages.
- **Stale legacy help text.** `printHelp` still hardcodes pre-Cobra `--flag`
  descriptions (`--create`, `--backup`, …). Out of scope for the split; flagged
  for a later cleanup. (Same class as the `cmd/tmoney/some-file.tdb` note in
  [`cli-router.md`](cli-router.md).)
```
