# Design sketch: TUI decomposition — one modal registry, and the god struct's other half

**Date:** 2026-08-08 (revised 2026-08-09 after design review)
**Status:** PROPOSED — **covers item 4a only.** See the closeout table below.

**Addresses:** `specs/code-quality-review.md` item 4 (TUI god-objects: dialog
state on `App`, wizards past 1k–1.8k lines) — **partially**.

### Item 4 is four jobs, not one

Item 4 names two symptoms in one paragraph: `App` field sprawl, and 1k–1.8k-line
surface files. They have different causes and different fixes, and conflating
them is how this work would acquire a permanent, wrong TODO. Split explicitly:

| Slice | What it is | Covered here? |
|---|---|---|
| **4a** | The modal layer has no single concept: 81 loose fields, four hand-maintained lists, two invisible dialogs | **Yes** — phases 0–4 |
| **4b** | View-layer god files (`price_view.go` 1,116, `investment_register_view.go` 1,032) | No — separate design (§8) |
| **4c** | Controller boundary: `Open`/`Submit`/`Close` move off `*App` onto the surface type | Phase 5 pilots one surface; the rest is deferred |
| **4d** | Surface **file** size: `paycheck_wizard.go` 1,886, `loan_wizard.go` 1,288, `split_dialog.go` 1,172 | No — the note at the end of §4 explains why it is cheap and independent |

**4d is the slice that actually closes the review's line table, and it does not
depend on 4a or 4c.** Measured: 1,462 of `paycheck_wizard.go`'s 1,886 lines are
*already* off `App` (70 of its 77 functions are on the wizard type or free).
A full 4c controller extraction would remove ~424 lines and leave a 1,462-line
file. `split_dialog.go`'s `App` region is 199 lines of 1,172. The wizards are god
files in their **non-`App` half**, so making them controllers barely dents their
size. See the note at the end of §4.

Nothing here should be read as closing item 4. It closes 4a, pilots 4c, and
names 4b and 4d with the measurement that sizes them.

| Phase | Shipped | What it does |
|---|---|---|
| 0 | **yes** | Render the two dialogs that no code paints today (a live bug, §1.3) |
| 1 | **yes** | Declare `Modal`; give three types the methods they lack; one registry replaces the key and paint cascades |
| 2 | — | Move the mouse cascade onto the registry; close the mouse/keyboard gap that `specs/tui.md` forbids |
| 3 | — | Group each surface's loose fields into one struct; `App` sheds ~65 fields |
| 4 | — | Move `app_update.go`'s message bodies onto those structs — **off** `App`, not into another `App` method |
| 5 | — | One pilot controller: `open`/`submit`/`close` off `*App` for a single surface, proving 4c is reachable |

Read §1 first for the measurement, then §3 for the one prescription in the
review that this design rejects, then the phases.

---

## Goal

Make the TUI's modal layer one concept instead of four copies of a list.
Concretely, and measured:

1. The set of modal surfaces is declared **once**. Today it is written out four
   times, in four files, by hand (§1.2).
2. Every modal surface is reachable, paintable and clickable. Two are not
   today (§1.3).
3. Each dialog family owns its state in **one struct**. Today 81 of `App`'s 160
   fields are loose modal state (§1.1).
4. `app_update.go` stops reading other surfaces' private fields. It is 1,002
   lines and touches 94 of the 160 fields.
5. The review's instruction to make the wizards "packages under `tui/`" is
   **rejected in writing** (§3), so the next reader does not re-derive it from
   the old review.

Target numbers at the end of phase 5:

| Measure | Now | Target |
|---|---|---|
| `App` fields | 160 | ~95 |
| Methods on `*App` | 357 | **~398, and stated honestly** — see below |
| Copies of the modal list | 4 (31 + 29 + 32 + 30 entries) | 1 |
| `app_update.go` lines | 1,002 | ~300 |
| Modal surfaces that never render | 2 | 0 |
| New packages | — | 0 |
| Surface-file line counts | 1,886 / 1,288 / 1,172 | **unchanged — that is 4d** |

The `*App` method-count row is the one the first draft of this design got wrong,
and the corrected number is still not flattering. It is stated anyway.

`app_update.go` has 89 case arms. **61 have four or more code lines** and would
need a name if moved; 28 are short enough to stay inline. The first draft's
phase 3 said "move each arm's body into its surface's file as an `App` method",
which lands `*App` at **~418 methods** — the wrong direction for a design whose
complaint is that everything is an `App` method.

Reordering structs-first helps, and it does not fix it. Measured, only **20 of
those 61 arms can become surface-struct methods**. The other 41 are pinned to
`*App` by what they touch: **53 of the 89 arms** touch the status bar, return a
`load*`/`reload*` command, call a service, read the sidebar, call `switchView`,
set `a.err`, or coordinate two surfaces. §5.5 forbids putting services on a
surface struct, so those arms cannot move without breaking a rule this design
relies on elsewhere.

So the honest projection is **357 → ~398** (+41), against ~418 for the naive
ordering. The reorder is worth doing — it is 20 methods and it puts the message
body next to the state it mutates — but **this design does not reduce `App`'s
method surface, and phases 0–5 are not the thing that will.**

That is 4c at full scope, and it is worth pricing here so phase 5 has a target
to argue about. Of today's 357 `*App` methods, **130 reference exactly one
surface family and touch no service, no chrome field and no second surface** —
they are relocatable in principle. The other 227 are pinned. So a complete 4c
would land `App` somewhere near **270 methods**, a ~30% cut. That figure is the
prize phase 5 exists to price — it is **not** a commitment this design makes,
and it must not be read into the status line. Phases 0–5 claim none of it.

## Non-goals

- **No new packages.** §3 gives the measurement that decides this. It is the
  same decision `design-service-decomposition.md` made for item 3, for the same
  reason, with a blast radius roughly 50× larger here.
- **No view-layer work.** `price_view.go` (1,116 lines, 33 `App` methods) and
  `investment_register_view.go` (1,032 / 20) are god files, but they are feature
  slices, not modal state. A `View` interface would also collapse the 12-arm
  focus switch in `switchView` (`app_menu.go:296`). That is a separate design.
  Mixing the two produces one change nobody can review.
- **No bubbletea sub-models.** A controller must not implement `tea.Model`.
  In bubbletea v2 `Update` returns the **root** model, so returning a controller
  from any `Update` path replaces the whole application. `tea.Model.View()` also
  returns `tea.View`, not `string`, while every overlay today returns a string
  that `App` composites. The registry keeps `Render(styles) string`.
- **No move of the sticky date.** `specs/tui.md:172-178` pins
  `txnDialogLastSavedDate` as session state shared across the transaction,
  investment and corporate-action dialogs, with "Cancel does not update it".
  It is a property of the session, not of any one dialog. It stays on `App`.
- **No work on review items 5 or 6.** `app.Services` keeps exposing
  repositories, and `NewServices` keeps its heal-on-open side effects. Phases 3
  and 5 touch code near both. Neither is in scope; §8 records what belongs to
  which item so it is not rediscovered.
- **No generic component framework.** The registry is one interface and one
  slice. If per-surface glue still hurts after phase 5, that is the moment to
  consider more — not before.
- **No surface file splits (4d) inside these phases.** Cheap, mechanical and
  independent — see the note at the end of §4. Excluded to keep one change
  reviewable, not because it is hard or unwanted. It may run in parallel under
  the rule in §8; it is a non-goal *of these commits*, not of the quarter.

---

## 1. The problem, measured

### 1.1 Half of `App` is modal state

`App` declares **160 fields** across 337 lines (`app.go:90-429`). **81** of them
are dialog, wizard or overlay state. 357 methods hang off `*App` across the 56
non-test files.

No dialog owns a struct. Each family spreads its handle, its form data and its
parallel ID slices across separate flat fields. The Sell dialog is typical:

