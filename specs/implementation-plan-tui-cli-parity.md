# Implementation Plan: TUI ↔ CLI Parity

Source: the 2026-07-20 parity audit (run after `investment list`/`edit`
shipped in `49733ed`). Every gap below is CLI-surface work — the domain
service methods already exist (`transaction.Service.Update/Delete`,
`scheduled.Service.Update/Delete` + `SetTransfer`,
`category.Service.Create/Update/Delete/MergeCategories`,
`account.Service.Update/Delete`) because the TUI already calls them.

Each phase is independently shippable, TDD'd (tests written with the
phase), gated on `go build ./... && go test ./...` + `gofmt` +
`golangci-lint`, docs (`specs/cli.md` + README) updated in the same
commit, and committed directly to `main` before the next phase starts.

House conventions that apply throughout (established by `transfer edit`,
`security edit`, and `investment edit`):

- Edit commands take `--txn-id`/`--id` plus delta-style flags — only
  supplied flags take effect; at least one editable flag required;
  `cmd.Flags().Changed()` distinguishes set-to-empty from unset.
- Reconciled rows are refused (`tmoney reconcile` owns that state);
  transfer legs are routed to `transfer edit`.
- Destructive commands preview by default where practical and/or demand
  an explicit ID; every mutation ends with
  `cmdutil.AutoBackupAfterModification`.
- ID discovery comes from the matching `list` command's `--show-ids`.

## Status Legend

- `[ ]` not started
- `[~]` in progress
- `[x]` done

## Priority Rationale

Transaction edit/delete first: it is the regular-register analog of the
just-shipped `investment edit`, the most-missed gap, and it establishes
the `--status` flag shape that Phase 2 finishes. The cleared-status
toggle rides the same dispatch code. Scheduled edit/delete/transfers
next — same pattern on a smaller surface. Splits and the category noun
are the two genuinely new CLI surfaces, so they land mid-plan once the
conventions are settled; account edit/delete and the corporate-action
listing are small and independent; the by-design exclusions close the
plan as a docs-only phase.

## Phase 1: `transaction edit` + `transaction delete` — [ ]

The regular-register analog of `investment edit`. Edits dispatch to
`transaction.Service.Update` (`internal/transaction/transaction_service.go:134`);
delete to `Service.Delete` (`:211`).

- [ ] `internal/cli/transaction/edit.go`: `transaction edit --txn-id <uuid>`
      with delta flags `--amount`, `--date`, `--payee`, `--category`,
      `--memo` (explicit `""` clears), `--status` (`cleared`/`uncleared`;
      `reconciled` rejected with a pointer to `tmoney reconcile`).
      Refuse: transfer legs (→ `transfer edit`), split parents
      (→ TUI, until Phase 4), void and reconciled rows. Payee
      auto-creates via `payeeSvc.GetOrCreate` (same as `transaction add`);
      category must exist (same as `transaction add`).
- [ ] `internal/cli/transaction/delete.go`: `transaction delete <txn-id>`
      (positional, matching `transaction void <id>`). Refuse transfer
      legs (→ `transfer delete`), split parents (until Phase 4), and
      reconciled rows. Print what was deleted.
- [ ] Register both in `internal/cli/transaction/transaction.go`.
- [ ] Tests: flag validation, each editable field, clears-vs-unset memo,
      refusal cases (transfer leg, split parent, reconciled, void),
      delete happy path + refusals, account balance reflects both.
- [ ] Docs: `specs/cli.md` sections (alphabetical: delete before list,
      edit after delete), README register-scripting examples,
      `specs/transaction-status.md` note about the new `--status` flag
      (replaces the "no CLI toggle" caveat added in `c8666c9`).

## Phase 2: Cleared-status toggle everywhere — [ ]

Phase 1's `transaction edit --status` covers the regular register. This
phase finishes the investment side so both registers' `c`-key behavior
is scriptable.

- [ ] `internal/cli/investment/edit.go`: add `--status`
      (`cleared`/`pending`; `reconciled` rejected) routed through
      `investment.Service.SetClearedStatus`. Replaces the current
      implicit carry-over-only behavior; carry-over remains the default
      when the flag is absent.
- [ ] Tests: set cleared, set back to pending, reconciled rejected,
      default carry-over still works.
- [ ] Docs: `specs/cli.md` `investment edit` editable-flags list, README.

## Phase 3: `scheduled edit` + `scheduled delete` + scheduled transfers — [ ]

`scheduled.Service.Update` (`internal/scheduled/scheduled_service.go:143`),
`Delete` (`:178`), and `Transaction.SetTransfer`
(`internal/scheduled/scheduled.go:225`) all exist.

- [ ] `internal/cli/scheduled/edit.go`: `scheduled edit --id <uuid>` with
      delta flags `--name`, `--amount`, `--payee`, `--category`,
      `--frequency`, `--next-date`, `--account`, `--memo`, `--auto-post`.
      Refuse multi-line (split/paycheck) templates with a pointer to the
      TUI (their line editing is Phase 4+ scope at the earliest).
- [ ] `internal/cli/scheduled/delete.go`: `scheduled delete <id>`
      (positional). Template only — posted history stays.
