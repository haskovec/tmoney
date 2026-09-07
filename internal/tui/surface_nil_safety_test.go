package tui

import (
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/widget"
)

// The two panics found while rebasing the surface structs, pinned so they
// cannot come back: a submit that drops its surface and then reads it, and an
// applier that reaches through a surface the user may already have closed.

// TestSecurityEditSubmit_UsesTheCapturedEditID: submitSecurityDialog must read
// the surface's mode and edit target BEFORE setting the surface to nil. The
// first surface-struct draft read them after, which dereferenced nil on every
// save. Driving a real edit through to the database is what proves the captured
// id reached the update.
func TestSecurityEditSubmit_UsesTheCapturedEditID(t *testing.T) {
	database := dbtest.New(t)
	svc := security.NewService(security.NewRepository(database), database)

	sec := security.NewSecurity("AAPL", "Apple Inc.", security.TypeStock)
	if err := svc.Create(sec); err != nil {
		t.Fatalf("create security: %v", err)
	}

	app := &App{statusbar: widget.NewStatusBar(), securitySvc: svc}
	d := buildEditSecurityDialog(sec)
	d.SetVisible(true)
	app.security = securitySurface{modalSurface: modalSurface{dlg: d}, mode: securityDialogModeEdit, editID: sec.ID}
	d.Fields()[1].Value = "Apple Incorporated"

	_, cmd := app.submitSecurityDialog()
	if app.security.dlg != nil {
		t.Fatal("submit must drop the surface")
	}
	if cmd == nil {
		t.Fatal("submit must return the update command")
	}
	if msg, ok := cmd().(securityUpdatedMsg); !ok {
		t.Fatalf("update command returned %T, want securityUpdatedMsg", msg)
	}

	got, err := svc.GetByID(sec.ID)
	if err != nil {
		t.Fatalf("reload security: %v", err)
	}
	if got.Name != "Apple Incorporated" {
		t.Errorf("name after edit = %q, want %q (the update did not reach the captured edit id)", got.Name, "Apple Incorporated")
	}
}

// TestApplyCreatedCategoryToLoan_ToleratesAClosedWizard: the category persist is
// asynchronous, so the wizard can be gone by the time the applier runs. It must
// clear the sub-dialog and return, not dereference the nil surface.
func TestApplyCreatedCategoryToLoan_ToleratesAClosedWizard(t *testing.T) {
	app := &App{
		createCat: createCatSurface{origin: createCatOrigin{surface: createCatSourceLoanWizard,
			loanField: 3},

			modalSurface: modalSurface{dlg: buildCreateCategoryDialog("Escrow", "", nil, category.TypeExpense)}},
	}
	app.loan = loanSurface{}

	newCat := category.NewCategory("Escrow", category.TypeExpense)
	app.applyCreatedCategoryToLoan(newCat, []*category.Category{newCat})

	if app.createCat.dlg != nil {
		t.Error("the applier must clear the create-category sub-dialog when the wizard is gone")
	}
	if app.createCat.origin.loanField != -1 {
		t.Errorf("createCatLoanField = %d, want -1 after the applier gives up", app.createCat.origin.loanField)
	}
}

// The three submit entry points that read the loan surface, and the import and
// link-transfers ones, must be no-ops on a nil surface rather than panics. The
// dispatcher only reaches them while the surface is visible, so this is the
// contract for the next async or test caller, not a production path today.
func TestSubmitPaths_AreNoOpsOnANilSurface(t *testing.T) {
	app := &App{statusbar: widget.NewStatusBar()}
	for name, fn := range map[string]func() (any, any){
		"submitLoanWizard":          func() (any, any) { return app.submitLoanWizard() },
		"submitNewLoanWizard":       func() (any, any) { return app.submitNewLoanWizard() },
		"submitEditLoanWizard":      func() (any, any) { return app.submitEditLoanWizard() },
		"submitImportDialog":        func() (any, any) { return app.submitImportDialog() },
		"submitLinkTransfersDialog": func() (any, any) { return app.submitLinkTransfersDialog() },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s panicked on a nil surface: %v", name, r)
				}
			}()
			fn()
		})
	}
}