```go
// internal/tui/app.go
sellDialog            *dialog.Dialog
sellDialogData        *sellDialogData
sellDialogSecurityIDs []types.ID
sellDialogLots        []*investment.Lot
```

Four fields, one concept, and nothing in the type system ties them together.
The Transfer Shares dialog uses five. The three corporate-action dialogs use
four each.

### 1.2 The same list is written four times

There are **32 distinct modal surfaces**. The list of them appears in four
places, in four files, maintained by hand — and no list holds all 32:

| File | Function | Surfaces | Orders by |
|---|---|---|---|
| `app.go:539` | `handleKeyPress` | 31 | input priority (first match wins) |
| `app_view.go:39` | `renderLayout` | 29 | paint order (last painted is on top) |
| `app_mouse.go:11, 232` | `handleMouseEvent` / `handleDialogMouse` | 32 | its own third order |
| `app_helpers.go:130` | `isDialogVisible` | 30 | none (an `\|\|` chain) |

`app_mouse.go:249` documents the duplication in a comment:
`// dialog.Dialog cascade (same order as handleKeyPress)`.

Adding a dialog therefore costs four edits in four files, and nothing detects a
missed one. §1.3 is what happens when an edit is missed.

### 1.3 Two dialogs take keystrokes and never render — a live bug

Of the 32 surfaces, exactly **three** are absent from at least one list:

| Surface | key | paint | mouse | isDialogVisible |
|---|:---:|:---:|:---:|:---:|
| `importDialog` | Y | **N** | Y | **N** |
| `linkTransfersDialog` | Y | **N** | Y | **N** |
| `corporateActionDetail` | **N** | **N** | Y | Y |

`importDialog` and `linkTransfersDialog` are built (`app_update.go:900, 911,
919, 943`), key-routed (`app.go:575, 580`) and mouse-armed (`app_mouse.go:320,
331`). They appear in **neither** `renderLayout` nor `isDialogVisible`.

Both are reachable from the menu bar:
- File → "Import Transactions..." (`widget/menubar.go:130` → `app_menu.go:76`)
- Transactions → "Link Transfers..." (`widget/menubar.go:174`)

Proven by probe, not by reading. A scratch test set each dialog on an otherwise
default `App` and called `viewContent()`:

```
TestProbe_ImportDialogNeverRenders
    NOT RENDERED: title "Pick Source Account" absent from renderLayout output
TestProbe_LinkTransfersDialogNeverRenders
    NOT RENDERED: title "Link Transfers" absent from renderLayout output
TestProbe_ControlAboutDialogDoesRender
    control OK: About renders
```

The control case rules out a broken probe. `git log -S 'importDialog' --
internal/tui/app_view.go` returns nothing: the import dialog has never been
painted in the history of the repository.

The user-visible effect: choosing File → Import blanks nothing and shows
nothing, but the next keystrokes go to an invisible form. Because
`handleMouseEvent` gates on `isDialogVisible()` (`app_mouse.go:26`), which also
omits both dialogs, their mouse arms are dead too.

The mirror case: `corporateActionDetail` is in `isDialogVisible` and the mouse
gate but has **no** key arm, and a second compositor inside the view body paints
it (`corporate_action_history.go:232-235`) rather than `renderLayout`. That one
is arguably correct — it is a view-embedded panel, not a floating modal — but
nothing in the code says so, which is why it reads as a fourth inconsistency.
Phase 0 makes that call explicitly, either way.

This is the same finding shape as item 3's §11.3: *a duplicated concept is a
place where two answers disagree, and enumerating the disagreements finds bugs.*
Here the concept is duplicated four times and three answers disagree.

### 1.4 The interface already exists; three types do not implement it

`*dialog.Dialog` — 26 of the 31 surfaces — already provides the whole set:

```go
IsVisible() bool                                        // dialog/dialog.go:90
SetVisible(bool)                                        // dialog/dialog.go:95
HandleKey(tea.KeyPressMsg) DialogAction                 // dialog/input.go:8
HandleMouse(tea.MouseMsg, int, int) DialogAction        // dialog/mouse.go:274
Render(widget.Styles) string                            // dialog/render.go:18
```

`Render` and `HandleKey` take styles as a **parameter**, not off `App`. That is
the property that makes a registry possible at all.

Three bespoke types diverge, and each divergence is small and known:

| Type | Has | Lacks |
|---|---|---|
| `PaycheckWizard` | all five | `HandleMouse` takes an extra `styles` parameter (`paycheck_wizard.go:1358`) |
| `SplitDialog` | four | only `HandleMouseLocal(x, y)` (`split_dialog.go:834`); hard-blocked from the mouse path by an early return (`app_mouse.go:245`) |
| `SchedulePreviewDialog` | `IsVisible` only | `SetVisible`, `HandleKey`, `HandleMouse`, `Render` — all four live as `App` methods |

So the interface cannot be declared today. Phase 1 fixes three types first.
That is a small, bounded, compiler-checked job, not a rewrite.

### 1.5 Two modals are visible together in exactly one case

A registry must reproduce today's behavior. The four lists order the surfaces
differently, so the safety question is: **which pairs can be visible at the same
time?** For any pair that cannot co-occur, the orders cannot disagree
observably.

Enumerated from the code, the answer is short.

**Case A — the create-category divert is a swap, not a stack.** All eight
originating surfaces hide themselves before showing the sub-dialog. Verified in
all eight:

| Surface | Hides itself at |
|---|---|
| Transaction | `transaction_dialog.go:510` |
| Transfer | `transfer_dialog.go:440` |
| Scheduled | `scheduled_dialog.go:612` |
| Scheduled transfer | `scheduled_transfer_dialog.go:445` |
| Scheduled preview | `scheduled_preview_dialog.go:805` (hides the header) |
| Split | `split_dialog.go:1017` |
| Paycheck wizard | `paycheck_wizard.go:1619` |
| Loan wizard | `loan_wizard.go:625` |

The preview case is subtle and load-bearing:
`SchedulePreviewDialog.IsVisible()` **delegates** to its header dialog
(`scheduled_preview_dialog.go:457`), and the divert hides that header. So the
preview leaves all four lists at once. Any registry must keep that aliasing, or
the preview will paint under the sub-dialog.

**Case B — the merger confirmation is also a swap.** `submitMergerDialog` calls
`closeMergerDialog()` before it loads the confirmation
(`corporate_action_merger_dialog.go:208`).

**Case C — `showConfirmDialog` is the one true stack.** It leaves the surface
underneath visible (`scheduled_dialog.go:935`, `:1027`). It also parks a
continuation on `App`:

```go
confirmAction func() tea.Msg    // app.go:405
```

Nine call sites across seven files. Here the orders agree: confirm has the
highest key priority (rank 2) and paints second from the top (rank 28).

**Conclusion.** The four lists agree on every pair that can co-occur. They
disagree only on pairs that cannot. Collapsing them is therefore safe — but the
proof is this enumeration, not an assumption, and phase 1 must land it as
tests-as-spec. The one behavior change the collapse forces is the §1.3 fix, and
phase 0 lands that separately so it is reviewed as a bug fix rather than hidden
inside a refactor.

---

## 2. The shape

### 2.1 One interface

```go
// Modal is a surface that takes input away from the view underneath while it
// is visible. Every dialog, wizard and overlay in the TUI is one.
type Modal interface {
	IsVisible() bool
	SetVisible(bool)
	HandleKey(tea.KeyPressMsg) dialog.DialogAction
	HandleMouse(msg tea.MouseMsg, styles widget.Styles, w, h int) dialog.DialogAction
	Render(styles widget.Styles) string
}
```

`*dialog.Dialog` needs a two-line adapter for the wider `HandleMouse`; it
already has the rest.