- [ ] `internal/cli/scheduled/add.go`: add `--transfer-to <account>`
      (mutually exclusive with `--payee`/`--category` per
      `SetTransfer`'s single-line constraint) so scheduled transfers can
      be created from the CLI.
- [ ] `scheduled list`: add `--show-ids` for discovery (matching the
      other list commands).
- [ ] Tests: edit each field, frequency validation reuses the existing
      parser, delete, transfer creation + posting produces a linked
      pair, multi-line refusal.
- [ ] Docs: `specs/cli.md` (edit/delete sections + add's new flag +
      list's `--show-ids`), README, `specs/scheduled-transactions.md`
      CLI notes.

## Phase 4: Split transactions from the CLI — [ ]

New surface. Entry: repeated `--split "Category=amount[:memo]"` flags on
`transaction add`; editing/deleting split lines stays TUI-only this
phase (the `ReplaceSplits` calling convention is dialog-shaped), but the
refusals from Phase 1 get pointers here.

- [ ] `internal/cli/transaction/add.go`: repeated `--split` flag
      (parse `Category=amount` with optional `:memo`; amounts must sum
      to `--amount`, or `--amount` may be omitted and derived). Transfer
      lines (`Transfer:<Account>=amount`) follow the split dialog's
      semantics via the transfer-aware `ReplaceSplits` path.
- [ ] `transaction list`/`search`: render split parents with their line
      count (e.g. `[3 splits]`) so scripted output shows them.
- [ ] Decision (record here before implementing): whether a
      `transaction edit --split` replace-all-lines mode is worth it in
      the same phase, or whether split editing stays TUI-only. Default:
      TUI-only; revisit after real usage.
- [ ] Paycheck wizard: **out of CLI scope** — it is a guided template
      builder over the same split machinery. Once `--split` lands, a
      paycheck schedule is expressible via `scheduled add --split …` if
      Phase 3+4 are both done; note that in the docs instead of building
      a wizard.
- [ ] Tests: sum validation, derived total, transfer line, list
      rendering, posting a scheduled split unchanged.
- [ ] Docs: `specs/cli.md` `transaction add`, README,
      `specs/multiline-splits-and-paycheck.md` CLI notes.

## Phase 5: `category` noun — [ ]

No `category` command exists at all today
(`internal/cli/root.go` registers no category noun). Service support:
`Create`/`Update`/`Delete`/`MergeCategories`/`List*`
(`internal/category/category_service.go`).

- [ ] New package `internal/cli/category/` with:
  - [ ] `category add --name X [--parent Y] [--type income|expense]`
  - [ ] `category list [--type …] [--show-ids]` (tree-indented like the
        TUI combo box)
  - [ ] `category rename --id/--name … --to …` (via `Service.Update`)
  - [ ] `category delete <id-or-name>` — refuse when transactions
        reference it, matching `Service.Delete`'s guard; suggest merge
  - [ ] `category merge --from X --to Y` (via `MergeCategories`)
- [ ] Register the noun in `internal/cli/root.go`.
- [ ] System categories (transfer categories etc.) are refused for
      rename/delete/merge — reuse the existing system-category guard.
- [ ] Tests per subcommand incl. guards.
- [ ] Docs: new `## category` section in `specs/cli.md` (alphabetical,
      after `account`), README, and update `specs/categories.md` —
      its Edit/Delete/Merge operations section finally has an
      implementing surface; note TUI remains create-only inline.

## Phase 6: `account edit` + `account delete` — [ ]

`account.Service.Update` (`internal/account/account_service.go:50`) and
`Delete` (`:58`) exist; the TUI account dialog and
`MenuActionDeleteAccount` already use them.

- [ ] `internal/cli/account/edit.go`: `account edit --name X` plus delta
      flags `--new-name`, `--interest-rate`, `--opening-balance`,
      `--opening-date`, `--memo`… (match the TUI dialog's editable
      field set exactly — enumerate from `account_dialog.go` when
      implementing). Type changes follow the TUI's rules (`--type` only
      where the dialog allows it; lot-tracking stays with
      `investment enable-lots`/`disable-lots` per the existing spec
      note at `specs/cli.md` account section).
- [ ] `internal/cli/account/delete.go`: `account delete --name X`
      with an explicit `--confirm` (it cascades transactions — mirror
      whatever confirmation the TUI shows). Suggest `account close` in
      the help text as the usually-better option.
- [ ] Tests: rename reflected in `account list`/`GetByName`, guard
      cases, delete with/without `--confirm`.
- [ ] Docs: `specs/cli.md` (edit after close, delete after…
      alphabetical: add/balance/close/delete/edit/list/reopen/show),
      README, `specs/accounts.md` CLI notes.

## Phase 7: `investment actions` (corporate-action history) — [ ]

Read-only listing of recorded splits/mergers/spin-offs — the CLI
counterpart of the TUI's corporate-action history view
(`internal/tui/corporate_action_history.go`), and the discovery surface
for the existing mutating commands.

- [ ] `internal/cli/investment/actions.go`: `investment actions`
      `[--ticker X] [--type split|merger|spin_off] [--show-ids]`,
      newest-first, columns matching the TUI view (date, type,
      securities, ratio/terms).
- [ ] Register in `internal/cli/investment/investment.go`.
- [ ] Tests: seeded split + merger listed, filters, empty case.
- [ ] Docs: `specs/cli.md` (alphabetical: actions right after the
      `investment` intro, before buy), README.

## Phase 8: Document the by-design exclusions — [ ]

Docs-only. Undo/redo is already spec'd as TUI-only
(`specs/undo-redo.md`: "No CLI undo"); the price chart has no such
statement.

- [ ] `specs/cli.md`: short "Deliberately TUI-only" note near the top
      (undo/redo, price chart, paycheck wizard, split-line editing —
      whichever of the Phase 4 decisions stand) so future parity audits
      don't re-flag them.
- [ ] `specs/prices-chart.md`: one line stating the chart is TUI-only;
      `price list` is the CLI's view of the same data.
- [ ] README: mirror the note where the TUI/CLI split is introduced.

## Done

(move phases here as they complete, with commit hashes)
