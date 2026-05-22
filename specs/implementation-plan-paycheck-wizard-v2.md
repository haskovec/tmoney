# Implementation Plan: Paycheck Wizard v2

This document defines the order in which the v2 paycheck wizard redesign
(`specs/multiline-splits-and-paycheck.md`, "Paycheck Wizard" section)
should be implemented. Each item is one small session of work following
a red-green (test-first) pattern. Mark items as complete with `[x]` as
they are finished.

Spec:

- `specs/multiline-splits-and-paycheck.md` — feature spec (v2 wizard
  section)
- `specs/tui.md` — TUI layout reference (v2 wizard mockup)
- `specs/implementation-plan-multiline-splits-and-paycheck.md` —
  v1 plan; this v2 plan extends the work shipped there

## Status Legend

- `[ ]` Not started
- `[x]` Complete

## Priority Rationale

The plan is ordered so each phase is a coherent shippable unit:

1. **Data model first** (Phase 1). Add the nullable `paycheck_section`
   column and the repo plumbing. After Phase 1, the data model supports
   tagging but nothing writes the tag — the wizard still works as v1
   would.
2. **Default category seeds** (Phase 2). Standalone — `Income:Bonus`
   and `Income:Retro Pay` become available for the post-time preview.
3. **Wizard layout rewrite** (Phase 3). Replace the v1 form with the
   v2 five-section structure (Earnings / Pre-tax / Taxes / Post-tax /
   Net Pay Destinations), multi-line earnings, new pre-population,
   `[+ Add line]` affordances, and $0 row elision. Wizard starts
   tagging saved lines with `paycheck_section`.
4. **Round-trip via tags** (Phase 4). `Edit as paycheck →` switches
   from the v1 category-pattern heuristic to reading the
   `paycheck_section` tag, so reopened paychecks land back in their
   original sections.
5. **Smoke check** (Phase 5). Final manual verification.

The legacy v1 wizard implementation is replaced in place. Per the
spec, there are no v1 paycheck schedules to migrate (the only user
hasn't been happy with v1 and hasn't relied on it) — `paycheck_section`
is nullable so any pre-v2 multi-line schedules continue to load via
the generic dialog with the Edit-as-paycheck affordance hidden.

Inside each phase, the data-model and repository items come before
service items, which come before TUI items — same red-green ordering
used by the v1 plan.

---

## Phase 1: Data Model — `paycheck_section` Column

Add the nullable enum column to both split tables and wire it through
the repository layer.

- [x] **PW2-001 — Add `paycheck_section` column to `split_items` and `scheduled_split_items`**
  - RED: test asserts the new migration (next number, e.g.
    `017_paycheck_section.sql`) adds a nullable enum column
    `paycheck_section` to both tables, with the value set restricted
    to `'earnings' | 'pre_tax' | 'tax' | 'post_tax' | 'net_pay_destination'`.
    Existing rows have NULL after migration; the constraint allows
    NULL.
  - GREEN: add the migration in `internal/db/migrations/`. Update
    `internal/transaction/split_item.go` and
    `internal/scheduled/split_item.go` so each `Split` struct exposes
    a `PaycheckSection` field (string or typed enum, nullable —
    follow the existing pattern for nullable string-y columns; e.g.
    `*string` or a sentinel zero value). Bump `CurrentSchemaVersion`.
  - Confirm: `go build ./... && go test ./internal/db/... ./internal/transaction/... ./internal/scheduled/...` green.
  - Done: shipped as migration `020_paycheck_section.sql` (next
    available number after 019); `PaycheckSection` lives on
    `transaction.Split` (`internal/transaction/transaction.go`) and
    `scheduled.Split` (`internal/scheduled/split_item.go`) as
    `types.NullableString`. Repo plumbing is task PW2-002.