Styles stay a **per-call parameter**. `App.styles` is a value field mutated in
place by `Resize` (`app_update.go:21`) and `ApplyTheme` (`app.go:503`). A
controller that captured styles at open time would keep the pre-resize width and
the old palette for its whole life.

### 2.2 One registry

The glue that `App` supplies per surface — what to do on submit, what to do on
cancel — becomes data:

```go
type modalEntry struct {
	modal    Modal
	onSubmit func(*App) (tea.Model, tea.Cmd)
	onCancel func(*App) (tea.Model, tea.Cmd)
}

// modals returns every modal surface in priority order: index 0 receives keys
// first and paints last (on top). The single source of the order that
// handleKeyPress, renderLayout, handleMouseEvent and isDialogVisible each
// spelled out by hand.
func (a *App) modals() []modalEntry
```

Keys walk the slice forward and stop at the first visible entry. Paint walks it
backward. `isDialogVisible` becomes "any entry is visible". The mouse path walks
it forward like keys.

`renderLayout` keeps its explicit `SetMaxHeight` step, because that write is
load-bearing — see §5.1.

**Where the glue ends up.** Through phase 2, `onSubmit`/`onCancel` are closures
over `*App` calling today's `submitXDialog`/`closeXDialog` — that is what makes
phases 1–2 pure routing changes. Phase 5 moves the *bodies*, and the entry must
then be a thin wrapper, not a second implementation:

```go
// After phase 5, for the piloted surface. The entry stays one line; the
// behavior lives on the surface. The failure mode this forbids is a 200-line
// onSubmit closure on App coexisting with surface methods — two paths for one
// action, which is the thing this design exists to remove.
{
	modal:    a.transfer,
	onSubmit: func(a *App) (tea.Model, tea.Cmd) { return a.transfer.submit(a) },
	onCancel: func(a *App) (tea.Model, tea.Cmd) { a.transfer.close(); return a, nil },
}
```

Surfaces that have not been through 4c keep the closure-over-`App` form. Both
shapes are legal at once; what is not legal is one surface having both.

### 2.3 What the registry cannot do

Item 3's §11.3 found that every extracted type was defined by what it *cannot*
do. Stating that up front here:

- The registry **cannot own services**. It is a list of surfaces and their glue.
- The registry **cannot handle messages**. Saved messages arrive after their
  dialog is closed (§5.2), so message routing stays on `App`.
- The registry **cannot decide modality**. Each surface answers `IsVisible()`
  for itself, including `SchedulePreviewDialog`'s delegation to its header.
- The registry **cannot express two orders**. It is one slice. §1.5 is the proof
  that one order is enough; if a future surface needs key priority and paint
  order to differ, that surface is wrong, not the registry.

---

## 3. Why not packages — the review's item 4 prescription, rejected

The review says the wizards "should be packages under `tui/`". This design does
not do that, and the reason is a constraint the repository already wrote down.

Commit `01aa7b7`, which created `tui/widget` and `tui/dialog`, recorded it:

> Go requires methods to be defined in their type's declaring package, so the
> App-behavior files are pinned to `internal/tui/`. The widget and dialog files
> were the subset with zero App-method definitions.

So a package can only take code that is **already off `App`**. Packages are the
*result* of decomposition, not the *method*. Moving `paycheck_wizard.go` to
`internal/tui/paycheck` today would move a file whose last 430 lines are `App`
methods — they would have to stay behind, and the package would hold a type that
`App` still drives. That is a rename, not a decomposition.

The costs are measurable, and they are all paid before any structural gain:

1. **Tests.** All 66 test files declare `package tui`. There is no
   `package tui_test`. Grep finds **2,084** `app.<unexported>` references and
   **688** `&App{...}` literals, each naming three to ten unexported fields.
   `design-service-decomposition.md` refused a package split over **41** such
   tests. This is 50× that.
2. **`errMsg`.** `type errMsg struct{ err error }` (`app.go:864`) is the only
   error channel out of a `tea.Cmd`, with 246 references in 38 non-test files.
   A subpackage cannot construct it, and it cannot be imported back from `tui`
   without a cycle — `tui` already imports `tui/dialog` and `tui/widget`.
3. **The return type.** Every `handleXKey` and `submitX` returns
   `(tea.Model, tea.Cmd)` and returns `a`. A subpackage cannot name `*App`, so
   ~90 signatures change before one behavior does.
4. **Shared helpers.** The wizards' `App`-free regions call `parseAmountInput`,
   `parseDateInput`, `buildCategoryOptions` and `formatDashboardMoney`, all
   declared in `package tui`. `keyMap` is unexported with 103 `a.keys.*`
   references across 15 files.
5. **Naming.** `internal/loan` already exists and `loan_wizard.go` imports it,
   so `internal/tui/loan` needs an alias on day one. The CLI split hit this on 9
   of 11 nouns (`specs/cli-package-split.md`, R1).
6. **Precedent against.** `specs/cli-package-split.md` D1 states that per-noun
   vertical slices work for the CLI *because* "CLI commands share no mutable
   state (unlike `tui` views, which share one Bubbletea model), so per-noun
   vertical slices are clean here when per-view was not in `tui`." The CLI
   coupling was a star. The TUI's is a web: eight create-category surfaces,
   three owners of `SplitDialog`, twelve files touching `createCatDialog`.

**The decision.** Phases 1–4 stay in `package tui` and cost zero test rewrites
in phases 0–3. If, after phase 4, a surface's state and behavior live entirely
in one struct with no `App` methods left, that surface has become a package
candidate on its own merits. Re-open the question then, with that surface's
numbers. Do not re-open it from the old review.

---

## 4. Phases

### Phase 0 — paint the two invisible dialogs

Add `importDialog` and `linkTransfersDialog` to `renderLayout` and
`isDialogVisible`. Give `corporateActionDetail` a key arm or record why it does
not need one.

This is a bug fix, not a refactor. It ships first and alone so review can judge
it as one. It also removes the only user-visible change the registry would
otherwise smuggle in.

Tests: one render assertion per dialog, of the shape the §1.3 probe used.

**Record a decision for `corporateActionDetail` in this phase**, do not leave it
observed. It is painted by the view body (`corporate_action_history.go:232-235`),
sits in the mouse gate and `isDialogVisible`, and has no key arm. Either it is a
modal (give it a key arm and move it into the registry) or it is a view-embedded
panel (leave it out of `modals()` and say so in a comment). Both are defensible;
silence is not, because the next reader will re-derive the question.

#### Phase 0 is not a "pure bug fix" — say so

Painting these dialogs promotes two paths from unreachable to usable, and their
wiring has a known defect. `runImportPreview` (`import_dialog.go:373`),
`runImportExecute` (`:425`) and both link-transfer commands
(`link_transfers_dialog.go:31, 45`) each call `app.NewServices(a.db)` on a
command goroutine. `NewServices` writes on construction — `EnsurePaycheckCategories`
(`registry.go:101`), `EnsureValueAdjustmentCategory` (`:105`), `HealAllAccounts`
(`:131`) and `HealNextDates` (`:148`). So **opening the import preview silently
heals scheduled dates and investment accounts**, and it reads `a.db` at goroutine
time rather than capture time (§5.6).

Two things follow, and neither is optional:

1. **Phase 0's exit criteria include a manual smoke of File → Import and
   Transactions → Link Transfers end to end** — not merely "the title appears in
   the rendered output". A render assertion proves the paint fix; it does not
   prove the feature works.
2. **File the four call sites against review item 6 before phase 0 merges** (§8).
   They are item 6's territory, not this design's, but phase 0 is what makes
   them reachable, so phase 0 is what owes the follow-up.

