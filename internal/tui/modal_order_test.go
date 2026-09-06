package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// newModalTestApp returns an App with the chrome the walks need and every
// modal handle nil.
func newModalTestApp() *App {
	return &App{
		currentView: ViewDashboard,
		keys:        defaultKeyMap(),
		statusbar:   widget.NewStatusBar(),
		menubar:     widget.NewMenuBar(),
		sidebar:     NewSidebar(),
		styles:      widget.NewStyles(),
		width:       120,
		height:      40,
		ready:       true,
	}
}

func visibleDialog(title string) *dialog.Dialog {
	d := dialog.NewDialog(title)
	d.SetWidth(40)
	d.SetVisible(true)
	return d
}

// showSurface makes the named registry surface visible on app. The names match
// modalEntry.name. Keeping the setters in one table means the order and
// co-occurrence tests below cannot drift from the registry: surfaceSetters is
// checked against modals() for exact coverage by
// TestModals_EveryEntryHasATestSetter.
func surfaceSetters() map[string]func(*App) {
	return map[string]func(*App){
		"help":                   func(a *App) { a.showHelp = true },
		"confirm":                func(a *App) { a.confirmDialog = visibleDialog("Confirm") },
		"about":                  func(a *App) { a.aboutDialog = visibleDialog("About") },
		"backup":                 func(a *App) { a.backupDialog = &backupDialogState{dialog: visibleDialog("Backup")} },
		"file":                   func(a *App) { a.fileDialog = visibleDialog("File") },
		"import":                 func(a *App) { a.importDialog = visibleDialog("Import") },
		"linkTransfers":          func(a *App) { a.linkTransfersDialog = visibleDialog("Link") },
		"split":                  func(a *App) { a.splitDialog = newVisibleSplitDialog() },
		"createCategory":         func(a *App) { a.createCatDialog = visibleDialog("Create Category") },
		"transaction":            func(a *App) { a.txnDialog = visibleDialog("Transaction") },
		"transfer":               func(a *App) { a.transferDialog = visibleDialog("Transfer") },
		"scheduled":              func(a *App) { a.schedDialog = visibleDialog("Scheduled") },
		"schedulePreview":        func(a *App) { a.schedPreviewDialog = &SchedulePreviewDialog{headerDialog: visibleDialog("Preview")} },
		"paycheckWizard":         func(a *App) { a.paycheckWizard = newVisiblePaycheckWizard() },
		"loanWizard":             func(a *App) { a.loanWizard = visibleDialog("Loan") },
		"account":                func(a *App) { a.acctDialog = visibleDialog("Account") },
		"reconciliation":         func(a *App) { a.reconDialog = visibleDialog("Reconcile") },
		"closeAccount":           func(a *App) { a.closeAcctDialog = visibleDialog("Close Account") },
		"security":               func(a *App) { a.securityDialog = visibleDialog("Security") },
		"price":                  func(a *App) { a.priceDialog = visibleDialog("Price") },
		"priceImport":            func(a *App) { a.priceImportDialog = visibleDialog("Price Import") },
		"buy":                    func(a *App) { a.buyDialog = visibleDialog("Buy") },
		"sell":                   func(a *App) { a.sellDialog = visibleDialog("Sell") },
		"feeLiquidation":         func(a *App) { a.feeLiquidationDialog = visibleDialog("Fee") },
		"dividend":               func(a *App) { a.dividendDialog = visibleDialog("Dividend") },
		"transferShares":         func(a *App) { a.transferSharesDialog = visibleDialog("Transfer Shares") },
		"stockSplit":             func(a *App) { a.stockSplitDialog = visibleDialog("Stock Split") },
		"mergerConfirm":          func(a *App) { a.mergerConfirmData = &mergerConfirmData{} },
		"merger":                 func(a *App) { a.mergerDialog = visibleDialog("Merger") },
		"spinOff":                func(a *App) { a.spinOffDialog = visibleDialog("Spin-off") },
		"cashOperation":          func(a *App) { a.cashOperationDialog = visibleDialog("Cash") },
		"investmentTypeSelector": func(a *App) { a.investmentTypeSelector = visibleDialog("Type") },
	}
}

func newVisibleSplitDialog() *SplitDialog {
	sd := NewSplitDialog(types.MustNewMoney("10.00"), []string{"Groceries"}, []types.ID{types.NewID()})
	sd.SetVisible(true)
	return sd
}

func newVisiblePaycheckWizard() *PaycheckWizard {
	w := NewPaycheckWizard([]string{"Salary"}, []types.ID{types.NewID()}, nil)
	w.SetVisible(true)
	return w
}

// TestModals_EveryEntryHasATestSetter keeps the fixture honest. Without it, a
// surface added to the registry would silently escape every order and
// co-occurrence assertion below.
func TestModals_EveryEntryHasATestSetter(t *testing.T) {
	setters := surfaceSetters()
	entries := newModalTestApp().modals()
	if len(entries) == 0 {
		t.Fatal("modals() is empty; the coverage check below would pass vacuously")
	}
	for _, e := range entries {
		if _, ok := setters[e.name]; !ok {
			t.Errorf("registry surface %q has no setter in surfaceSetters()", e.name)
		}
	}
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		if seen[e.name] {
			t.Errorf("duplicate registry entry %q", e.name)
		}
		seen[e.name] = true
	}
	for name := range setters {
		if !seen[name] {
			t.Errorf("setter %q names no registry surface", name)
		}
	}
}

