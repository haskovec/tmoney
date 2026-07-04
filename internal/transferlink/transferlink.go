// Package transferlink links pairs of existing transactions that look like
// the two halves of a single transfer (typically created by importing each
// account's QIF/OFX/CSV separately) into proper TMoney transfers.
//
// The matcher is a post-hoc heuristic: for each pair of distinct accounts
// it looks for transactions where one side has amount = -X and the other
// has amount = +X, posted within a configurable number of days of each
// other. Pairs that are unique (each side matches only one candidate) are
// considered "clean" and can be linked automatically; everything else is
// flagged as "ambiguous" so the user can resolve it by hand.
package transferlink

import (
	"fmt"
	"sort"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// DefaultMaxDateDiffDays is the default tolerance window when finding
// transfer pairs. Banks often post the two sides of an internal transfer
// on different days; five days is generous enough for almost all real
// transfers without producing too many spurious matches.
const DefaultMaxDateDiffDays = 5

// Candidate is a pair of transactions that look like the two sides of a
// transfer.
type Candidate struct {
	From         *transaction.Transaction // negative-amount side ("money out")
	To           *transaction.Transaction // positive-amount side ("money in")
	FromAccount  string
	ToAccount    string
	DateDiffDays int
}

// Result groups the candidates found in a single FindUnlinked call.
type Result struct {
	// Clean candidates can be linked automatically. Each transaction in a
	// clean candidate appears in exactly one candidate, so there is no
	// pairing ambiguity.
	Clean []*Candidate
	// Ambiguous candidates have at least one transaction that matched more
	// than one possible counterpart. They are surfaced for review but not
	// linked automatically.
	Ambiguous []*Candidate
	// Scanned is the number of unlinked transactions that were eligible
	// for matching after filters were applied.
	Scanned int
}

// Service finds and links transfer candidates.
type Service struct {
	txnRepo      *transaction.Repository
	transferRepo *transaction.TransferRepository
	splitRepo    *transaction.SplitRepository
	accountRepo  *account.Repository
}

// NewService wires up the transferlink service.
func NewService(
	txnRepo *transaction.Repository,
	transferRepo *transaction.TransferRepository,
	splitRepo *transaction.SplitRepository,
	accountRepo *account.Repository,
) *Service {
	return &Service{
		txnRepo:      txnRepo,
		transferRepo: transferRepo,
		splitRepo:    splitRepo,
		accountRepo:  accountRepo,
	}
}

// FindUnlinked scans all transactions and returns candidate transfer
// pairs. maxDateDiffDays controls how far apart two postings can be and
// still be considered a match.
func (s *Service) FindUnlinked(maxDateDiffDays int) (*Result, error) {
	if maxDateDiffDays < 0 {
		maxDateDiffDays = DefaultMaxDateDiffDays
	}

	allTxns, err := s.txnRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list transactions: %w", err)
	}

	accounts, err := s.accountRepo.List(false)
	if err != nil {
		return nil, fmt.Errorf("failed to list accounts: %w", err)
	}
	acctByID := make(map[types.ID]*account.Account, len(accounts))
	for _, a := range accounts {
		acctByID[a.ID] = a
	}

	// Filter out transactions that aren't eligible for transfer linking.
	eligible := make([]*transaction.Transaction, 0, len(allTxns))
	for _, t := range allTxns {
		ok, err := s.isEligible(t, acctByID)
		if err != nil {
			return nil, err
		}
		if ok {
			eligible = append(eligible, t)
		}
	}

	// Build candidates: every (negative, positive) pair of eligible
	// transactions across different accounts whose amounts sum to zero
	// and whose dates are within the tolerance window.
	var candidates []*Candidate
	for i, ti := range eligible {
		if !ti.Amount.IsNegative() {
			continue
		}
		for j, tj := range eligible {
			if i == j || !tj.Amount.IsPositive() {
				continue
			}
			if ti.AccountID == tj.AccountID {
				continue
			}
			if !ti.Amount.Add(tj.Amount).IsZero() {
				continue
			}
			diff := absDays(ti.Date, tj.Date)
			if diff > maxDateDiffDays {
				continue
			}
			candidates = append(candidates, &Candidate{
				From:         ti,
				To:           tj,
				FromAccount:  acctByID[ti.AccountID].Name,
				ToAccount:    acctByID[tj.AccountID].Name,
				DateDiffDays: diff,
			})
		}
	}

	// Tally how many candidates each transaction participates in. A
	// candidate is "clean" only when both of its transactions appear in
	// exactly one candidate.
	counts := make(map[types.ID]int, len(eligible)*2)
	for _, c := range candidates {
		counts[c.From.ID]++
		counts[c.To.ID]++
	}

	res := &Result{Scanned: len(eligible)}
	for _, c := range candidates {
		if counts[c.From.ID] == 1 && counts[c.To.ID] == 1 {
			res.Clean = append(res.Clean, c)
		} else {
			res.Ambiguous = append(res.Ambiguous, c)
		}
	}

	// Stable presentation order: oldest first, with the closer date diffs
	// listed before farther ones when dates tie.
	sortCandidates(res.Clean)
	sortCandidates(res.Ambiguous)

	return res, nil
}