Worth stating plainly, because it cuts the other way: this is **not** a
regression phase 0 introduces. The dialogs already take keystrokes today
(`app.go:575, 580`) and Enter already submits, so a user can drive an invisible
import right now. Painting them strictly reduces risk. The point is only that
"nobody could use it" stops being the mitigation.

### Phase 1 — declare `Modal`; collapse the key and paint cascades

1. Normalize the three bespoke types (§1.4). `SchedulePreviewDialog` gains
   `SetVisible`, `HandleKey`, `HandleMouse` and `Render` by moving the existing
   `App` method bodies onto it. `SplitDialog` gains `HandleMouse`.
   `PaycheckWizard.HandleMouse` already has the target signature.
2. Declare `Modal` and `modals()`.
3. Replace `handleKeyPress`'s 31 entries and `renderLayout`'s 29 with walks over
   the registry. Derive `isDialogVisible` from it.
4. Land §1.5 as tests: one test that pins the registry order, and one per
   co-occurring pair (create-category over each of its 8 originators; confirm
   over scheduled and over split) asserting which surface takes a key and which
   paints on top.
5. Record the §5.0 nil decision in writing, and land guard 1.

`app_view.go` keeps its explicit `SetMaxHeight` call (§5.1).

#### Built: the interface is narrower than §2.1, and the reason is measured

§2.1 declares five methods. What shipped is two:

```go
type Modal interface {
	IsVisible() bool
	Render(styles widget.Styles) string
}
```

`HandleMouse` is deferred to phase 2, which is the phase that calls it — and
that is a better split than §1.4 implied, because `SplitDialog`'s missing
`HandleMouse` is a *mouse* problem, blocked behind a mouse early return.

`HandleKey` is the interesting one, and it is **not deferred — it is rejected**.
Measured across the 31 key arms: **30 already call `X.HandleKey(msg)` exactly
once.** So the method exists nearly everywhere and looks like an easy win. But
what follows the call is a switch on the returned `DialogAction`, and *that* is
what varies — four distinct action sets across the arms (`Submit`, `Cancel`,
`AddNew`, `Alternate`) — and **every arm of every switch needs `*App`**: to call
a service, to close a sibling surface, or to divert into create-category. A
registry walk cannot dispatch it.

Putting `HandleKey` in the interface would therefore add a member that 31 types
satisfy and no walk calls, and it would force the one type that lacks it,
`SchedulePreviewDialog`, into a restructure to produce a method nothing invokes.
Its multi-line path interleaves `maybeReseedLoanPreview` and
`freezeLoanSeedIfEdited` — both needing `a.scheduledTxnSvc` — between the inner
`HandleKey` and the action switch, and §5.5 forbids a surface holding a service.

So key dispatch is per-surface glue and lives on `modalEntry.onKey`, holding
today's `handleXKey` method unchanged. That is what makes the collapse a pure
routing change. `SetVisible` is out for the same class of reason: only the
create-category divert calls it, and it calls it from the originating surface,
never from the registry.

**The rule this sets: add a member when a walk needs it, not before.** §2.3
listed what the registry cannot do; this adds a fourth entry to that list — *the
registry cannot own input dispatch*, because every dispatch needs `*App`. That
is the same finding shape as §5.2 (it cannot route messages).

Three surfaces have no modal-typed field and needed adapters: the help overlay
(a bare `bool`), the merger confirmation (visibility is the presence of its
data), and the backup dialog (its dialog is one level in). Each is named in the
guard's `registryOnlySurfaces` with its reason, so a fourth cannot appear
silently.

#### Built: §1.5 verified by differential, and the paint order was wrong

§1.5 argues the collapse is safe because the four lists agree on every pair
that can co-occur. That argument was checked, not taken. With the old cascades
kept alongside the new walks, a differential ran every surface and every pair:

| Check | Result |
|---|---|
| `isDialogVisible`, per surface | identical |
| Paint, single surface | identical |
| **Key routing, all 496 pairs** | **identical** |
| Paint, all 496 pairs | **352 change which surface ends up on top** |

Key routing is identical by construction — the registry order is
`handleKeyPress`'s order — and the differential proves it rather than assuming
it. Paint is where the old lists disagreed, and 352 of 496 is far too large a
number to wave through with "they cannot co-occur". So the differential was
narrowed to the question that matters: **does any pair that can be visible at
once change?**

By §1.5's own classification the co-occurring set is small. Cases A and B are
*swaps*, not stacks — all eight create-category originators hide themselves, and
`submitMergerDialog` closes the merger dialog before its confirmation loads — so
neither belongs in the set. Only Case C, `showConfirmDialog`, is a true stack.
One correction to §1.5's supporting detail: `backup_dialog.go:118` nils
`a.backupDialog` *before* calling `showConfirmDialog`, so restore-from-backup is
a swap too, leaving confirm-over-scheduled and confirm-over-split.

**Neither changed.** The 352 are all pairs that cannot co-occur.

And the first pass got this wrong in an instructive way. A differential run
against a 4-character base layout reported **0 of 992 pairs differing** — a
false pass, because no overlay could be distinguished on a layout that small. A
self-test asking "can this comparison see *any* difference?" caught it. A
differential that cannot fail is worth less than no differential, because it
reads as proof.

**The new order also fixes a latent bug.** Five of the 352 are create-category
against its originators, and there the *old* order was wrong: create-category
was painted second from the bottom, so `transfer`, `scheduled`,
`schedulePreview`, `paycheckWizard` and `loanWizard` all painted **over** the
sub-dialog they had just opened. Invisible today only because each originator
hides itself first. Deriving paint from key priority makes the surface that owns
the keyboard also the surface on top — correct by construction rather than by
eight cooperating call sites. `TestModals_CreateCategoryOutranksItsOriginators`
pins it.

**Phase 1 does not touch the mouse.** `handleDialogMouse` keeps its 30 hand-written
entries until phase 2, so for one phase the registry is the source of truth for
two of the four lists and not the third. That is deliberate — the mouse path
carries the Cancel-semantics change (§ phase 2), which is a behavior change and
must be reviewed on its own. Do not let phase 1's exit criteria imply the
cascade is gone.

### Phase 2 — the mouse cascade

Move the mouse cascade's 32 entries (`handleMouseEvent` plus
`handleDialogMouse`) onto the same registry.

This closes a gap that `specs/tui.md:687-695` forbids: "Clicking a dialog button
is exactly equivalent to the keyboard action (Enter on Save, Esc on Cancel):
both input paths route through the same submit/cancel handler."

`handleMouseEvent` has **27** Cancel arms. Eight call the surface's close
helper. **Nineteen inline `X.SetVisible(false); X = nil`** and leak whatever
else that surface owns. The keyboard path calls the helper in every case, so
the two input paths leave different state behind. Measured, for the arms where
a helper exists:

| Surface | Helper nils | Mouse arm nils | Leaked |
|---|---|---|---|
| Transfer shares (`:558`) | 5 | 1 | 4 |
| Scheduled (`:389`) | 5 | 1 | 4 |
| Transfer (`:372`) | 4 | 1 | 3 |
| Sell (`:524`) | 4 | 1 | 3 |
| Transaction (`:358`) | 3 | 1 | 2 |
| Buy (`:512`) | 3 | 1 | 2 |
| Stock split (`:570`) | 3 | 1 | 2 |
| Merger (`:582`) | 3 | 1 | 2 |
| Spin-off (`:596`) | 3 | 1 | 2 |
| Account (`:438`) | 2 | 1 | 1 |
| Cash operation (`:608`) | 1 | 1 | 0 |

Routing both input paths through one `onCancel` fixes all of them at once.

That changes observable post-cancel state, and some of the 688 test literals may
assert on the leftovers. Audit those assertions **before** the change, not
during. Note also that `securityDialog` and `priceDialog` have **no** close
helper at all — give them one in this phase rather than teaching the registry an
exception.

