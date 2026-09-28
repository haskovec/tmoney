// The Price add/edit dialog. It is a modal surface registered in modals(), not
// view code: the Prices view opens it, and the modal registry routes its keys
// and paints it.

package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/tui/widget"
	"github.com/haskovec/tmoney/internal/types"
)

// priceSurface is the Price add/edit dialog's state. mode selects add vs edit;
// editID is meaningful only when mode is priceDialogModeEdit.
type priceSurface struct {
	modalSurface
	mode   priceDialogMode
	editID types.ID
}

// IsVisible must be declared here rather than promoted from modalSurface — see
// the note on modalSurface.
func (s *priceSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

type priceDialogMode int

const (
	priceDialogModeAdd priceDialogMode = iota
	priceDialogModeEdit
)

// buildAddPriceDialog builds the dialog for adding a new price.
func buildAddPriceDialog(sec *security.Security) *dialog.Dialog {
	d := dialog.NewDialog(fmt.Sprintf("Add Price — %s", securityLabel(sec)))

	today := time.Now().Format("2006-01-02")
	f := d.AddDateFieldISO("Date", today)
	f.Required = true

	f = d.AddTextField("Price", "", "e.g. 185.50", 15)
	f.Required = true

	d.SetButtons(priceDialogButtons(sec))

	return d
}

// priceDialogButtons returns the price dialog's buttons. The provider Lookup
// button is offered only when the security has a ticker — a tickerless
// security (e.g. a collective trust) cannot be fetched from a price provider,
// so its price comes from manual entry or transactions.
func priceDialogButtons(sec *security.Security) []dialog.DialogButton {
	buttons := []dialog.DialogButton{{Label: "Save", Primary: true}}
	if sec.Ticker != "" {
		buttons = append(buttons, dialog.DialogButton{Label: "Lookup", Action: dialog.DialogActionAlternate})
	}
	return append(buttons, dialog.DialogButton{Label: "Cancel"})
}

// buildEditPriceDialog builds the dialog for editing an existing price.
func buildEditPriceDialog(sec *security.Security, p *price.Price) *dialog.Dialog {
	d := dialog.NewDialog(fmt.Sprintf("Edit Price — %s", securityLabel(sec)))

	dateStr := p.Date.Time().Format("2006-01-02")
	f := d.AddDateFieldISO("Date", dateStr)
	f.Required = true

	priceStr := fmt.Sprintf("%.2f", p.Price.Float64())
	f = d.AddTextField("Price", priceStr, "e.g. 185.50", 15)
	f.Required = true

	d.SetButtons(priceDialogButtons(sec))

	return d
}

// handlePriceDialogKey handles key presses in the price add/edit dialog.
func (a *App) handlePriceDialogKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return a.priceDialogAction(a.price.dlg.HandleKey(msg))
}

// priceDialogAction dispatches a DialogAction for the price dialog, from either input path.
func (a *App) priceDialogAction(action dialog.DialogAction) (tea.Model, tea.Cmd) {
	switch action {
	case dialog.DialogActionCancel:
		a.price = priceSurface{}
		return a, nil
	case dialog.DialogActionSubmit:
		return a.submitPriceDialog()
	case dialog.DialogActionAlternate:
		return a.startPriceLookup()
	}
	return a, nil
}

// priceLookupResultMsg carries the outcome of a Lookup-button provider fetch.
type priceLookupResultMsg struct {
	price    types.Money
	date     types.Date
	currency string
	err      error
}

// startPriceLookup reads the dialog's current date + selected security and
// kicks off an async provider fetch to fill the Price field.
func (a *App) startPriceLookup() (tea.Model, tea.Cmd) {
	if a.price.dlg == nil || a.priceView.selectedSecurity == nil {
		return a, nil
	}
	fields := a.price.dlg.Fields()
	if len(fields) < 2 {
		return a, nil
	}
	a.price.dlg.SetErrorMsg("")
	return a, a.lookupPriceCmd(a.priceView.selectedSecurity.Ticker, strings.TrimSpace(fields[0].Value))
}

