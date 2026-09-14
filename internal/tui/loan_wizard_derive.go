package tui

import (
	"strconv"
	"strings"

	"github.com/haskovec/tmoney/internal/loan"
	"github.com/haskovec/tmoney/internal/tui/dialog"
	"github.com/haskovec/tmoney/internal/types"
)

// Derived state: which fields are visible, and the payment estimate the wizard
// recomputes from the principal, rate and term fields as the user types.

// refreshDerived recomputes conditional field visibility and the payment
// prefill. Called after every key/mouse edit.
func (s *loanSurface) refreshDerived() {
	if s.dlg == nil || s.state == nil {
		return
	}
	updateLoanWizardVisibility(s.dlg)
	s.updatePaymentPrefill()
}

// updateLoanWizardVisibility toggles the Hidden state of conditional fields:
// the interest category is hidden at 0% APR; the asset fields are hidden unless
// "Track an asset" is checked; escrow rows reveal progressively.
func updateLoanWizardVisibility(d *dialog.Dialog) {
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return
	}

	// Interest category: shown only when APR > 0.
	showInterest := loanAPRPositive(fields[loanFieldAPR].Value)
	fields[loanFieldInterestCategory].Hidden = !showInterest
	if !showInterest {
		fields[loanFieldInterestCategory].Error = ""
	}

	// Escrow rows: row 0 always visible; row k visible once row k-1 has a
	// category, and a row that itself has a category stays visible.
	for k := range loanMaxEscrowLines {
		catIdx := loanEscrowCatIndex(k)
		amtIdx := loanEscrowAmtIndex(k)
		visible := k == 0
		if k > 0 && fields[loanEscrowCatIndex(k-1)].SelectedIndex > 0 {
			visible = true
		}
		if fields[catIdx].SelectedIndex > 0 {
			visible = true
		}
		fields[catIdx].Hidden = !visible
		fields[amtIdx].Hidden = !visible
		if !visible {
			fields[amtIdx].Error = ""
		}
	}

	// Asset name/value: shown only when tracking an asset.
	trackAsset := fields[loanFieldTrackAsset].Checked
	fields[loanFieldAssetName].Hidden = !trackAsset
	fields[loanFieldAssetValue].Hidden = !trackAsset
	if !trackAsset {
		fields[loanFieldAssetName].Error = ""
		fields[loanFieldAssetValue].Error = ""
	}
}

// updatePaymentPrefill refreshes the Payment field from the amortization
// formula while the field is untouched (empty, or still equal to the last
// auto-computed value). Once the user types a different value the field is
// considered edited and the prefill stops overwriting it.
func (s *loanSurface) updatePaymentPrefill() {
	d, st := s.dlg, s.state
	fields := d.Fields()
	if len(fields) < loanFieldFieldsCount {
		return
	}
	payment := fields[loanFieldPayment]
	if payment.Value != "" && payment.Value != st.lastComputedPayment {
		return // user-edited; leave it alone
	}
	prefill, ok := computeLoanPaymentPrefill(fields)
	if !ok {
		return
	}
	prefillField(payment, prefill)
	st.lastComputedPayment = prefill
}

// computeLoanPaymentPrefill computes the standard amortization P&I payment from
// the prefill-only fields (original principal, APR, term). Returns ok=false
// when the inputs are incomplete or invalid so the caller leaves the field
// untouched.
func computeLoanPaymentPrefill(fields []*dialog.Field) (string, bool) {
	principalStr := strings.TrimSpace(fields[loanFieldOrigPrincipal].Value)
	termStr := strings.TrimSpace(fields[loanFieldTermMonths].Value)
	if principalStr == "" || termStr == "" {
		return "", false
	}
	principal, err := parseAmountInput(principalStr)
	if err != nil || !principal.IsPositive() {
		return "", false
	}
	term, err := strconv.Atoi(termStr)
	if err != nil || term <= 0 {
		return "", false
	}
	apr := types.ZeroMoney
	if aprStr := strings.TrimSpace(fields[loanFieldAPR].Value); aprStr != "" {
		apr, err = parseAmountInput(aprStr)
		if err != nil {
			return "", false
		}
	}
	pay, err := loan.Payment(principal, apr, term)
	if err != nil {
		return "", false
	}
	return pay.String(), true
}

// loanAPRPositive reports whether the APR field value parses to a positive rate.
func loanAPRPositive(value string) bool {
	s := strings.TrimSpace(value)
	if s == "" {
		return false
	}
	apr, err := parseAmountInput(s)
	if err != nil {
		return false
	}
	return apr.IsPositive()
}