- [x] **PW2-002 — Split repos read/write `paycheck_section`**
  - RED: test `TestSplitItemRepo_PaycheckSection_RoundTrip` — create a
    split with `PaycheckSection = "earnings"`; reload via the repo;
    field comes back set. Test `TestSplitItemRepo_NullPaycheckSection_RoundTrip`
    — create a split without setting the field; reload; comes back
    NULL/empty. Repeat both for `scheduled_split_items`.
  - GREEN: extend `internal/transaction/split_repository.go` and
    `internal/scheduled/split_repository.go` so `Create` / `Update`
    write the column and `scanSplits` / `GetByID` /
    `ListByTransaction` / `ListByScheduledTransaction` read it.
    The generic split dialog and the multi-line scheduled dialog do
    *not* set `PaycheckSection` — they pass through whatever the
    caller supplied (usually empty/NULL).
  - Confirm: tests green; full suite green.
  - Done: both repos now write `paycheck_section` via
    `dbutil.NullString` on `Create` and `Update`, and `scanSplits`
    reads the column on `GetByID` / `ListByTransaction` /
    `ListByScheduledTransaction`. The scheduled parent loader
    (`Repository.loadSplits`) inherits the tag transparently because
    it delegates to `SplitRepository.ListByScheduledTransaction`.

## Phase 2: Default Category Seeds

- [x] **PW2-003 — Seed `Income:Bonus` and `Income:Retro Pay`**
  - RED: test `TestFileInit_BonusAndRetroPayCategoriesExist` — opening
    or creating a database ensures `Income:Bonus` and `Income:Retro Pay`
    exist as subcategories of `Income`. Test that the existing
    `Income:Salary` seed is unaffected and that the existing tax /
    insurance seeds are unaffected.
  - GREEN: extend `category.PaycheckCategories` in
    `internal/category/category_service.go` to include the two new
    pairs (income type). The existing `EnsurePaycheckCategories()`
    invocation in `app.NewServices` walks the list and creates any
    missing pairs idempotently — no new caller needed.
  - Confirm: tests green; manual smoke not required (covered by the
    automated test).
  - Done: added `{Income, Bonus}` and `{Income, Retro Pay}` to
    `PaycheckCategories` in
    `internal/category/category_service.go`. Verified by
    `TestFileInit_BonusAndRetroPayCategoriesExist` in
    `internal/category/category_service_test.go` (also re-checks the
    existing `Income:Salary`, `Tax:*`, and `Insurance:Health` seeds).

## Phase 3: Wizard Layout v2

Replace the v1 wizard form with the v2 five-section structure. Each
slice rewrites part of the form in place; integration points
(`Transactions → New Paycheck Schedule…` menu, save dispatch via
`scheduled.Service`) are preserved.

