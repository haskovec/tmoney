package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// importDialogStep is the current step of the import workflow.
type importDialogStep int

const (
	importStepOptions importDialogStep = iota
	importStepSourcePicker
	importStepConfirm
)

// Format options shown in the dialog. The first entry means auto-detect.
var importFormatOptions = []string{"Auto-detect", "CSV", "QIF", "OFX/QFX"}

// Duplicate handling options shown in the dialog.
var importDuplicateOptions = []string{
	"None (no duplicate detection)",
	"Skip duplicates",
	"Update duplicates",
}

// importDialogState holds state shared across the import dialog steps.
type importDialogState struct {
	step       importDialogStep
	accountIDs []types.ID
	// Captured from the options dialog — needed when we run the preview cmd
	// and again at confirm time to summarise to the user.
	filePath          string
	accountID         types.ID
	accountName       string
	format            imexport.Format
	duplicateHandling imexport.DuplicateHandling
	// sourceAccount is set when the user picks one source account out of a
	// multi-account CSV. When non-empty the preview is filtered to only
	// rows whose Account column matches.
	sourceAccount string
	// sourceOptions is populated when the parse step finds multiple source
	// accounts in the file; the picker dialog renders these.
	sourceOptions []string
	// Result of the parse + match preview, populated before showing the
	// confirm dialog.
	preview *imexport.ImportResult
}

// importPreviewedMsg is sent when the import preview has finished parsing
// and matching the file.
type importPreviewedMsg struct {
	state  *importDialogState
	result *imexport.ImportResult
}

// importNeedsSourceMsg is sent when the parsed file contains rows for more
// than one source account; the user must pick which one to import before
// the preview can run.
type importNeedsSourceMsg struct {
	state   *importDialogState
	sources []string
}

// importCompletedMsg is sent when the import has been executed.
type importCompletedMsg struct {
	created int
	updated int
	skipped int
	errors  []string
}