`SplitDialog`'s mouse early-return (`app_mouse.go:245`) is a deliberate block
today. Keep it as an explicit registry flag, or delete it and give the split
dialog working mouse support as a separate, stated change. Do not let it
disappear by accident.

### Phase 3 — per-surface state structs

Collapse each family's loose fields into one struct held by one `App` field:

```go
// before: 4 fields
sellDialog            *dialog.Dialog
sellDialogData        *sellDialogData
sellDialogSecurityIDs []types.ID
sellDialogLots        []*investment.Lot

// after: 1
sell *sellSurface
```

This is the first phase that changes tests, because the 688 literals name the
old fields. Do it one family at a time, smallest first, so each commit is
reviewable and revertible. Suggested order by test coupling, cheapest first:
transfer shares, fee liquidation, spin-off, merger, stock split, buy, dividend,
sell, then the create-category cluster.

It also touches `app_update.go`: **107 references to the 79 modal fields live
there**, and every one is a compiler-proven rename. That is why this phase comes
before the message move and not after — see phase 4.

The create-category scratch fields are the deliberate last commit, and their end
state is first-class, not an afterthought. Today five fields encode one idea —
"which surface do I return to, and where in it":

```go
createCatDialog       *dialog.Dialog
createCatSource       createCategorySource   // 8-value enum
createCatSplitRow     int                    // -1 sentinel
createCatPaycheckLine *PaycheckLine          // nil sentinel
createCatLoanField    int                    // -1 sentinel
```

Three of the five are sentinel-carrying scratch slots for three *different*
originator shapes. They become one value:

```go
// createCatOrigin records which surface opened the create-category sub-dialog
// and where inside that surface the new category must land. A nil pointer
// means no sub-dialog is in flight, which replaces the three separate
// -1 / nil sentinels.
//
// Go has no sum type, so the invariant is stated rather than enforced:
// `surface` selects which of the three slots is meaningful, and the other two
// are undefined, not zero. Read them only through the switch that dispatches
// on `surface`, and add a test asserting each surface enum reads exactly the
// slot it owns — that test is the sum type.
type createCatOrigin struct {
	surface  createCategorySource
	splitRow int           // valid when surface == createCatSourceSplitDialog
	line     *PaycheckLine // valid when surface == createCatSourcePaycheckWizard
	loanField int          // valid when surface == createCatSourceLoanWizard
}
```