// Link converts each candidate into a real transfer by setting matching
// transfer IDs on both sides and writing them back via the underlying
// transfer repository. Candidates with mismatched amounts or other
// validation problems are skipped and reported in the returned errors.
func (s *Service) Link(candidates []*Candidate) (linked int, errs []error) {
	for _, c := range candidates {
		if err := s.linkOne(c); err != nil {
			errs = append(errs, fmt.Errorf("link %s ↔ %s on %s: %w",
				c.FromAccount, c.ToAccount, c.From.Date, err))
			continue
		}
		linked++
	}
	return linked, errs
}

// linkOne writes the transfer linkage for a single candidate. Both sides
// of an existing pair already have IDs and persistence rows, so we just
// stamp them with a shared transfer ID and call the transfer repo's
// Update path (which validates and persists both rows).
func (s *Service) linkOne(c *Candidate) error {
	if c.From.IsTransfer() || c.To.IsTransfer() {
		return fmt.Errorf("one or both transactions are already transfers")
	}
	if c.From.AccountID == c.To.AccountID {
		return fmt.Errorf("from and to accounts must differ")
	}
	if !c.From.Amount.Add(c.To.Amount).IsZero() {
		return fmt.Errorf("amounts do not net to zero")
	}

	transferID := types.NewID()
	c.From.SetTransfer(transferID, c.To.AccountID)
	c.To.SetTransfer(transferID, c.From.AccountID)

	// A transfer carries at most one shared category. Normalize the two legs to
	// a single value before writing them back:
	//   - exactly one leg categorized → mirror it onto the other;
	//   - both categorized and different → the outflow (From) leg wins;
	//   - both the same or both empty → leave them untouched.
	// Capture the originals so the error rollback can restore them alongside the
	// transfer fields (importing separately may have left the rows categorized).
	origFromCat := c.From.CategoryID
	origToCat := c.To.CategoryID
	switch {
	case c.From.HasCategory() && !c.To.HasCategory():
		c.To.SetCategory(c.From.CategoryID.ID)
	case !c.From.HasCategory() && c.To.HasCategory():
		c.From.SetCategory(c.To.CategoryID.ID)
	case c.From.HasCategory() && c.To.HasCategory() && c.From.CategoryID.ID != c.To.CategoryID.ID:
		c.To.SetCategory(c.From.CategoryID.ID)
	}

	pair := &transaction.TransferPair{FromTransaction: c.From, ToTransaction: c.To}
	if err := s.transferRepo.Update(pair); err != nil {
		// Roll back the in-memory link and categories so a partial failure
		// doesn't leave the caller's structs in an invalid state if they retry.
		c.From.ClearTransfer()
		c.To.ClearTransfer()
		c.From.CategoryID = origFromCat
		c.To.CategoryID = origToCat
		return err
	}
	return nil
}

// isEligible decides whether a transaction can participate in transfer
// linking. The filters mirror the constraints on real transfers:
// non-void, no splits, not already a transfer, not in an investment
// account (those go through the dedicated InvestmentService), and a
// non-zero amount.
func (s *Service) isEligible(t *transaction.Transaction, acctByID map[types.ID]*account.Account) (bool, error) {
	if t.IsTransfer() || t.IsVoid() || t.Amount.IsZero() {
		return false, nil
	}
	a, ok := acctByID[t.AccountID]
	if !ok || !a.Active {
		return false, nil
	}
	if a.Type.IsInvestmentType() {
		return false, nil
	}
	count, err := s.splitRepo.CountByTransaction(t.ID)
	if err != nil {
		return false, fmt.Errorf("failed to count splits for %s: %w", t.ID.String(), err)
	}
	if count > 0 {
		return false, nil
	}
	return true, nil
}

// absDays returns the absolute difference in calendar days between two
// dates.
func absDays(a, b types.Date) int {
	diff := a.Time().Sub(b.Time()) / (24 * time.Hour)
	if diff < 0 {
		diff = -diff
	}
	return int(diff)
}

func sortCandidates(cs []*Candidate) {
	sort.SliceStable(cs, func(i, j int) bool {
		if !cs[i].From.Date.Equal(cs[j].From.Date) {
			return cs[i].From.Date.Before(cs[j].From.Date)
		}
		if cs[i].DateDiffDays != cs[j].DateDiffDays {
			return cs[i].DateDiffDays < cs[j].DateDiffDays
		}
		return cs[i].FromAccount < cs[j].FromAccount
	})
}
