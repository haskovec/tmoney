// The price import dialog (bulk CSV import). It is a modal surface registered
// in modals(), not view code: the Prices view opens it, and the modal registry
// routes its keys and paints it.

package tui

import (
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/imexport"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/tui/dialog"
)

// buildImportPriceDialog builds the dialog for importing prices from CSV.
func buildImportPriceDialog() *dialog.Dialog {
	d := dialog.NewDialog("Import Prices")

	f := d.AddTextField("CSV File", "", "Path to CSV file", 0)
	f.Required = true

	d.AddCheckboxField("Overwrite existing", false)

	d.SetButtons([]dialog.DialogButton{
		{Label: "Import", Primary: true},
		{Label: "Cancel"},
	})

	return d
}

// handlePriceImportDialogKey handles key presses in the import dialog.
func (a *App) handlePriceImportDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.priceImportDialogAction(a.priceImportDialog.HandleKey(msg))
}

// priceImportDialogAction dispatches a DialogAction for the price import dialog, from either input path.
func (a *App) priceImportDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.priceImportDialog.SetVisible(false)
		a.priceImportDialog = nil
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitImportPriceDialog()
	}
	return a, nil
}

// submitImportPriceDialog processes the import dialog submission.
func (a *App) submitImportPriceDialog() (tea.Model, tea.Cmd) {
	fields := a.priceImportDialog.Fields()

	filePath := strings.TrimSpace(fields[0].Value)
	if filePath == "" {
		a.priceImportDialog.SetErrorMsg("CSV file path is required.")
		return a, nil
	}

	overwrite := fields[1].Checked

	a.priceImportDialog.SetVisible(false)
	a.priceImportDialog = nil

	return a, a.importPrices(filePath, overwrite)
}

// importPrices imports prices from a CSV file.
func (a *App) importPrices(filePath string, overwrite bool) tea.Cmd {
	return func() tea.Msg {
		if a.services.Price == nil || a.services.Security == nil {
			return errMsg{err: fmt.Errorf("services not available")}
		}

		f, err := os.Open(filePath)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to open file: %w", err)}
		}
		defer f.Close()

		result, err := imexport.ParsePriceCSV(f)
		if err != nil {
			return errMsg{err: fmt.Errorf("failed to parse CSV: %w", err)}
		}

		if result.HasErrors() {
			var msgs []string
			for _, e := range result.Errors {
				msgs = append(msgs, fmt.Sprintf("line %d: %s", e.Line, e.Message))
			}
			return errMsg{err: fmt.Errorf("CSV errors:\n%s", strings.Join(msgs, "\n"))}
		}

		// Resolve tickers to security IDs
		var prices []*price.Price
		for _, rec := range result.Records {
			sec, lookupErr := a.services.Security.GetByTicker(rec.Ticker, "USD")
			if lookupErr != nil {
				return errMsg{err: fmt.Errorf("unknown ticker %q (line %d): %w", rec.Ticker, rec.SourceLine, lookupErr)}
			}
			prices = append(prices, price.NewPrice(sec.ID, rec.Date, rec.Price, price.SourceImport))
		}

		importResult, err := a.services.Price.BulkImport(prices, overwrite)
		if err != nil {
			return errMsg{err: fmt.Errorf("import failed: %w", err)}
		}

		return priceImportedMsg{
			total:    importResult.Total,
			imported: importResult.Imported,
			skipped:  importResult.Skipped,
		}
	}
}