This is the tangle item 4 named first ("category-create routing:
`createCatSource`, split row, paycheck line, loan field"). It is scheduled last
because it touches eight surfaces (16 functions across 8 files, see the note below), not
because it is optional.

### Phase 4 — messages onto the surfaces, off `App`

`app_update.go` is 1,002 lines, 89 case arms, and it reads or writes 94 of the
160 fields. Most arms are one surface's `*DataMsg` or `*SavedMsg` handler poking
that surface's private fields from another file.

**The obvious move is the wrong one.** Moving each arm into its surface's file
as `func (a *App) handleXMsg(...)` is pure motion the compiler proves — and it
takes `*App` from 357 methods to ~418 (61 arms have four or more code lines). It relocates the sprawl instead of removing it. That is file locality,
not a boundary.

Because phase 3 has already run, the bodies have somewhere better to go. Take
the largest arm, `transferDialogDataMsg` at 34 lines (`app_update.go:453`). It
touches four fields of one surface, plus exactly two `App`-owned values —
`a.sidebar.SelectedAccountID()` and `a.txnDialogLastSavedDate`. So:

```go
case transferDialogDataMsg:
	a.transfer.applyData(msg.data, a.sidebar.SelectedAccountID(), a.txnDialogLastSavedDate)
	return a, nil
```

`applyData` is a method on `*transferSurface`. The two `App`-owned inputs pass
in as values, which is also the §5.6 discipline.

Arms that genuinely belong to `App` stay on `App`: anything touching the status
bar, the undo manager, a view reload, more than one surface, or `switchView`.
Do not force those onto a surface struct to make a number look better — record
how many stayed and why.

Exit measure for this phase is the method count, not the line count. The bar is
**bounded growth, not zero growth**: `*App` may gain the pinned arms and nothing
else. If the count lands near 418 rather than 398, the reorder did not do its
job and the phase is not done.

### Phase 5 — one pilot controller

Phases 0–4 give every surface one state struct and one registry entry. They do
**not** give it `Open`/`Submit`/`Close`. Those stay `*App` methods, so no surface
could yet leave `package tui`. Phase 5 proves the door is real by walking one
surface through it.

Pick **the transfer dialog**, measured:

| | |
|---|---|
| File | 754 lines, 11 `App` methods |
| `App`-method bodies | **502 lines** — the part that would move |
| Free functions and type declarations | 252 lines — already take data, not `*App`; untouched |
| Services touched | 4 (`accountSvc`, `categorySvc`, `transferSvc`, `undoManager`) |
| `App` state touched that is **not** its own | **5 references total** — `createCatDialog` ×2, `createCatSource` ×1, `currentRegisterAccountID` ×2 |
| `statusbar` / `sidebar` / `currentView` / sticky date | **zero references** |
| Own fields | 4 → 1 |

That last row is why it is the right pilot: every piece of `App` that the
transfer dialog reaches for is either a service or the create-category divert.
There is no hidden coupling to the chrome. All the chrome-touching work lives in
the two message arms in `app_update.go:453-508`, and `transferDialogSavedMsg`
(18 lines) is the part that provably cannot move — which is a useful result to
get from a pilot rather than from an argument.

Two candidates to rule out explicitly, because both look cheap and are not:

- **Create-category.** Its own file holds 2 of the cluster's 22 `*App` methods
  (see the note below); the rest are spread over 9 files, and three of its five
  fields are typed handles into `PaycheckWizard`, `SplitDialog` and the loan
  wizard. Piloting it would mean defining the controller contract for all eight
  originating surfaces *first*. It is the terminal step, not the first one.
- **Paycheck.** 1,886 lines, a hand-rolled field system that re-implements
  `dialog/input.go`, and only 3 `&App{}` literals across its tests. It would
  prove nothing about the common case. (It also carries one misfiled method,
  `relaunchAsPaycheckWizard` at `paycheck_wizard.go:1869`, which is the sole
  source of its coupling to the scheduled dialog's internals — move that first
  and paycheck's real dependency set is 3 services.)

The full ordering, easiest to hardest true controller extraction:
**transfer → paycheck → loan → split → create-category.**

Deps must be a **live indirection**, not captured pointers. `switchDatabase`
re-points 18 service fields at runtime (`file_dialog.go:230-247`) and closes the
previous `*db.DB`, so a surface holding a captured `*transfer.Service` would be
a use-after-close — the exact class commit `6dede4d` fixed, and one the existing
guard would no longer catch (§5.5). Concretely, every dep is a function, never a
pointer:

```go
// Every service is reached through a FUNCTION, never a captured pointer.
// switchDatabase replaces these on the App when the user opens another file;
// a closure re-reads the field at call time, so it is correct by construction
// and switchDatabase needs no new line.
type transferDeps struct {
	accounts   func() *account.Service
	categories func() *category.Service
	transfers  func() *transfer.Service
	undo       func() *undo.Manager
}
```

Keep the existing nil guards: these may legitimately return nil in tests.

**Written this way, item 5 costs the pilot nothing.** When item 5 collapses
`App`'s 17 service fields into one `*app.Services`, each closure changes to read
one field and no signature moves:

```go
transfers: func() *transfer.Service { return a.services.Transfer },
```

That is the whole migration for the pilot, which is the point of choosing
closures over a captured struct.

**The pilot inherits one known bug; do not score it as a regression.**
`undoManager` is assigned once in `NewApp` and `switchDatabase` never re-points
or clears it (§5.5), so the undo stack outlives the database its commands were
built against. A `func() *undo.Manager` dep faithfully reproduces that: it
returns the same manager before and after a file switch, because that is what
`App` holds. Post-switch undo misbehaviour in the pilot is the **pre-existing**
item-5 bug surfacing, not something phase 5 introduced. Say so in the phase 5
commit message so the next person does not bisect to it.

If the pilot lands cleanly, 4c becomes a per-surface decision with a known cost.
If it does not, that is the answer, and it is cheaper to learn on one 753-line
file than on eight.

### Note — the create-category divert is the most coupled surface, not the least

Worth stating as a number, because its small own-file footprint is misleading:

| File | Openers + appliers |
|---|---|
| `transfer_dialog.go` | 2 |
| `scheduled_dialog.go` | 2 |
| `scheduled_transfer_dialog.go` | 2 |
| `scheduled_preview_dialog.go` | 2 |
| `split_dialog.go` | 2 |
| `paycheck_wizard.go` | 2 |
| `loan_wizard.go` | 2 |
| `transaction_dialog.go` | 1 (+ the router) |

**16 functions across 8 files**, plus the 8-arm router at
`create_category_dialog.go:271-308`. Counting the router and its helpers, the
whole cluster is **22 `*App` methods across 9 files**.

Two details make it worse than the file layout suggests:

- **The router does not live in its own file.** `handleCreateCatDialogKey`,
  `cancelCreateCatDialog`, `submitCreateCatDialog` and
  `parentsForCreateCatDialog` are all in **`transaction_dialog.go:533-633`**,
  not in `create_category_dialog.go`. So the 308-line file holds 2 of the
  cluster's 22 methods. Anyone sizing this surface by its filename will be
  wrong by an order of magnitude.
- **Three of its five fields are typed handles into other surfaces' guts.**
  `createCatPaycheckLine` is a `*PaycheckLine`; `createCatSplitRow` indexes
  `splitDialog.rows`; `createCatLoanField` indexes `loanWizard.Fields()`. It is
  not a surface with dependencies — it is a surface that reaches inside three
  others.

Anything that touches this touches every transaction-entry surface at once.
Last in phase 3; never a pilot. Moving the four misfiled router methods into
`create_category_dialog.go` is a free, compiler-proven first step, and worth
doing whenever the file is next open.

### Note — surface **file** size is 4d, and it is the cheap one

Nothing in phases 0–5 makes `paycheck_wizard.go` smaller. Saying so plainly
matters, because item 4 named that file first and a reader will check.

The reason is measurable. These files are god files in the half that has nothing
to do with `App`:

| File | Lines | Funcs | On `*App` | On the type / free |
|---|---|---|---|---|
| `paycheck_wizard.go` | 1,886 | 77 | 7 | **70** |
| `loan_wizard.go` | 1,288 | 31 | 15 | 16 |
| `split_dialog.go` | 1,172 | 38 | 5 | **33** |
| `scheduled_preview_dialog.go` | 1,150 | 27 | 11 | 16 |

A complete 4c extraction of `paycheck_wizard.go` moves its `App` region (line
1,463 onward, ~424 lines) and leaves **1,462 lines**. Still a god file.

The fix for that is a **file split** — the same compiler-proven motion item 3
used in its phases 1b and 2, where `transaction` and `corporate_action` went
from 2 files to 12 with byte-identical bodies. `paycheck_wizard.go` already
clusters cleanly for it:

| Cluster | Lines | Funcs |
|---|---|---|
| declarations + misc | 405 | 26 |
| render | 389 | 6 |
| model / rows | 297 | 21 |
| input / focus | 289 | 12 |
| `App` glue | 270 | 7 |
| build / derive | 112 | 5 |

Five or six files of 270–405 lines. No design risk, no test churn, no dependency
on 4a or 4c. **4d could ship before this design does.** It is listed as a
non-goal here only to keep one change reviewable — not because it is hard.

---

## 5. Risks the phases must handle

Each of these is a real property of the current code, verified. None is a reason
not to proceed; each is a reason a specific phase needs a specific test.

### 5.0 The typed-nil trap — the crash phase 1 would introduce

This is the sharpest hazard in the design, and it is one the current code is
accidentally immune to.

A nil `*dialog.Dialog` stored in a `Modal` interface value is **not a nil
interface**. Calling through it dereferences the nil receiver. Three of the four
`IsVisible` implementations do exactly that:

| Type | `IsVisible` body | Nil-safe? |
|---|---|---|
| `*dialog.Dialog` (`dialog/dialog.go:90`) | `return d.visible` | **No** |
| `*SplitDialog` (`split_dialog.go:207`) | `return sd.visible` | **No** |
| `*SchedulePreviewDialog` (`:457`) | `return p.headerDialog != nil && …` | **No** (nil `p`) |
| `*PaycheckWizard` (`:437`) | `return w != nil && w.visible` | **Yes** |

Today the **31 hand-written `X != nil && X.IsVisible()` gates are the nil
guard.** A registry deletes all of them. And `NewApp` initializes only 29 of
`App`'s 160 fields — every modal handle starts nil — so a naive registry would
panic on the first keypress after start-up, before the user has opened anything.

Verified empirically, not reasoned about. A scratch test stored a nil
`*SplitDialog` in an interface:

```
interface holding a nil *SplitDialog is NOT nil, as expected
PANIC as predicted: runtime error: invalid memory address or nil pointer dereference
PaycheckWizard.IsVisible survives a nil receiver
```

Two fixes, and phase 1 must pick one **in writing**:

1. `modals()` skips entries whose handle is nil — the nil check moves from 31
   sites to one; or
2. every `Modal` implementation is made nil-safe, as `PaycheckWizard` already
   is (`w != nil && w.visible`).

Option 2 is preferable: it makes the interface safe by construction rather than
relying on one correct call site, and it is a three-line change to three types
that phase 1 is already editing.

Either way phase 1 owes the guard: **`modals()` must be walkable on a zero
`App`.** That test is ~15 lines and it protects against a crash, not a style
violation, which makes it the highest-value test in §6.1.

**The rule outlives phase 1, and phase 3 is where it is most likely to be
forgotten.** After phase 3 the registry holds `*sellSurface`, `*transferSurface`
and friends, not bare `*dialog.Dialog`. That reintroduces the trap in two new
shapes:

- a **nil `*sellSurface`** stored in a `Modal` interface, and
- a **non-nil surface whose inner `*dialog.Dialog` is nil** — the common case,
  since a surface struct will exist before its dialog is built.

So the nil-safety obligation applies to every surface struct's `IsVisible`,
`HandleKey`, `HandleMouse` and `Render`, not only to the three pre-registry
types. The idiom is `PaycheckWizard`'s:

```go
func (s *sellSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }
```

— which needs `*dialog.Dialog.IsVisible` to be nil-safe too, i.e. option 2 taken
at the `dialog` package level. Phase 3's exit criteria carry this; guard 1 is
re-run against the post-phase-3 registry, not retired after phase 1.

### 5.1 Rendering writes state that mouse hit-testing reads

`overlayDialog` calls `d.SetMaxHeight(a.dialogMaxHeight())` as a side effect of
painting (`app_view.go:246`). `HandleMouse` then computes geometry through
`DialogBounds` → `RenderedHeight()`, which reads that same `maxHeight`
(`dialog/mouse.go:275`, `dialog/render.go:893`).

So a tall dialog's click coordinates are correct **only because** the paint pass
ran first. Phase 1 must keep the write in the paint walk; phase 2 must not
assume `HandleMouse` is independent of it. A regression here misplaces clicks in
a Sell dialog with many lots, and no test would catch it today.

### 5.2 Saved messages arrive after the dialog is gone

Every submit closes its dialog **synchronously**, then returns the async
command. `submitStockSplitDialog` calls `closeStockSplitDialog()`
(`corporate_action_split_dialog.go:274`), which nils three fields, and only then
returns the goroutine that emits `stockSplitDialogSavedMsg`
(`app_update.go:328`). Sixteen arms share this shape.

Therefore: **the registry must not route messages.** "Send the message to the
active modal" would silently drop every saved message, because by then there is
no active modal. Message handling stays a switch on `App`. §2.3 states this as a
property of the design, not an omission.

### 5.3 Styles is a value, mutated in place

`styles widget.Styles` (`app.go:109`) is mutated through pointer-receiver
methods on the addressable field: `a.styles.Resize` on every `WindowSizeMsg`
(`app_update.go:21`) and `a.styles.ApplyTheme` on theme load (`app.go:503`,
`app_theme.go:49`). Passing styles per call is what keeps this correct. Do not
cache styles in a surface struct in phase 3.

Related: `widget/styles.go:45-122` declares ~22 package-level mutable colour
vars that `ApplyTheme` reassigns. `internal/tui/test_helpers_test.go` exists
only to undo that leak between tests. There are zero `t.Parallel()` calls in
the TUI tree. Keep it that way.

### 5.4 `showConfirmDialog` carries a continuation

`confirmAction func() tea.Msg` (`app.go:405`) holds a closure over `*App` and
its services, from nine call sites. It is the destructive-action gate for void,
delete, restore-from-backup, corporate-action reversal, security delete, price
delete and the loan-demotion warning.

It is also the only existing example of the shape a decomposed TUI wants: a
surface *asks* for something and `App` executes it. Treat it as the seed to
generalize, not as an obstacle. It stays as-is through phase 4; phase 5's pilot is where the shape gets
generalized, if it does.

### 5.5 `switchDatabase` re-points services; nothing else is notified

`file_dialog.go:230-247` reassigns 18 service and repository fields when the
user opens another file, and `backup_dialog.go:126-140` reaches
`switchDatabase` by a second route after a restore. The previous `*db.DB` is
kept as `prevDB` and closed on the next switch (`file_dialog.go:222`).

Commit `6dede4d` ("re-point every service when a file is opened") fixed exactly
this class of bug. A surface struct that captures service pointers at open time
would reintroduce it, and the stale pointer would be a use-after-close, not just
stale data. **Phase 3 surface structs must not hold services.** They hold form
state. `App` keeps the services and passes them in at call time.

**And the existing regression test will not tell you if you get this wrong.**
`6dede4d` shipped a structural guard, `internal/tui/switch_database_test.go`,
which reflects over `app.Services` to learn the service pointer types and then
walks `App` looking for stale ones. It walks `appT.NumField()` — **`App`'s
top-level fields only.** It does not recurse into struct-typed fields. So the
moment phase 3 moves a service pointer inside `sellSurface`, that pointer
silently leaves the guard's coverage and the test still passes.

The guard has an anti-vacuity check (`t.Fatal` if it matched zero fields), which
is good, but that only proves *some* fields matched — not that the ones you just
moved are still among them. So phase 3 owes one of two things, and must say
which it chose:

1. Extend the guard to recurse one level into struct-typed `App` fields; or
2. Add a guard asserting **no surface struct has a service-typed field at all**,
   which is the stronger rule and matches the exit criterion above.

Option 2 is preferable: it is the invariant this design actually wants, and it
cannot rot into a partially-covering walk.

`undoManager` is a related hazard: it is assigned once in `NewApp` (`app.go:454`)
and `switchDatabase` never re-points or clears it, so the undo stack outlives
the database its commands were built against. That is a pre-existing bug, out of
scope here, and it belongs with review item 5 (§8).

### 5.6 Command closures read mutable `App` state on another goroutine

`loadSellDialogData` returns a closure that reads `a.investmentEditTxnID` and
`a.investmentRegister.account` (`investment_sell_dialog.go:146, 137`) while
`Update` writes `a.investmentEditTxnID` on the bubbletea goroutine
(`app_update.go:158, 188, 217, 250, 270, 311, 495`). The same file already shows
the correct pattern 200 lines later — `editTxnID := a.investmentEditTxnID`
captured synchronously at `:355`.

64 `return func() tea.Msg` closures capture `a`. This refactor does not create
the race and does not have to fix it, but phase 4 should not make it harder to
see. Where a phase touches one of these closures, capture the values
synchronously.

### 5.7 Nothing detects a visual regression

There are no golden or snapshot files anywhere under `internal/tui` — the only
`testdata` directory is `theme/testdata`. Render coverage is substring
assertions against unexported `render*` methods. A pure code-motion refactor can
break spacing, centring, scrollbars or z-order with a fully green
`go test ./...`.

The repository's answer to this is a manual visual smoke check as a first-class
milestone: `MS-014`, `MS-023`, `MS-030` and `PW2-010` are the only unchecked
items in the whole `specs/` tree. Schedule one per phase, and make it a named
checklist rather than "look at it", because a vague smoke step is one nobody
can fail:

| Phase | Smoke checklist |
|---|---|
| 0 | File → Import runs end to end; Transactions → Link Transfers previews and confirms |
| 1 | Create-category opens from each of its 8 originators and returns to the right field; confirm-over-scheduled stacks correctly; help overlay still paints on top |
| 2 | Click Cancel and press Esc on the same dialog and compare the resulting state; click a row in a Sell dialog with enough lots to scroll (the §5.1 hit-test path) |
| 3 | Every dialog still opens, saves and cancels after its fields move into a struct |
| 4 | Every `*SavedMsg` still produces its status-bar notification and view reload |
| 5 | The transfer dialog: new, edit, cancel, undo; then File → Open another file and repeat |

### 5.8 The transfer arch guard scans `internal/tui`

`internal/transfer/arch_test.go:41-68` walks every non-test `.go` file under
`internal/` and `cmd/`, skipping only `internal/transfer/**` and
`internal/scheduled/transfer_port.go`. It fails on any line matching a regex of
banned method names, including `CreateTransfer`, `UpdateTransfer`,
`DeleteTransfer` and `TransferPair`.

A transfer surface struct with a natural method name fails that guard, and the
failure reads as unrelated. Name around it.

---

## 6. Exit criteria

Per phase, checkable:

| Phase | Exit criteria |
|---|---|
| 0 | Both dialogs render in a test; **manual smoke of File → Import and Transactions → Link Transfers end to end**; the four `NewServices`-in-a-goroutine sites filed against item 6; a recorded decision for `corporateActionDetail`; every modal name appears in all four lists |
| 1 | `Modal` declared and the §5.0 nil decision recorded; `handleKeyPress` and `renderLayout` contain no per-dialog `if` — **the mouse cascade is untouched until phase 2**; the §1.5 co-occurrence tests pass; §6.1 guards 1–3 land with this phase; smoke checklist |
| 2 | `handleDialogMouse` contains no per-dialog `if`; every mouse Cancel routes through the same `onCancel` as Esc; the pre-audited test assertions updated deliberately; visual smoke check |
| 3 | `App` under ~100 fields; each surface's state is one field; **no surface struct holds a service pointer** (§5.5); every surface struct is nil-safe and guard 1 is re-run against the new registry (§5.0); guard 2's filter updated in the same commit as the first surface struct (§6.1); `createCatOrigin` replaces the five scratch fields |
| 4 | `app_update.go` under ~300 lines; `*App` gains **only the documented pinned arms (≤ 41), never the full 61** — landing near ~398, not ~418; each arm that stayed is listed with its reason |
| 5 | The transfer surface owns `open`/`submit`/`close`; its registry entry is a thin wrapper, not a second implementation (§2.2); `App` supplies only services; the deps are a live indirection, not captured pointers; the inherited undo/DB mismatch is noted in the commit, not treated as a regression |

**What these criteria deliberately do not claim.** No phase reduces
`paycheck_wizard.go`, `loan_wizard.go`, `split_dialog.go` or
`scheduled_preview_dialog.go` below 1,000 lines. Those counts are unchanged at
the end of phase 5, and the note at the end of §4 explains why: they are 4d, they are cheap, and they
are independent. A closeout that quietly implied otherwise would be the kind of
thing item 3's §11.1 refused to do when it named `backfill.go` at 562 lines
rather than redefining its own ceiling.

### 6.1 Guards against a fifth list

The failure mode this design most needs to prevent is its own regression: a
future dialog added to `modals()` *and* to a new bespoke `if` somewhere.
`go test -p 8 ./...` is the entire gate — `.golangci.yml` is 18 lines of
`errcheck` exclusions, the `Makefile` has no lint target, and `.github/` holds
only `dependabot.yml`. So the guard has to be a test.

The in-repo pattern is `internal/transfer/arch_test.go`, including its
convention of a self-test that proves the guard fires. Three guards are worth
writing, in this order:

**Guard 1 — `modals()` survives a zero `App` (~15 lines).** Write this first.
It protects against the §5.0 crash, not a style rule, and it is the cheapest
test in the set. Construct an `App` with no dialogs built and walk the registry
calling `IsVisible` on every entry. A panic fails the test with a message
naming the two acceptable fixes.

**Guard 2 — every modal field is in the registry, exactly once.** This is the
one that would have caught §1.3. It must not be written against a
hand-maintained list of expected names; that is a fifth list. Build both sides
mechanically:

- The **must-be-registered** set from `reflect.TypeFor[App]()`, filtering fields
  whose type is `*dialog.Dialog` or one of the bespoke surface types. Reflection
  over a struct's unexported field *names and types* is fine from inside the
  package — the restriction is on `Set` and `Interface()`, which panic even
  in-package, so do not try to populate `App` reflectively and compare pointer
  identity.
- The **is-registered** set by parsing `modals()` with `go/ast`.

There are **26 `*dialog.Dialog` fields on `App`, all of them modal**, so the
type filter needs no exception list.

**Guard 2's filter must change in the same commit as the first surface struct.**
Written for phase 1, the must-register set is "fields of type `*dialog.Dialog`
or one of the bespoke types". After phase 3 those fields are gone — they live
inside `*sellSurface` and friends — so a filter frozen at the phase-1 shape
matches nothing and passes vacuously on day one of phase 3. Two ways out:

1. Land guard 2 only after phase 3, keyed to the final shape; or
2. Land it in phase 1 keyed to "fields whose type implements `Modal`", and
   update the filter in the same commit that introduces the first surface
   struct.

Option 2 is better — the guard earns its keep during phases 1–2, which is when
the four lists are being merged — but it only works if the anti-vacuity check
is real: **fail the test when the filter matches zero fields**, exactly as
`switch_database_test.go` already does (`t.Fatal` on an empty set). Without
that, the filter's own obsolescence is invisible.

**Guard 3 — the self-tests.** The precedent teaches a sharper lesson than "write
a self-test". `internal/transfer/arch_test.go:187` proves a *copy* of the guard
fires, and that copy has drifted: it exercises 9 of the 15 banned names. So
factor each guard into a **pure function over source text**, with the file walk
as a thin wrapper, and have the self-test call the same function with fabricated
input. That is the precedent's "the regex is a value" trick, one level up.

**Two guards to skip.** "The key walk and the paint walk use the same slice" is
theatre as stated — a fresh slice per call is unobservable from outside. Its
non-theatre replacement is a pairwise order assertion generated from `modals()`
itself, which is worth having and is really part of the §1.5 co-occurrence
tests. And "no per-dialog `if` in `handleKeyPress`" adds nothing that guard 2
does not already prove. (Note for whoever writes it: `handleMouseEvent` has zero
per-dialog `if`s today — all 30 are in `handleDialogMouse`.)

---

## 7. Cost

Item 3's §11.2 recorded that its +100-line estimate came in at +432, because a
decomposition is mostly an opportunity to write down why each cluster is a
cluster. That lesson is applied here rather than repeated.

| Phase | Production lines | Test lines | Note |
|---|---|---|---|
| 0 | +10 | +60 | bug fix |
| 1 | −250 net, +300 comments | +250 | the three type fixes are moves; §6.1 guards |
| 2 | −180 net | +100 | plus edits to existing assertions |
| 3 | +150 | churn in ~40 test files | struct declarations and their doc comments |
| 4 | −400 out of `app_update.go` | 0 | bodies land on the surface structs |
| 5 | ~0 net | ~24 literals in the transfer tests | one surface only |

Expect the totals to land above these. The deletion that is real and predictable
is the cascade collapse: 122 hand-written entries across four files become three
loops and one ordered slice.

The full TUI suite runs in 25.7s uncached, so the feedback loop is not a
constraint on phase size.

---

## 8. Deferred, with a decision

- **Packages under `tui/`.** Decided against for now (§3). Phase 5 is the
  precondition, not the decision: a surface with no `*App` methods left is a
  package candidate on its own numbers. Re-open it then — not from the old
  review.
- **Surface file splits (4d).** Not in these phases, and cheap (see the note at
  the end of §4). It can run in parallel, under one rule:

  > **4d may move functions between files inside `package tui`. It may not
  > rename a field, change a signature, or touch a surface that a live phase is
  > editing.**

  Under that rule `paycheck_wizard.go`, `loan_wizard.go` and `split_dialog.go`
  are free to split at any time — phases 0–2 do not touch their fields, and
  phase 3 renames fields rather than moving functions, so the two changes hit
  different lines. The exception is **transfer during phase 5**: that surface is
  being restructured, so do not split its file concurrently. Same for whichever
  surface phase 3 is mid-conversion on.
- **The view layer (4b).** Out of scope, and a real second design. `price_view.go`
  and `investment_register_view.go` are the two files that most deserve it, and
  `switchView`'s 12-arm focus switch is the seam.
- **`undoManager` is never re-pointed on a database switch** (§5.5). A
  pre-existing bug found while planning this. It belongs with
  **review item 5** (boundary leaks / `app.Services`), whose owner will already
  be touching the composition root — not here.
- **`app.NewServices` called inside four `tea.Cmd` goroutines**
  (`import_dialog.go:373, 425`; `link_transfers_dialog.go:31, 45`). Each builds
  a second registry and re-runs the write side effects — `EnsurePaycheckCategories`
  (`registry.go:101`), `EnsureValueAdjustmentCategory` (`:105`),
  `HealAllAccounts` (`:131`), `HealNextDates` (`:148`) — so opening the import
  preview silently heals scheduled dates and investment accounts. Each also
  reads `a.db` at goroutine time rather than capture time (§5.6). This is
  **review item 6** ("`NewServices` mutates the database as a constructor side
  effect") reaching into the TUI.

  These four sites bypass `newTUIServices` and so lack the Yahoo price provider,
  but that is harmless here — neither import nor transfer-linking fetches
  prices. The heal-on-open re-run and the `a.db` read are the real defects.

  Out of scope for this design, but **phase 0 is what makes these paths
  reachable**, so phase 0 owes the item-6 follow-up before it merges (see
  phase 0).
- **`App` holds 17 service fields rather than one `*app.Services`.** Collapsing
  them would make `switchDatabase`'s 18-line re-point block one assignment and
  the `6dede4d` bug class structurally impossible. This is **review item 5**'s
  territory. Attractive, orthogonal, and not part of item 4 — but phase 5's
  deps design should be written so it does not have to change if item 5 later
  collapses those 17 fields into one.
- **`themeReloadFailedMsg` is emitted (`app_theme.go:44`) with no case arm**, so
  the error is dropped. One-line fix; belongs with phase 3, which touches the
  switch.