- [x] **PW2-004 — Restructure wizard to v2 layout with new pre-population**
  - RED: test `TestPaycheckWizard_V2Layout_OpensWithSpecPrePopulation`
    — invoking the wizard opens a form whose structure matches the
    v2 spec:
    - One Earnings row pre-populated with `Income:Salary`
    - Zero Pre-tax Deduction rows
    - Three Tax rows pre-populated with `Tax:Federal`, `Tax:Social Security`, `Tax:Medicare`
    - Zero Post-tax Deduction rows
    - Primary deposit picker only in Net Pay Destinations (no
      pre-populated additional transfers)
  - GREEN: rewrite the `PaycheckWizard` struct in
    `internal/tui/paycheck_wizard.go`:
    - Drop the scalar `gross` / `grossCategory` fields. Add a
      `earningsLines []*PaycheckLine` slice with one default-seeded
      row (`Income:Salary`, empty amount).
    - Drop the v1 hardcoded `preTaxLineSpecs` (Federal/State/SS/
      Medicare/401k) and `postTaxLineSpecs` (Health/HSA). Replace
      with empty `preTaxLines` / `postTaxLines` slices.
    - Add a new `taxLines []*PaycheckLine` slice with three
      default-seeded rows (`Tax:Federal`, `Tax:Social Security`,
      `Tax:Medicare`).
    - Constructor `NewPaycheckWizard(categoryOptions, categoryIDs, accounts)`
      seeds the four slices and the primary deposit picker per the
      table above.
    - Update accessors (`PreTaxLines() / TaxLines() / PostTaxLines() /
      EarningsLines() / AdditionalTransfers()`).
  - Confirm: existing `TestPaycheckWizard_OpensWithEmptyForm` is
    replaced or rewritten to match v2 expectations; new test passes;
    full suite green.
  - Done: added `PaycheckEarnings` and `PaycheckNetPayDestination`
    enum values, grew `sections` from `[3]` to `[5]`, and seeded
    Earnings (`Income > Salary`) and Tax (`Federal`, `Social
    Security`, `Medicare`) defaults in `NewPaycheckWizard` via a new
    `seedSection` helper. Added `EarningsLines()` and
    `AdditionalTransfers()` accessors; updated `Title()`,
    iteration bounds in `AddRow` / `RemoveRow` / `BuildSplits` /
    `computeTotal` / `collectFocusables` / `Render`. Per-section
    `addRowLabel()` matches the spec mockup wording (incl.
    `[+ Add transfer]` for Net Pay Destinations).
    `NewPaycheckWizardFromSchedule` now clears the pre-populated
    rows up front so the schedule's existing splits are the only
    content rendered (PW2-008 will replace the heuristic routing
    with `paycheck_section` tag dispatch). Replaced
    `TestPaycheckWizard_OpensWithEmptyForm` with
    `TestPaycheckWizard_V2Layout_OpensWithSpecPrePopulation`;
    fixture gained `Tax > Medicare`; AddRow/RemoveRow tests
    adjusted for pre-populated row counts.

- [x] **PW2-005 — `[+ Add line]` affordance for each section**
  - RED: test `TestPaycheckWizard_AddLine_AppendsRowToSection` — for
    each of the four mutable sections (Earnings, Pre-tax, Taxes,
    Post-tax), pressing the section's "+ Add" button appends a new
    row to that section's slice with an empty amount field and the
    section's default category (or first transfer-target for
    transfer-line-capable rows). Net Pay Destinations' existing
    `AddAdditionalTransfer` continues to work.
  - GREEN: add `AddEarningsLine() / AddPreTaxLine() / AddTaxLine() /
    AddPostTaxLine()` methods to `PaycheckWizard`. Each appends a
    `*PaycheckLine` with appropriate defaults: Earnings rows are
    categorized (default `Income:Salary` or first income category);
    Pre-tax / Taxes / Post-tax rows are categorized by default, with
    a per-row toggle to transfer-line (re-use the existing transfer
    routing). Render path adds `[+ Add ...]` rows at the end of each
    section.
  - Confirm: tests green; manual smoke: open wizard, click each Add
    button, verify rows appear.
  - Done: added five named helpers — `AddEarningsLine`,
    `AddPreTaxLine`, `AddTaxLine`, `AddPostTaxLine`, and
    `AddAdditionalTransfer` — alongside a private `addLineForSection`
    dispatcher used by the `wizardFocusAddRow` activation path in
    `activate`. `AddEarningsLine` seeds `Income > Salary`;
    `AddPreTaxLine` / `AddTaxLine` / `AddPostTaxLine` leave the row
    categorized at `(None)` so the user picks (and can flip to a
    transfer-line via the combined picker); `AddAdditionalTransfer`
    auto-targets the first account other than the current deposit
    account (falling back to index 0 when no alternative exists), so
    the new transfer row doesn't collide with the schedule's parent
    account. The `[+ Add …]` render path already existed from
    PW2-004; this commit only rewires its activation to the new
    helpers so each section's new row picks up the right defaults.
    Verified by new `TestPaycheckWizard_AddLine_AppendsRowToSection`
    in `internal/tui/paycheck_wizard_test.go`; the existing AddRow /
    BuildSplits tests still pass because the underlying `AddRow`
    primitive is unchanged.

