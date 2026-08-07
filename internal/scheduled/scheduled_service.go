package scheduled

import (
	"fmt"
	"slices"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// This file holds the Service type: its dependencies and wiring, the join-if-bound
// transaction triad, the closed-account guard every write runs, plain CRUD and the
// list reads, and the small schedule-date predicates.
//
// Posting a schedule into the ledger lives in posting.go and auto_post.go.

// Service provides business logic for scheduled transaction operations.
type Service struct {
	repo        *Repository
	txnRepo     *transaction.Repository
	txnSvc      *transaction.Service
	accountRepo *account.Repository
	// transferPort posts transfer occurrences through the one transfer owner.
	// It is an interface rather than a *transfer.Service because a direct
	// scheduled → transfer import is an "import cycle not allowed in test"
	// (see transfer_port.go). Injected post-construction, like
	// transaction.Service's counterpart port.
	transferPort TransferPort
	db           *db.DB
	tx           db.Queryer // nil outside a transaction
}

// NewService creates a new Service.
//
// txnSvc may be nil for legacy single-line use; posting a multi-line
// scheduled transaction requires a non-nil txnSvc so the multi-line create
// path (including paired transfer counterparts) can be delegated.
//
// accountRepo backs the closed-account freeze gate: a schedule may not be
// created against, or posted into, a closed account.
func NewService(
	repo *Repository,
	txnRepo *transaction.Repository,
	txnSvc *transaction.Service,
	database *db.DB,
	accountRepo *account.Repository,
) *Service {
	return &Service{
		repo:        repo,
		txnRepo:     txnRepo,
		txnSvc:      txnSvc,
		accountRepo: accountRepo,
		db:          database,
	}
}

// SetTransferPort wires the transfer owner used to post transfer occurrences.
// Set after both services exist, for the cycle reason in transfer_port.go.
func (s *Service) SetTransferPort(p TransferPort) { s.transferPort = p }

// InTx returns a copy of the service bound to tx, with every repository field
// and the collaborating transaction service rebound to the same transaction so
// all writes join one atomic unit. Posting a scheduled transaction commits the
// posted rows and the schedule advance together; the transaction service's own
// CreateTransfer/CreateWithSplits/Create join this tx via txnSvc.InTx(tx).
// The original service is unchanged and remains safe for non-transactional use.
//
// Optional collaborators (txnRepo, txnSvc, accountRepo) are nil-guarded to match
// test fixtures that construct a partial service; the production wiring always
// sets them.
func (s *Service) InTx(tx db.Queryer) *Service {
	c := *s
	c.tx = tx
	c.repo = s.repo.WithTx(tx)
	if s.txnRepo != nil {
		c.txnRepo = s.txnRepo.WithTx(tx)
	}
	if s.txnSvc != nil {
		c.txnSvc = s.txnSvc.InTx(tx)
	}
	if s.accountRepo != nil {
		c.accountRepo = s.accountRepo.WithTx(tx)
	}
	return &c
}

// q returns the active Queryer for ad-hoc service-level SQL: the bound
// transaction if any, else the live connection. A bound service must not query
// the pool directly — a pool read would miss the transaction's own
// uncommitted writes, and a pool write would escape its atomicity.
func (s *Service) q() db.Queryer {
	if s.tx != nil {
		return s.tx
	}
	return s.db.Conn()
}

// runInTx begins a new transaction if the service is unbound, or joins the
// already-bound transaction. This is what makes service methods composable
// without savepoints: an outer flow binds once, inner calls join. A bound
// service must never reach db.WithTx (nesting would deadlock the mutex).
func (s *Service) runInTx(fn func(b *Service) error) error {
	if s.tx != nil {
		return fn(s) // already bound — join the caller's tx
	}
	return s.db.WithTx(func(tx db.Queryer) error {
		return fn(s.InTx(tx))
	})
}

// referencedAccountIDs returns every account a schedule touches: its source
// account, its single-line transfer destination, and every transfer-line
// split target.
func referencedAccountIDs(st *Transaction) []types.ID {
	ids := []types.ID{st.AccountID}
	if st.IsTransfer() {
		ids = append(ids, st.TransferAccountID.ID)
	}
	for _, sp := range st.Splits {
		if sp.TransferAccountID.Valid {
			ids = append(ids, sp.TransferAccountID.ID)
		}
	}
	return ids
}

// ensureNoClosedAccounts rejects a schedule that references any closed account
// (source, transfer destination, or transfer-line split target). Nil-tolerant
// for fixtures constructed without an accountRepo; production always wires one.
func (s *Service) ensureNoClosedAccounts(st *Transaction) error {
	if s.accountRepo == nil {
		return nil
	}
	for _, id := range referencedAccountIDs(st) {
		acct, err := s.accountRepo.GetByID(id)
		if err != nil {
			return fmt.Errorf("failed to load account for closed check: %w", err)
		}
		if acct.IsClosed() {
			return &ClosedAccountError{ID: id.String()}
		}
	}
	return nil
}

// Create validates and creates a new scheduled transaction. If st carries
// child Splits, the schedule is multi-line: child rows are persisted and
// validation enforces the signed-sum / mutually-exclusive-shape rules.
func (s *Service) Create(st *Transaction) error {
	if err := s.validateScheduledTransaction(st); err != nil {
		return err
	}
	if err := s.validateScheduledSplits(st); err != nil {
		return err
	}
	if err := s.validateTransferCategory(st); err != nil {
		return err
	}
	if err := s.ensureNoClosedAccounts(st); err != nil {
		return err
	}
	// Persist the parent and all child splits in one transaction: a mid-flow
	// failure rolls the whole thing back, so a parent never lands without its
	// splits and no compensation delete is needed.
	return s.runInTx(func(b *Service) error {
		if err := b.repo.Create(st); err != nil {
			return err
		}
		for _, split := range st.Splits {
			split.ScheduledTransactionID = st.ID
			if err := b.repo.SplitRepo().Create(split); err != nil {
				return fmt.Errorf("failed to create scheduled split: %w", err)
			}
		}
		return nil
	})
}

// GetByID retrieves a scheduled transaction by its ID.
func (s *Service) GetByID(id types.ID) (*Transaction, error) {
	return s.repo.GetByID(id)
}

// Update validates and updates an existing scheduled transaction. If st
// carries Splits, the existing child rows are replaced (DELETE + INSERT) so
// the persisted template matches st.Splits exactly.
func (s *Service) Update(st *Transaction) error {
	if err := s.validateScheduledTransaction(st); err != nil {
		return err
	}
	if err := s.validateScheduledSplits(st); err != nil {
		return err
	}
	if err := s.validateTransferCategory(st); err != nil {
		return err
	}
	// NextDate is the date of the next pending occurrence; it must never
	// precede StartDate. When the user shifts StartDate forward past the
	// current NextDate, advance NextDate with it so the schedule list and
	// due-detection see the new anchor. Backward StartDate shifts leave
	// NextDate alone — an in-progress schedule shouldn't roll back to its
	// origin just because the user corrected the recorded anchor.
	if st.NextDate.Before(st.StartDate) {
		st.NextDate = st.StartDate
	}
	// Replace children and the parent in one transaction so a failure mid-way
	// can't leave the template with its old splits deleted but the new ones only
	// half-inserted. Clear existing children first: the parent's DELETE+INSERT
	// (under the hood of repo.Update) would otherwise trip the FK from
	// scheduled_split_items.
	return s.runInTx(func(b *Service) error {
		if _, err := b.repo.SplitRepo().DeleteByScheduledTransaction(st.ID); err != nil {
			return fmt.Errorf("failed to clear existing scheduled splits: %w", err)
		}
		if err := b.repo.Update(st); err != nil {
			return err
		}
		for _, split := range st.Splits {
			split.ScheduledTransactionID = st.ID
			if err := b.repo.SplitRepo().Create(split); err != nil {
				return fmt.Errorf("failed to insert updated scheduled split: %w", err)
			}
		}
		return nil
	})
}

// Delete removes a scheduled transaction.
func (s *Service) Delete(id types.ID) error {
	return s.repo.Delete(id)
}

// HealNextDates corrects rows whose NextDate precedes StartDate. Older
// binaries updated StartDate without syncing NextDate; this normalizes
// any poisoned rows by setting NextDate := StartDate. Intended to run
// once on file open. Returns the count of rows healed.
func (s *Service) HealNextDates() (int, error) {
	return s.repo.HealNextDates()
}

// List returns all scheduled transactions ordered by next_date ascending.
func (s *Service) List() ([]*Transaction, error) {
	return s.repo.List()
}

// ListByAccount returns all scheduled transactions for an account.
func (s *Service) ListByAccount(accountID types.ID) ([]*Transaction, error) {
	return s.repo.ListByAccount(accountID)
}

// ListDue returns all scheduled transactions that are due (next_date <= today).
func (s *Service) ListDue() ([]*Transaction, error) {
	return s.repo.ListDue()
}

// ListUpcoming returns scheduled transactions with next_date within the specified number of days.
func (s *Service) ListUpcoming(days int) ([]*Transaction, error) {
	return s.repo.ListUpcoming(days)
}

// ListReferencing returns every schedule that references the given account as
// its source, single-line transfer destination, or a transfer-line split
// target. It backs the soft warning shown when closing an account.
func (s *Service) ListReferencing(accountID types.ID) ([]*Transaction, error) {
	all, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	var out []*Transaction
	for _, st := range all {
		if slices.Contains(referencedAccountIDs(st), accountID) {
			out = append(out, st)
		}
	}
	return out, nil
}

// IsDue checks if a scheduled transaction is due (next_date <= today).
func (s *Service) IsDue(id types.ID) (bool, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return false, err
	}
	return st.IsDue(), nil
}

// IsCompleted checks if a scheduled transaction has finished all occurrences.
func (s *Service) IsCompleted(id types.ID) (bool, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return false, err
	}
	return st.IsCompleted(), nil
}

// GetNextDate returns the next occurrence date for a scheduled transaction.
func (s *Service) GetNextDate(id types.ID) (types.Date, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return types.Date{}, err
	}
	return st.NextDate, nil
}

// CalculateNextDate calculates what the next occurrence would be after the current next_date.
// Does not modify the scheduled transaction.
func (s *Service) CalculateNextDate(id types.ID) (types.Date, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return types.Date{}, err
	}
	return st.CalculateNextDate(), nil
}
