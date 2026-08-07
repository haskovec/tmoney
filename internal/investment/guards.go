package investment

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// The guards every investment write runs before it touches a row.
//
// They are package-level functions taking the account repository rather than
// methods, because two types need them: Service, which owns trades and heal, and
// CounterpartService, which owns the investment side of a transfer line. Making
// them free functions is what lets the second type exist without either holding
// the first or copying forty lines of guard.
//
// Each takes the repository the CALLER is bound to, so a guard inside a
// transaction reads that transaction's view.

// loadInvestmentAccount retrieves an account and verifies it is an investment
// account. It does NOT check whether the account is closed: read and maintenance
// paths use it directly and stay ungated.
func loadInvestmentAccount(accountRepo *account.Repository, accountID types.ID) (*account.Account, error) {
	acct, err := accountRepo.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	if !acct.Type.IsInvestmentType() {
		return nil, &account.NotInvestmentError{
			AccountID: accountID.String(),
			Type:      string(acct.Type),
		}
	}

	return acct, nil
}

// requireOpenInvestmentAccount verifies that the given account exists, is an
// investment account, and is not closed. It is the write-path guard — only
// mutation methods call it, which is why the closed check belongs here.
func requireOpenInvestmentAccount(accountRepo *account.Repository, accountID types.ID) error {
	acct, err := loadInvestmentAccount(accountRepo, accountID)
	if err != nil {
		return err
	}
	if acct.IsClosed() {
		return &account.AccountClosedError{ID: accountID.String()}
	}
	return nil
}

// requireOpenAccount returns an AccountClosedError when the account is closed.
// Used by mutation paths that hold only an account ID (e.g. DeleteTransaction);
// it deliberately does NOT funnel through loadInvestmentAccount, so read and
// maintenance paths remain ungated.
func requireOpenAccount(accountRepo *account.Repository, accountID types.ID) error {
	acct, err := accountRepo.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("failed to load account for closed check: %w", err)
	}
	if acct.IsClosed() {
		return &account.AccountClosedError{ID: accountID.String()}
	}
	return nil
}

// validateInvestmentTransaction validates an investment transaction and returns
// any validation errors.
func validateInvestmentTransaction(accountRepo *account.Repository, txn *Transaction) error {
	errors := txn.Validate()
	if errors.HasErrors() {
		return &types.ServiceValidationError{Errors: errors}
	}
	// Reject activity dated before the account opened (catches mistyped years
	// such as "0018" for "2018"). Corporate-action Exchange rows carry the
	// action date and are written via the repository, not this path, so they
	// are never seen here; the type guard is belt-and-suspenders.
	if txn.Type != TransactionTypeExchange {
		acct, err := accountRepo.GetByID(txn.AccountID)
		if err != nil {
			return fmt.Errorf("failed to load account for date validation: %w", err)
		}
		if err := acct.ValidateTransactionDate(txn.Date); err != nil {
			return err
		}
	}
	return nil
}