- [x] **PW2-006 — BuildSplits tags lines with `paycheck_section`; $0 rows elided**
  - RED: test `TestPaycheckWizard_BuildSplits_TagsEachLine` — fill out
    a wizard with one row in each section; call `BuildSplits()`;
    every returned `*scheduled.Split` has `PaycheckSection` set to
    the matching enum value (`earnings` for the earnings row,
    `pre_tax` for the pre-tax row, etc.). The implicit primary
    deposit (the parent's account) does *not* get a split row — it's
    the parent transaction.
  - Test `TestPaycheckWizard_BuildSplits_ElidesZeroRows` — fill out
    the wizard with the three pre-populated tax rows but leave one at
    $0 (or empty); call `BuildSplits()`; only the non-zero tax rows
    are returned.
  - Test `TestPaycheckWizard_BuildSplits_BalanceInvariant` —
    `parent_amount == signed_sum(splits.amount)` holds across mixed
    sections.
  - GREEN: extend `BuildSplits()` in
    `internal/tui/paycheck_wizard.go` to (1) walk all four mutable
    section slices plus the additional-transfers slice and emit one
    split per non-empty / non-zero row, (2) set `PaycheckSection` on
    each split based on which section it came from, (3) return splits
    in section order (earnings → pre-tax → tax → post-tax →
    net-pay-destination) so the storage order is deterministic for
    the generic dialog's display, (4) skip rows whose amount field is
    empty or parses to zero. Categorized vs transfer-line shape per
    row is unchanged from v1.
  - Confirm: tests green.
  - Done: added a private `PaycheckSection.tagString()` mapping
    (`earnings` / `pre_tax` / `tax` / `post_tax` /
    `net_pay_destination`) that mirrors the CHECK constraint in
    migration 020, and stamped it onto each `*scheduled.Split`
    produced by `buildLineSplit` so every wizard-emitted row carries
    a non-NULL `PaycheckSection` for PW2-008 to read back. The
    section-order walk and the empty/zero-amount skip were already in
    place from PW2-004/PW2-005 — verified by the three new tests
    (`TestPaycheckWizard_BuildSplits_TagsEachLine`,
    `TestPaycheckWizard_BuildSplits_ElidesZeroRows`,
    `TestPaycheckWizard_BuildSplits_BalanceInvariant`) in
    `internal/tui/paycheck_wizard_test.go`.

## Phase 4: Round-Trip via `paycheck_section`

Replace the v1 category-name heuristic in `Edit as paycheck →` with
direct reads of the `paycheck_section` tag.

- [x] **PW2-007 — Detection heuristic uses tags + earnings line**
  - RED: test `TestLooksLikePaycheck_V2_RequiresTagsAndEarnings` —
    `looksLikePaycheck(st, ...)` returns true when (a) the schedule
    is multi-line, (b) **every** split item has a non-NULL
    `paycheck_section`, and (c) at least one line is tagged
    `earnings`.
  - Test `TestLooksLikePaycheck_V2_NullTagHidesAffordance` — a
    multi-line schedule whose first split has NULL `paycheck_section`
    (e.g., a line added via the generic dialog) returns false even
    if the other lines are correctly tagged.
  - Test `TestLooksLikePaycheck_V2_NoEarningsTag_Returns_False` — a
    fully-tagged schedule with no `earnings`-tagged line returns
    false.
  - GREEN: rewrite `looksLikePaycheck` in
    `internal/tui/paycheck_wizard.go`. Drop the v1 logic that scanned
    category display names for the `"Tax > "` prefix and looked for a
    positive categorized split. Replace with the three-clause check
    above. The function no longer needs `categoryOptions` /
    `categoryIDs` arguments — drop them and update the one caller
    (`buildEditScheduledDialog`).
  - Confirm: tests green; the v1 heuristic tests are replaced or
    deleted; full suite green.
  - Done: rewrote `looksLikePaycheck` in
    `internal/tui/paycheck_wizard.go` to walk the schedule's splits,
    bail to `false` on any nil/NULL-tagged split, and require at
    least one `earnings`-tagged line. Dropped the
    `categoryOptions` / `categoryIDs` arguments — only call site
    (`internal/tui/scheduled_dialog.go:333`, the
    Edit-as-paycheck affordance gate in
    `buildEditScheduledDialog`) updated to the single-arg form.
    The `TestScheduledDialog_EditAsPaycheck_RelaunchesWizard`
    fixture grew `PaycheckSection` tags on its five splits
    (earnings / tax / tax / post_tax / pre_tax) so the affordance
    still surfaces — the v1 section-routing in
    `NewPaycheckWizardFromSchedule` is unchanged, so the test's
    section-count assertions still pass (PW2-008 will rewrite that
    routing to read the tags). Three new tag-heuristic tests live
    in `internal/tui/paycheck_wizard_test.go`
    (`TestLooksLikePaycheck_V2_RequiresTagsAndEarnings`,
    `…_NullTagHidesAffordance`,
    `…_NoEarningsTag_Returns_False`).

- [x] **PW2-008 — `NewPaycheckWizardFromSchedule` reads tags to group lines**
  - RED: test `TestNewPaycheckWizardFromSchedule_V2_GroupsByTag` —
    given a schedule whose splits carry the v2 tags (one
    `earnings`-tagged positive line, two `tax`-tagged negatives, one
    `pre_tax`-tagged transfer-line, two `post_tax`-tagged categorized
    lines, one `net_pay_destination`-tagged transfer-line), the
    builder pre-fills the wizard with the right rows in each section:
    one row in Earnings, two in Taxes, one in Pre-tax, two in
    Post-tax, one in Additional Transfers. Storage order in the
    schedule does not matter — section assignment is from the tag,
    not the position.
  - Test `TestNewPaycheckWizardFromSchedule_V2_MultipleEarningsLines`
    — a paycheck with two `earnings`-tagged lines (e.g., `Income:Salary`
    +5000 and `Income:Imputed LTD` +44.03) opens with both rows in
    the Earnings section.
  - GREEN: rewrite `NewPaycheckWizardFromSchedule` in
    `internal/tui/paycheck_wizard.go`. Drop the v1 logic that
    matched lines against `preTaxLineSpecs.defaultCategory` /
    `postTaxLineSpecs.defaultCategory` slot tables by category name.
    Replace with: walk the schedule's splits; route each split by
    its `PaycheckSection` value (`earnings` → `earningsLines`,
    `pre_tax` → `preTaxLines`, `tax` → `taxLines`, `post_tax` →
    `postTaxLines`, `net_pay_destination` → `additionalTransfers`).
    Within each section, append rows in stored order. Magnitudes are
    re-rendered as positive strings (the wizard re-applies the sign
    at save time via the existing `buildDeductionSplit` path, per
    MS-027).
  - Confirm: tests green; the v1 round-trip test
    (`TestScheduledDialog_EditAsPaycheck_RelaunchesWizard`) is
    updated to use tagged splits in its fixture and continues to
    pass; full suite green.
  - Done: replaced the v1 category-name routing in
    `NewPaycheckWizardFromSchedule` with tag-based dispatch through a
    new private helper `sectionForTag(string) PaycheckSection` that
    inverts `PaycheckSection.tagString()`
    (`earnings`/`pre_tax`/`tax`/`post_tax`/`net_pay_destination` →
    the matching `PaycheckSection`; unknown/empty falls back to
    `PaycheckPostTax`). The split-loop now reads
    `sp.PaycheckSection.String` to pick the section, then routes the
    transfer-account vs categorized branch as before. Storage order
    within each section is preserved (rows are appended one by one).
    NULL-tagged splits are defensively routed to `PaycheckPostTax`,
    though `looksLikePaycheck` rejects any such schedule before this
    path opens. The amount field is still pre-filled with
    `sp.Amount.String()` verbatim — v2's `BuildSplits` preserves the
    user's typed sign (unlike v1's `buildDeductionSplit` flip), so
    `-800` round-trips as `-800`; the plan's "positive strings"
    phrasing was inherited from v1 and no longer applies under
    v2. Two new tests
    (`TestNewPaycheckWizardFromSchedule_V2_GroupsByTag` and
    `TestNewPaycheckWizardFromSchedule_V2_MultipleEarningsLines`)
    exercise the routing — the first deliberately stores splits in a
    shuffled non-section order (net_pay_destination, post_tax,
    post_tax, pre_tax, tax, tax, earnings) to prove section
    assignment comes from the tag, not the position. The existing
    round-trip test
    (`TestScheduledDialog_EditAsPaycheck_RelaunchesWizard`) had its
    section-count assertions updated to match the v2 tag-based
    routing: Salary (tagged `earnings`) lands in EarningsLines,
    Federal/Social Security in TaxLines, the 401(k) transfer
    (tagged `pre_tax`) in PreTaxLines, and Health in PostTaxLines —
    with zero AdditionalTransfers since no `net_pay_destination`-
    tagged line exists in the fixture. Full suite (5291 tests) and
    lint stay green.