// buildImportOptionsDialog creates the first-step dialog asking for file
// path, target account, format, and duplicate handling.
func buildImportOptionsDialog(accounts []*account.Account, defaultAccountID types.ID) (*dialog.Dialog, []types.ID) {
	d := dialog.NewDialog("Import Transactions")
	d.SetWidth(64)

	pathField := d.AddTextField("File", "", "Path to .csv, .qif, .ofx or .qfx file", 0)
	pathField.Required = true

	options := make([]string, 0, len(accounts))
	ids := make([]types.ID, 0, len(accounts))
	selected := 0
	for i, a := range accounts {
		options = append(options, a.Name)
		ids = append(ids, a.ID)
		if !defaultAccountID.IsNil() && a.ID == defaultAccountID {
			selected = i
		}
	}
	if len(options) == 0 {
		options = []string{"(no accounts)"}
	}
	d.AddSelectField("Account", options, selected)

	d.AddSelectField("Format", importFormatOptions, 0)
	d.AddSelectField("Duplicates", importDuplicateOptions, 0)

	d.SetButtons([]dialog.DialogButton{
		{Label: "Preview", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	return d, ids
}

// buildImportSourcePickerDialog creates the dialog shown when the file
// contains rows for more than one source account. The user picks exactly
// one source to import in this pass.
func buildImportSourcePickerDialog(sources []string, target string) *dialog.Dialog {
	d := dialog.NewDialog("Pick Source Account")
	d.SetWidth(64)

	d.AddTextField("Importing into", target, "", 0)
	d.AddTextField("File contains", fmt.Sprintf("%d accounts — pick one", len(sources)), "", 0)
	d.AddSelectField("Source account", sources, 0)

	d.SetButtons([]dialog.DialogButton{
		{Label: "Continue", Primary: true},
		{Label: "Cancel"},
	})
	d.SetVisible(true)
	return d
}

// buildImportConfirmDialog creates the second-step dialog showing the
// import preview summary and asking the user to confirm or cancel.
//
// The summary is composed of plain text fields. They are technically
// editable but no submit handler reads them — only state.preview is used
// when the user confirms. Default focus is on the primary button so a
// single Enter confirms.
func buildImportConfirmDialog(state *importDialogState) *dialog.Dialog {
	d := dialog.NewDialog("Confirm Import")
	d.SetWidth(64)

	r := state.preview
	d.AddTextField("File", state.filePath, "", 0)
	if state.sourceAccount != "" {
		d.AddTextField("Source", state.sourceAccount, "", 0)
	}
	d.AddTextField("Target account", state.accountName, "", 0)

	parsed := len(r.Rows)
	dateRange := "(none)"
	if parsed > 0 {
		dateRange = fmt.Sprintf("%s to %s", r.DateFrom.String(), r.DateTo.String())
	}
	d.AddTextField("Parsed", fmt.Sprintf("%d transactions", parsed), "", 0)
	d.AddTextField("Will create", fmt.Sprintf("%d new", r.NewCount()), "", 0)
	d.AddTextField("Will update", fmt.Sprintf("%d matched", r.MatchCount()), "", 0)
	d.AddTextField("Will skip", fmt.Sprintf("%d duplicates", r.SkipCount()), "", 0)
	if r.ReviewCount() > 0 {
		d.AddTextField("Needs review", fmt.Sprintf("%d (will be created)", r.ReviewCount()), "", 0)
	}
	d.AddTextField("Date range", dateRange, "", 0)
	if total := r.TotalAmount(); !total.IsZero() {
		d.AddTextField("Net amount", total.String(), "", 0)
	}
	if len(r.Errors) > 0 {
		d.AddTextField("Parse errors", fmt.Sprintf("%d encountered", len(r.Errors)), "", 0)
		maxN := min(5, len(r.Errors))
		errPreview := strings.Join(r.Errors[:maxN], "; ")
		if len(r.Errors) > maxN {
			errPreview += " …"
		}
		d.AddTextField("First errors", errPreview, "", 0)
	}

	primary := "Import"
	if parsed == 0 {
		primary = "Close"
	}
	d.SetButtons([]dialog.DialogButton{
		{Label: primary, Primary: true},
		{Label: "Cancel"},
	})
	// Focus the primary button so Enter immediately confirms.
	d.SetFocusIndex(len(d.Fields()))
	d.SetVisible(true)
	return d
}

// importFormatFromIndex maps a select-field index to the corresponding
// imexport.Format. Index 0 means auto-detect, in which case the empty
// format is returned and the caller should call imexport.DetectFormat.
func importFormatFromIndex(idx int) imexport.Format {
	switch idx {
	case 1:
		return imexport.FormatCSV
	case 2:
		return imexport.FormatQIF
	case 3:
		return imexport.FormatOFX
	default:
		return ""
	}
}

// importDuplicateFromIndex maps a select-field index to the corresponding
// duplicate handling mode.
func importDuplicateFromIndex(idx int) imexport.DuplicateHandling {
	switch idx {
	case 1:
		return imexport.DuplicateHandlingSkip
	case 2:
		return imexport.DuplicateHandlingUpdate
	default:
		return imexport.DuplicateHandlingNone
	}
}

// startImport opens the import options dialog populated from the current
// account list. Called from the File menu action handler.
func (a *App) startImport() tea.Cmd {
	return func() tea.Msg {
		if a.accountSvc == nil {
			return errMsg{err: fmt.Errorf("account service not available")}
		}
		accounts, err := a.accountSvc.List(true)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to list accounts: %w", err)}
		}
		var defaultID types.ID
		if a.sidebar != nil {
			defaultID = a.sidebar.SelectedAccountID()
		}
		return importDialogOpenMsg{accounts: accounts, defaultAccountID: defaultID}
	}
}

// importDialogOpenMsg carries the loaded account list back to the Update
// loop so the dialog can be built on the main goroutine.
type importDialogOpenMsg struct {
	accounts         []*account.Account
	defaultAccountID types.ID
}

// importSurface is the import workflow's state. One dialog handle serves all
// three steps; state carries the parse and preview results between them.
type importSurface struct {
	modalSurface
	state *importDialogState
}

// IsVisible must be declared here rather than promoted from modalSurface — see
// the note on modalSurface.
func (s *importSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// closeImportDialog clears the import dialog state.
func (a *App) closeImportDialog() {
	a.importer = nil
}

// handleImportDialogKey routes keys to the import dialog and dispatches
// the appropriate submit handler based on which step the dialog is on.
func (a *App) handleImportDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if a.importer == nil {
		return a, nil
	}
	return a.importDialogAction(a.importer.dlg.HandleKey(msg))
}

// importDialogAction dispatches a DialogAction for the import dialog, from either input path.
func (a *App) importDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.closeImportDialog()
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitImportDialog()
	}
	return a, nil
}

// submitImportDialog dispatches the right submit handler based on the
// current step of the import workflow.
func (a *App) submitImportDialog() (tea.Model, tea.Cmd) {
	if a.importer.state == nil {
		return a, nil
	}
	switch a.importer.state.step {
	case importStepOptions:
		return a.submitImportOptions()
	case importStepSourcePicker:
		return a.submitImportSourcePicker()
	case importStepConfirm:
		return a.submitImportConfirm()
	}
	return a, nil
}

// submitImportSourcePicker captures the user's source-account choice and
// re-runs the preview filtered to that account.
func (a *App) submitImportSourcePicker() (tea.Model, tea.Cmd) {
	state := a.importer.state
	d := a.importer.dlg
	if state == nil || d == nil {
		return a, nil
	}
	fields := d.Fields()
	if len(fields) < 3 {
		return a, nil
	}
	picked := fields[2].SelectedOption()
	if picked == "" {
		d.SetErrorMsg("Pick a source account.")
		return a, nil
	}
	state.sourceAccount = picked
	return a, a.runImportPreview(state)
}

