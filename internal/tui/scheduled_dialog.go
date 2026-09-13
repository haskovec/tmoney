package tui

import (
	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// pendingSplitScheduled carries scalar field values from the scheduled
// dialog into the multi-line split editor. The SplitDialog produces
// transaction.Split rows; submitScheduledSplitDialog translates them
// into scheduled.Split children at save time.
type pendingSplitScheduled struct {
	mode        scheduledDialogMode
	existing    *scheduled.Transaction
	accountID   types.ID
	payeeName   string
	amount      types.Money
	memo        string
	frequency   scheduled.Frequency
	interval    int
	startDate   types.Date
	endDate     types.NullableDate
	occurrences types.NullableInt
	autoPost    bool
	leadDays    int
}

// transactionSplitsFromScheduled converts the children of an existing
// scheduled.Transaction into transaction.Split rows so the SplitDialog
// can seed itself from a previously-saved multi-line template. The
// returned rows carry only the fields the split editor uses
// (category/transfer target, amount, memo) plus paycheck_section, which the
// editor does not expose but must not destroy — see scheduledSplitsFromTransaction.
func transactionSplitsFromScheduled(st *scheduled.Transaction) []*transaction.Split {
	if st == nil || len(st.Splits) == 0 {
		return nil
	}
	rows := make([]*transaction.Split, 0, len(st.Splits))
	for _, sp := range st.Splits {
		t := &transaction.Split{
			BaseModel: types.NewBaseModel(),
			Amount:    sp.Amount,
		}
		if sp.CategoryID.Valid {
			t.CategoryID = sp.CategoryID.ID
		}
		if sp.TransferAccountID.Valid {
			t.TransferAccountID = sp.TransferAccountID
		}
		if sp.Memo.Valid {
			t.SetMemo(sp.Memo.String)
		}
		t.PaycheckSection = sp.PaycheckSection
		rows = append(rows, t)
	}
	return rows
}

// scheduledSplitsFromTransaction converts the transaction.Split rows the split
// editor produces back into scheduled.Split children for a multi-line template.
// It is the inverse of transactionSplitsFromScheduled. A transfer line carries
// its category through (carry-through) so an already-categorized template
// transfer line survives an Edit Series round-trip; the split editor offers no
// picker for it (v1 non-goal). paycheck_section rides through the same way, so
// editing a line here no longer demotes a paycheck schedule to a generic one.
// LoanSection is deliberately left unset: minting fresh children is what keeps
// the "paycheck_section IS NULL OR loan_section IS NULL" CHECK unreachable.
func scheduledSplitsFromTransaction(splits []*transaction.Split) scheduled.SplitCollection {
	children := scheduled.SplitCollection{}
	for _, ts := range splits {
		child := &scheduled.Split{
			BaseModel: types.NewBaseModel(),
			Amount:    ts.Amount,
		}
		switch {
		case ts.TransferAccountID.Valid:
			child.TransferAccountID = ts.TransferAccountID
			if !ts.CategoryID.IsNil() {
				child.CategoryID = types.NullableID{ID: ts.CategoryID, Valid: true}
			}
		default:
			child.CategoryID = types.NullableID{ID: ts.CategoryID, Valid: true}
		}
		if ts.Memo.Valid {
			child.SetMemo(ts.Memo.String)
		}
		child.PaycheckSection = ts.PaycheckSection
		children = append(children, child)
	}
	return children
}

// scheduledDialogMode indicates whether the dialog is creating or editing.
type scheduledDialogMode int

const (
	scheduledDialogModeNew scheduledDialogMode = iota
	scheduledDialogModeEdit
)

// scheduledDialogData holds the loaded data needed for the scheduled dialog.
type scheduledDialogData struct {
	mode       scheduledDialogMode
	scheduled  *scheduled.Transaction // non-nil when editing
	accounts   []*account.Account
	payees     []*payee.Payee
	payeeMap   map[string]*payee.Payee // lowercase name -> payee
	isTransfer bool                    // true for the single-line transfer dialog
}

// scheduledDialogDataMsg is sent when scheduled dialog data has been loaded.
type scheduledDialogDataMsg struct {
	data *scheduledDialogData
}

// scheduledDialogSavedMsg is sent when a scheduled transaction has been saved.
type scheduledDialogSavedMsg struct{}

// paycheckDemotionWarning is shown before a save that would drop a
// paycheck-shaped schedule's paycheck_section tags. Nothing behavioural breaks
// (no posting path reads the column), but the hand-entered section layout is
// lost and only re-entering every line in the wizard brings it back. An edit
// that preserves every tag saves silently — the generic editor is meant to be
// usable on a paycheck.
const paycheckDemotionWarning = "This drops the paycheck section tags — the schedule will no longer open in the paycheck wizard until you save it there again. Continue?"

// Scheduled dialog field indices.
const (
	schedFieldAccount    = 0
	schedFieldPayee      = 1
	schedFieldCategory   = 2
	schedFieldAmount     = 3
	schedFieldMemo       = 4
	schedFieldFrequency  = 5
	schedFieldInterval   = 6
	schedFieldStartDate  = 7
	schedFieldDuration   = 8
	schedFieldEndDate    = 9
	schedFieldOccurrence = 10
	schedFieldAutoPost   = 11
	schedFieldLeadDays   = 12
	schedFieldSplit      = 13
)

// accountIsAssetByID reports whether the account with the given ID in
// accounts is of type asset specifically (Type == TypeAsset, not the
// broader IsAssetType). Used to decide whether the Value Adjustment
// category is offered in a scheduled-transaction category picker.
func accountIsAssetByID(accounts []*account.Account, id types.ID) bool {
	if id.IsNil() {
		return false
	}
	for _, acc := range accounts {
		if acc != nil && acc.ID == id {
			return acc.Type == account.TypeAsset
		}
	}
	return false
}

// buildFrequencyOptions returns display names for all frequencies.
func buildFrequencyOptions() []string {
	freqs := scheduled.AllFrequencies()
	options := make([]string, len(freqs))
	for i, f := range freqs {
		options[i] = f.DisplayName()
	}
	return options
}

// frequencyFromIndex returns the Frequency for a given select index.
func frequencyFromIndex(index int) scheduled.Frequency {
	freqs := scheduled.AllFrequencies()
	if index < 0 || index >= len(freqs) {
		return scheduled.FrequencyMonthly
	}
	return freqs[index]
}

// frequencyToIndex returns the select index for a given Frequency.
// Unknown frequencies fall back to the index of FrequencyMonthly in
// AllFrequencies so the dialog opens on a sensible default.
func frequencyToIndex(f scheduled.Frequency) int {
	freqs := scheduled.AllFrequencies()
	for i, freq := range freqs {
		if freq == f {
			return i
		}
	}
	for i, freq := range freqs {
		if freq == scheduled.FrequencyMonthly {
			return i
		}
	}
	return 0
}

// durationIndex constants for the radio field.
const (
	durationIndefinite  = 0
	durationUntilDate   = 1
	durationOccurrences = 2
)

// leadDays constants for the radio field.
const (
	leadDaysOnTheDay = 0
	leadDays3Days    = 1
	leadDays1Week    = 2
)

// leadDaysToIndex converts PostLeadDays value to radio index.
func leadDaysToIndex(days int) int {
	switch days {
	case 3:
		return leadDays3Days
	case 7:
		return leadDays1Week
	default:
		return leadDaysOnTheDay
	}
}

// leadDaysFromIndex converts a radio index to PostLeadDays value.
func leadDaysFromIndex(index int) int {
	switch index {
	case leadDays3Days:
		return 3
	case leadDays1Week:
		return 7
	default:
		return 0
	}
}

// schedSurface is the Scheduled Transaction dialog together with the form state that
// belongs to it. Its zero value is closed; closeScheduledDialog resets it to that.
type schedSurface struct {
	modalSurface
	data            *scheduledDialogData
	accountIDs      []types.ID
	categoryIDs     []types.ID
	categoryOptions []string
}

func (s *schedSurface) IsVisible() bool { return s != nil && s.dlg.IsVisible() }

// applyData builds whichever of the three scheduled forms data describes:
// the transfer form, the regular edit form, or the regular new form.
//
// categories is the full category list. The transfer form offers every
// non-system category; the regular form additionally offers Value Adjustment
// when the initially selected account is an asset.
//
// It reports whether the form built is the regular edit form, the only one
// that can carry the "Edit as loan ->" button.
func (s *schedSurface) applyData(data *scheduledDialogData, categories []*category.Category) bool {
	s.data = data

	// Single-line transfer schedules use a distinct dialog whose From/To
	// pickers exclude investment accounts (regular<->regular only). The
	// optional Category combo excludes every system category (Transfer,
	// Value Adjustment) — a transfer may be labeled with any non-system
	// category.
	if data.isTransfer {
		accountOptions, accountIDs := buildTransferAccountOptions(data.accounts)
		s.accountIDs = accountIDs
		categoryOptions, categoryIDs := buildCategoryOptions(categories)
		s.categoryIDs = categoryIDs
		s.categoryOptions = categoryOptions

		if data.mode == scheduledDialogModeEdit && data.scheduled != nil {
			s.dlg = buildEditScheduledTransferDialog(data.scheduled, accountOptions, categoryOptions, accountIDs, categoryIDs)
		} else {
			s.dlg = buildNewScheduledTransferDialog(accountOptions, categoryOptions)
		}
		return false
	}

	accountOptions, accountIDs := buildAccountOptions(data.accounts)
	s.accountIDs = accountIDs

	// Surface the Value Adjustment category when the initially
	// selected account is an asset account (edit: the schedule's
	// account; new: the first account, which the picker defaults
	// to). The picker tracks later account changes via
	// refreshSchedCategoryOptionsForAccount.
	initialAcctID := types.NilID
	if data.mode == scheduledDialogModeEdit && data.scheduled != nil {
		initialAcctID = data.scheduled.AccountID
	} else if len(accountIDs) > 0 {
		initialAcctID = accountIDs[0]
	}
	includeVA := accountIsAssetByID(data.accounts, initialAcctID)
	categoryOptions, categoryIDs := buildCategoryOptionsFor(categories, includeVA)
	s.categoryIDs = categoryIDs
	s.categoryOptions = categoryOptions

	if data.mode != scheduledDialogModeEdit || data.scheduled == nil {
		s.dlg = buildNewScheduledDialog(accountOptions, categoryOptions)
		return false
	}

	// Build payee name map for edit dialog
	payeeNames := make(map[types.ID]string)
	for _, p := range data.payees {
		payeeNames[p.ID] = p.Name
	}
	s.dlg = buildEditScheduledDialog(data.scheduled, accountOptions, accountIDs, categoryOptions, categoryIDs, payeeNames)
	return true
}