## Phase 5: Smoke Check

- [x] **PW2-009 — Spec docs already updated**
  - `specs/multiline-splits-and-paycheck.md`, `specs/tui.md`, and
    `README.md` were updated alongside this plan's authoring.
    No additional doc work needed.

- [ ] **PW2-010 — Visual smoke check**
  - Manual: invoke `Transactions → New Paycheck Schedule…`. Confirm
    the form opens with the v2 layout (5 sections; Earnings with one
    `Income:Salary` row; Pre-tax empty; Taxes with Federal/SS/Medicare;
    Post-tax empty; Net Pay Destinations with the primary picker
    only). Click `[+ Add earnings line]` and add an LTD imputed line
    (+$44.03). Add a matching $-44.03 line in Post-tax with the same
    `Income:Imputed LTD` category (creating the category inline if
    needed). Fill out gross, taxes, a 401(k) transfer in Pre-tax, an
    HSA transfer in Pre-tax, a Savings transfer in Net Pay
    Destinations. Save. Verify the schedule appears in the Scheduled
    view with the expected structure. Open Edit Series, click
    `Edit as paycheck →`. Verify the wizard reopens with all lines
    back in their original sections, including the multi-line
    Earnings (Salary + LTD Imputed) and the same-category offset in
    Post-tax. Cancel out. Post once via the preview dialog. Verify
    the register shows the parent + paired counterparts in 401k / HSA
    / Savings accounts and the parent's net amount equals the
    expected take-home.