// submitImportOptions validates the options dialog inputs, captures them
// onto importDialogState, and kicks off the preview command.
func (a *App) submitImportOptions() (tea.Model, tea.Cmd) {
	d := a.importer.dlg
	state := a.importer.state
	d.ClearErrors()

	fields := d.Fields()
	if len(fields) < 4 {
		return a, nil
	}

	path := strings.TrimSpace(fields[0].Value)
	if path == "" {
		fields[0].Error = "File path is required"
		return a, nil
	}

	if len(state.accountIDs) == 0 {
		d.SetErrorMsg("No accounts available; create one before importing.")
		return a, nil
	}
	accountIdx := fields[1].SelectedIndex
	if accountIdx < 0 || accountIdx >= len(state.accountIDs) {
		d.SetErrorMsg("Select a target account.")
		return a, nil
	}

	state.filePath = path
	state.accountID = state.accountIDs[accountIdx]
	state.accountName = fields[1].SelectedOption()
	state.format = importFormatFromIndex(fields[2].SelectedIndex)
	state.duplicateHandling = importDuplicateFromIndex(fields[3].SelectedIndex)

	// Don't dismiss the dialog here — leave it on screen until the preview
	// returns so the user has visual feedback that something is happening.
	return a, a.runImportPreview(state)
}

// runImportPreview reads, parses, and matches the import file.
//
// If the file contains rows for more than one source account *and* the
// user hasn't already picked one (state.sourceAccount empty), it returns
// importNeedsSourceMsg so the picker dialog opens. Once the user has
// chosen, this re-runs filtered to that source.
func (a *App) runImportPreview(state *importDialogState) tea.Cmd {
	return func() tea.Msg {
		format := state.format
		if format == "" {
			detected, err := imexport.DetectFormat(state.filePath)
			if err != nil {
				return errMsg{err: fmt.Errorf("cannot detect format: %w (use the Format dropdown to choose)", err)}
			}
			format = detected
			state.format = detected
		}

		f, err := os.Open(state.filePath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to open file: %w", err)}
		}
		defer f.Close()

		svc := app.NewServices(a.db)
		importSvc := imexport.NewImportService(
			imexport.NewServiceCategoryResolver(svc.Category),
			imexport.NewServicePayeeResolver(svc.Payee),
			imexport.NewRepoTransactionStore(svc.TransactionRepo, svc.PayeeRepo),
			imexport.NewServiceTransactionCreator(svc.Transaction),
		)

		parseResult, err := importSvc.Parse(f, format)
		if err != nil {
			return errMsg{err: fmt.Errorf("import parse failed: %w", err)}
		}

		sources := imexport.DistinctAccounts(parseResult)
		if len(sources) > 1 && state.sourceAccount == "" {
			return importNeedsSourceMsg{state: state, sources: sources}
		}
		if state.sourceAccount != "" {
			parseResult = imexport.FilterByAccount(parseResult, state.sourceAccount)
		}

		result, err := importSvc.PreviewRecords(parseResult, state.accountID, imexport.ImportOptions{
			Format:            format,
			DuplicateHandling: state.duplicateHandling,
		})
		if err != nil {
			return errMsg{err: fmt.Errorf("import preview failed: %w", err)}
		}
		return importPreviewedMsg{state: state, result: result}
	}
}

// submitImportConfirm executes the import after the user has reviewed the
// preview summary.
func (a *App) submitImportConfirm() (tea.Model, tea.Cmd) {
	state := a.importer.state
	if state == nil || state.preview == nil {
		a.closeImportDialog()
		return a, nil
	}
	// If nothing was parsed there's nothing to do — just close.
	if len(state.preview.Rows) == 0 {
		a.closeImportDialog()
		return a, nil
	}
	a.closeImportDialog()
	return a, a.runImportExecute(state)
}

// runImportExecute applies the import to the database.
func (a *App) runImportExecute(state *importDialogState) tea.Cmd {
	return func() tea.Msg {
		svc := app.NewServices(a.db)
		importSvc := imexport.NewImportService(
			imexport.NewServiceCategoryResolver(svc.Category),
			imexport.NewServicePayeeResolver(svc.Payee),
			imexport.NewRepoTransactionStore(svc.TransactionRepo, svc.PayeeRepo),
			imexport.NewServiceTransactionCreator(svc.Transaction),
		)
		if err := importSvc.Execute(state.preview, state.accountID); err != nil {
			return errMsg{err: fmt.Errorf("import execution failed: %w", err)}
		}
		return importCompletedMsg{
			created: state.preview.Created,
			updated: state.preview.Updated,
			skipped: state.preview.Skipped,
			errors:  state.preview.Errors,
		}
	}
}