// TestModals_WalkableOnAZeroApp is guard 1 of the design's section 6.1, and the
// highest-value test in the set: it protects against a crash, not a style
// violation.
//
// A nil *dialog.Dialog stored in a Modal is NOT a nil interface, so calling
// through it dereferences a nil receiver. The 31 hand-written
// `X != nil && X.IsVisible()` gates used to be that nil guard, and the registry
// deleted all of them. NewApp initialises only a handful of App's fields, so
// every modal handle starts nil — a registry that was not nil-safe would panic
// on the first keypress after start-up, before the user opened anything.
//
// If this panics, the two acceptable fixes are: make modals() skip entries
// whose handle is nil, or make every Modal implementation nil-safe. This
// repository chose the second — see dialog.Dialog.IsVisible.
func TestModals_WalkableOnAZeroApp(t *testing.T) {
	a := &App{}
	for _, e := range a.modals() {
		if e.modal.IsVisible() {
			t.Errorf("surface %q reports visible on a zero App", e.name)
		}
	}
	if a.isDialogVisible() {
		t.Error("isDialogVisible must be false on a zero App")
	}
	if got := a.frontmostModal(); got != nil {
		t.Errorf("frontmostModal on a zero App = %q, want nil", got.name)
	}
}

// TestModals_EachSurfaceAloneIsFrontmost pins that every surface can actually
// win the key when it is the only one open. A surface that never becomes
// frontmost is unreachable — the shape of the phase 0 bug.
func TestModals_EachSurfaceAloneIsFrontmost(t *testing.T) {
	for name, show := range surfaceSetters() {
		t.Run(name, func(t *testing.T) {
			a := newModalTestApp()
			show(a)

			if !a.isDialogVisible() {
				t.Fatalf("isDialogVisible is false with %q open", name)
			}
			front := a.frontmostModal()
			if front == nil {
				t.Fatalf("no frontmost modal with %q open", name)
			}
			if front.name != name {
				t.Errorf("frontmost = %q, want %q", front.name, name)
			}
			if front.onKey == nil {
				t.Errorf("surface %q has no key handler", name)
			}
		})
	}
}

// rank returns a surface's index in the registry: lower takes keys first and
// paints on top.
func rank(t *testing.T, name string) int {
	t.Helper()
	for i, e := range newModalTestApp().modals() {
		if e.name == name {
			return i
		}
	}
	t.Fatalf("no registry surface named %q", name)
	return -1
}

// assertStacks pins one co-occurring pair: with both surfaces visible, `over`
// must take the key and paint on top of `under`.
//
// This is what makes collapsing four hand-maintained lists into one ordered
// slice safe. The old lists ordered the surfaces three different ways, so the
// question is not "do the orders match" — they did not — but "do they differ on
// any pair that can be visible at once". Enumerated from the code, only these
// can co-occur; every other disagreement is unobservable.
func assertStacks(t *testing.T, over, under string) {
	t.Helper()
	a := newModalTestApp()
	setters := surfaceSetters()
	setters[under](a)
	setters[over](a)

	if front := a.frontmostModal(); front == nil || front.name != over {
		got := "none"
		if front != nil {
			got = front.name
		}
		t.Errorf("%q over %q: key went to %q", over, under, got)
	}
	if ro, ru := rank(t, over), rank(t, under); ro >= ru {
		t.Errorf("%q (rank %d) must outrank %q (rank %d) so it paints on top", over, ro, under, ru)
	}
}

// showConfirmDialog is the one true stack: it leaves the surface underneath
// visible (unlike the create-category divert and the merger confirmation,
// which both hide their originator first). Nine call sites across seven files
// reach it, so confirm must outrank everything it can cover.
func TestModals_ConfirmStacksOverTheSurfaceUnderneath(t *testing.T) {
	for _, under := range []string{"scheduled", "split", "transaction", "security", "price"} {
		t.Run(under, func(t *testing.T) { assertStacks(t, "confirm", under) })
	}
}

// The help overlay is reachable from anywhere and swallows every key, so it
// outranks everything including confirm.
func TestModals_HelpStacksOverEverything(t *testing.T) {
	entries := newModalTestApp().modals()
	if entries[0].name != "help" {
		t.Fatalf("help must be the first registry entry, got %q", entries[0].name)
	}
	assertStacks(t, "help", "confirm")
}

// TestModals_CreateCategoryOutranksItsOriginators covers the divert. All eight
// originating surfaces hide themselves before showing the sub-dialog, so this
// ordering is belt-and-braces rather than load-bearing — except for the split
// dialog, which outranks create-category and would paint over it if it ever
// failed to hide. The next test pins that hiding.
func TestModals_CreateCategoryOutranksItsOriginators(t *testing.T) {
	for _, under := range []string{"transaction", "transfer", "scheduled", "schedulePreview", "paycheckWizard", "loanWizard"} {
		t.Run(under, func(t *testing.T) { assertStacks(t, "createCategory", under) })
	}
}

// TestModals_SplitOutranksCreateCategory states the exception explicitly, so a
// future reader does not "fix" the order. Split takes keys before
// create-category, which is only safe because split hides itself on divert.
func TestModals_SplitOutranksCreateCategory(t *testing.T) {
	assertStacks(t, "split", "createCategory")
}

// The merger confirmation is a swap, not a stack: submitMergerDialog closes the
// merger dialog before loading the confirmation. The ordering is asserted
// anyway because the confirmation is the surface that must win if they ever do
// overlap.
func TestModals_MergerConfirmOutranksMergerDialog(t *testing.T) {
	assertStacks(t, "mergerConfirm", "merger")
}