---

## Out of Scope

The following are explicitly deferred — not in this implementation plan:

- **CLI surface for the v2 paycheck wizard.** Wizard remains TUI-only,
  same as v1 (and same as the v1 spec's deferred list).
- **Category-master `tax_treatment` field.** The v2 wizard's
  `paycheck_section` is a wizard-layout hint, *not* a tax classifier.
  Tax-aware reports (W-2 Box 1 reconciliation, pre-tax vs post-tax
  separation in spending reports) would need a separate
  `tax_treatment` field on the category master, with its own spec.
- **Migration of v1 paycheck schedules to tagged form.** Per the
  current user, there are no v1 paycheck schedules in the wild that
  need migrating. If a future user lands here with v1 schedules, the
  spec's "NULL tag hides the Edit-as-paycheck affordance until
  re-saved through the wizard" path handles them — they're still
  valid multi-line schedules and post normally.
- **Imputed-income pairing logic in the wizard.** Per the spec, the
  two lines (earnings + offsetting deduction) are entered
  independently; the wizard does not auto-create the offset. If a
  future revision wants automatic pairing, it would need either
  per-line metadata (`paired_with_split_item_id`) or a new wizard
  concept (`[Mark as imputed]` checkbox); either is out of scope here.
- **Variable-per-line amounts on multi-line schedules.** Same as v1:
  per-instance variation lives in the post-time preview, not on the
  template.