// lookupPriceCmd fetches the provider's close on/before the given date for the
// ticker and delivers it as a priceLookupResultMsg.
func (a *App) lookupPriceCmd(ticker, dateStr string) tea.Cmd {
	return func() tea.Msg {
		date, err := types.ParseDate(dateStr)
		if err != nil {
			return priceLookupResultMsg{err: fmt.Errorf("enter a valid date first (YYYY-MM-DD)")}
		}
		if a.services.Price == nil {
			return priceLookupResultMsg{err: fmt.Errorf("price service not available")}
		}
		provider, err := a.services.Price.ProviderRegistry().Get(defaultRefreshProviderName)
		if err != nil {
			return priceLookupResultMsg{err: err}
		}
		quote, err := provider.FetchQuoteOn(ticker, date)
		if err != nil {
			return priceLookupResultMsg{err: err}
		}
		return priceLookupResultMsg{price: quote.Price, date: quote.Date, currency: quote.Currency}
	}
}

// handlePriceLookupResult fills the Price (and resolved Date) fields from a
// completed lookup, or surfaces the error on the still-open dialog.
func (a *App) handlePriceLookupResult(msg priceLookupResultMsg) (tea.Model, tea.Cmd) {
	if a.price.dlg == nil {
		return a, nil
	}
	if msg.err != nil {
		a.price.dlg.SetErrorMsg("Lookup failed: " + msg.err.Error())
		return a, nil
	}
	fields := a.price.dlg.Fields()
	if len(fields) >= 2 {
		// fields[0] is a masked FieldDate: it overwrites digits from the
		// first one, so its cursor must stay at 0 — no prefillField here.
		fields[0].Value = msg.date.Time().Format("2006-01-02")
		prefillField(fields[1], fmt.Sprintf("%.2f", msg.price.Float64()))
	}
	a.statusbar.AddNotification(
		fmt.Sprintf("Fetched %.2f %s on %s", msg.price.Float64(), msg.currency, msg.date.String()),
		widget.NotificationInfo,
	)
	return a, nil
}

// submitPriceDialog processes the price dialog submission.
func (a *App) submitPriceDialog() (tea.Model, tea.Cmd) {
	fields := a.price.dlg.Fields()

	dateStr := strings.TrimSpace(fields[0].Value)
	priceStr := strings.TrimSpace(fields[1].Value)

	if dateStr == "" {
		a.price.dlg.SetErrorMsg("Date is required.")
		return a, nil
	}
	if priceStr == "" {
		a.price.dlg.SetErrorMsg("Price is required.")
		return a, nil
	}

	date, err := types.ParseDate(dateStr)
	if err != nil {
		a.price.dlg.SetErrorMsg("Invalid date format. Use YYYY-MM-DD.")
		return a, nil
	}

	amount, err := types.NewMoney(priceStr)
	if err != nil {
		a.price.dlg.SetErrorMsg("Invalid price value.")
		return a, nil
	}

	if amount.IsZero() || !amount.IsPositive() {
		a.price.dlg.SetErrorMsg("Price must be positive.")
		return a, nil
	}

	secID := a.priceView.selectedSecurity.ID
	mode := a.price.mode
	editID := a.price.editID

	a.price = priceSurface{}

	if mode == priceDialogModeAdd {
		return a, a.createPrice(secID, date, amount)
	}
	return a, a.updatePrice(editID, secID, date, amount)
}

// createPrice creates a new price via the service.
func (a *App) createPrice(securityID types.ID, date types.Date, amount types.Money) tea.Cmd {
	return func() tea.Msg {
		if a.services.Price == nil {
			return errMsg{err: fmt.Errorf("price service not available")}
		}

		p := price.NewPrice(securityID, date, amount, price.SourceManual)
		if err := a.services.Price.AddPrice(p); err != nil {
			return errMsg{err: err}
		}
		return priceAddedMsg{}
	}
}

// updatePrice updates an existing price via the service.
func (a *App) updatePrice(id, securityID types.ID, date types.Date, amount types.Money) tea.Cmd {
	return func() tea.Msg {
		if a.services.Price == nil {
			return errMsg{err: fmt.Errorf("price service not available")}
		}

		p := price.NewPrice(securityID, date, amount, price.SourceManual)
		p.ID = id
		if err := a.services.Price.UpdatePrice(p); err != nil {
			return errMsg{err: err}
		}
		return priceUpdatedMsg{}
	}
}
