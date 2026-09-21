package transfer

import (
	"fmt"
	"sort"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// This file owns the one operation that moves an account between the two
// ledgers: a type change from an investment type to a regular one, or back.
// See specs/design-hsa-split.md §5.
//
// It lives here and not in internal/account because it must read
// investment_transactions and write transactions, and both ledger packages
// import internal/account. This package already sits above both ledgers and
// owns the "which table does a leg live in" rule (LedgerFor).
//
// account.Service.Update is the guard: it refuses a cross-ledger type change
// while the departing ledger holds rows (account.LedgerChangeError). This is
// the door that empties the ledger first.

// cashKinds are the investment row types that carry no security and can be
// expressed as a plain register row. Every other type refuses the move.
var cashKinds = map[investment.TransactionType]bool{
	investment.TransactionTypeDeposit:      true,
	investment.TransactionTypeWithdrawal:   true,
	investment.TransactionTypeInterest:     true,
	investment.TransactionTypeFee:          true,
	investment.TransactionTypeTransferCash: true,
}

// LedgerMovePlan describes what ChangeAccountType will do. Callers show it to
// the user before anything is written.
type LedgerMovePlan struct {
	AccountID   types.ID
	AccountName string
	From        account.Type
	To          account.Type
	FromLedger  Ledger
	ToLedger    Ledger

	// RowsByType counts the investment rows that will become register rows.
	// It is empty when no rows move.
	RowsByType map[investment.TransactionType]int
	// Total is the sum of RowsByType.
	Total int
}

// CrossesLedger reports whether the change moves the account to the other
// table. A same-ledger change (checking → savings, investment →
// hsa_investment) does not.
func (p *LedgerMovePlan) CrossesLedger() bool { return p.FromLedger != p.ToLedger }

// MovesRows reports whether any rows will be rewritten.
func (p *LedgerMovePlan) MovesRows() bool { return p.Total > 0 }

// SortedTypes returns the moved row types in a stable order for display.
func (p *LedgerMovePlan) SortedTypes() []investment.TransactionType {
	out := make([]investment.TransactionType, 0, len(p.RowsByType))
	for t := range p.RowsByType {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// LedgerMoveRefusedError is returned when a cross-ledger type change cannot
// be performed. Reason is a short phrase suitable for a user-facing message.
type LedgerMoveRefusedError struct {
	AccountID   types.ID
	AccountName string
	From        account.Type
	To          account.Type
	Reason      string
	Rows        int
}

func (e *LedgerMoveRefusedError) Error() string {
	return fmt.Sprintf("cannot change %q from %s to %s: %s (%d row(s))",
		e.AccountName, e.From, e.To, e.Reason, e.Rows)
}

// StalePlanError is returned when the account changed between
// PlanAccountTypeChange and ChangeAccountType.
type StalePlanError struct {
	AccountID types.ID
}

func (e *StalePlanError) Error() string {
	return fmt.Sprintf("the account %s changed since the plan was made; plan again", e.AccountID.String())
}

// PlanAccountTypeChange validates a type change and returns what it would do.
//
// Investment → regular is allowed when every row is a cash kind and the
// account has no positions or lots; any security row refuses the whole
// change. Regular → investment is allowed only when the account has no
// transactions and no scheduled transactions, because register rows carry
// payees, categories and splits that the investment ledger cannot hold.
func (s *Service) PlanAccountTypeChange(acctID types.ID, to account.Type) (*LedgerMovePlan, error) {
	if !to.IsValid() {
		return nil, fmt.Errorf("invalid account type: %q", to)
	}
	acct, err := s.accountRepo.GetByID(acctID)
	if err != nil {
		return nil, fmt.Errorf("failed to load account: %w", err)
	}
	plan := &LedgerMovePlan{
		AccountID:   acct.ID,
		AccountName: acct.Name,
		From:        acct.Type,
		To:          to,
		FromLedger:  LedgerFor(acct.Type),
		ToLedger:    LedgerFor(to),
		RowsByType:  map[investment.TransactionType]int{},
	}
	if !plan.CrossesLedger() {
		return plan, nil
	}

	refuse := func(reason string, rows int) error {
		return &LedgerMoveRefusedError{
			AccountID: acct.ID, AccountName: acct.Name,
			From: acct.Type, To: to, Reason: reason, Rows: rows,
		}
	}

	if plan.FromLedger == LedgerRegular {
		rows, err := s.accountRepo.CountLedgerRows(acct.ID, false)
		if err != nil {
			return nil, err
		}
		if rows > 0 {
			return nil, refuse("register rows and scheduled transactions cannot be moved into the investment ledger; delete or move them first", rows)
		}
		return plan, nil
	}

	rows, err := s.invRepo.ListByAccount(acct.ID, investment.TransactionFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list investment rows: %w", err)
	}
	security := 0
	for _, r := range rows {
		if !cashKinds[r.Type] {
			security++
			continue
		}
		plan.RowsByType[r.Type]++
		plan.Total++
	}
	if security > 0 {
		return nil, refuse("the account holds security rows (buy, sell, dividend, shares); only a cash-only account can leave the investment ledger", security)
	}

	var holdings int
	if err := s.q().QueryRow(`
		SELECT (SELECT COUNT(*) FROM investment_positions WHERE CAST(account_id AS VARCHAR) = ?)
		     + (SELECT COUNT(*) FROM investment_lots WHERE CAST(account_id AS VARCHAR) = ?)`,
		acct.ID.String(), acct.ID.String()).Scan(&holdings); err != nil {
		return nil, fmt.Errorf("failed to count positions and lots: %w", err)
	}
	if holdings > 0 {
		return nil, refuse("the account has positions or lots but no security rows; run `investment rebuild-positions` first", holdings)
	}
	return plan, nil
}

// ChangeAccountType applies a plan in one database transaction: it re-plans
// inside the transaction and refuses a stale plan, rewrites each investment
// row as a register row, deletes the source rows, and writes the new type.
// Transfer ids are preserved, so the partner legs need no change.
//
// Moved rows keep their id, date, amount, memo, status and transfer link.
// They get no payee and no category; pending becomes uncleared.
func (s *Service) ChangeAccountType(plan *LedgerMovePlan) error {
	return s.runInTx(func(b *Service) error {
		fresh, err := b.PlanAccountTypeChange(plan.AccountID, plan.To)
		if err != nil {
			return err
		}
		if fresh.From != plan.From || fresh.Total != plan.Total || !sameCounts(fresh.RowsByType, plan.RowsByType) {
			return &StalePlanError{AccountID: plan.AccountID}
		}

		if fresh.MovesRows() {
			rows, err := b.invRepo.ListByAccount(plan.AccountID, investment.TransactionFilter{})
			if err != nil {
				return fmt.Errorf("failed to list investment rows: %w", err)
			}
			for _, r := range rows {
				if err := b.txnRepo.Create(registerRowFrom(r)); err != nil {
					return fmt.Errorf("failed to move row %s: %w", r.ID.String(), err)
				}
				if err := b.invRepo.Delete(r.ID); err != nil {
					return fmt.Errorf("failed to remove moved row %s: %w", r.ID.String(), err)
				}
			}
		}

		acct, err := b.accountRepo.GetByID(plan.AccountID)
		if err != nil {
			return fmt.Errorf("failed to load account: %w", err)
		}
		acct.Type = plan.To
		if !plan.To.IsInvestmentType() {
			acct.TrackLots = false
		}
		if errs := acct.Validate(); errs.HasErrors() {
			return &types.ServiceValidationError{Errors: errs}
		}
		return b.accountRepo.Update(acct)
	})
}

// registerRowFrom maps one cash-kind investment row onto a register row.
func registerRowFrom(r *investment.Transaction) *transaction.Transaction {
	t := transaction.NewTransaction(r.AccountID, r.Date, r.TotalAmount)
	t.ID = r.ID
	t.CreatedAt = r.CreatedAt
	t.Memo = r.Memo
	t.TransferID = r.TransferID
	t.TransferAccountID = r.TransferAccountID
	switch r.Status {
	case investment.TransactionStatusCleared:
		t.Status = transaction.StatusCleared
	case investment.TransactionStatusReconciled:
		t.Status = transaction.StatusReconciled
	default:
		t.Status = transaction.StatusUncleared
	}
	return t
}

func sameCounts(a, b map[investment.TransactionType]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
