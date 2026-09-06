package imexport

import (
	"fmt"
	"io"
	"strings"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// ImportAction represents the action to take for an imported transaction.
type ImportAction string

const (
	// ImportActionNew creates a new transaction.
	ImportActionNew ImportAction = "new"
	// ImportActionMatch updates an existing matched transaction.
	ImportActionMatch ImportAction = "match"
	// ImportActionSkip skips the imported row.
	ImportActionSkip ImportAction = "skip"
	// ImportActionReview requires user review (low-confidence match).
	ImportActionReview ImportAction = "review"
)

// ImportRow represents a single row in the import review, combining the
// parsed record, match result, and the user-chosen action.
type ImportRow struct {
	Record     *ImportRecord
	Match      MatchResult
	Action     ImportAction
	CategoryID types.NullableID
	PayeeID    types.NullableID
}

// ImportResult holds the summary of an import operation.
type ImportResult struct {
	Rows     []ImportRow
	Created  int
	Updated  int
	Skipped  int
	Errors   []string
	DateFrom types.Date
	DateTo   types.Date
}

// TotalAmount returns the total amount of non-skipped rows.
func (r *ImportResult) TotalAmount() types.Money {
	total := types.ZeroMoney
	for _, row := range r.Rows {
		if row.Action != ImportActionSkip {
			total = total.Add(row.Record.Amount)
		}
	}
	return total
}

// NewCount returns the number of new rows.
func (r *ImportResult) NewCount() int {
	count := 0
	for _, row := range r.Rows {
		if row.Action == ImportActionNew {
			count++
		}
	}
	return count
}

// MatchCount returns the number of matched rows.
func (r *ImportResult) MatchCount() int {
	count := 0
	for _, row := range r.Rows {
		if row.Action == ImportActionMatch {
			count++
		}
	}
	return count
}

// SkipCount returns the number of skipped rows.
func (r *ImportResult) SkipCount() int {
	count := 0
	for _, row := range r.Rows {
		if row.Action == ImportActionSkip {
			count++
		}
	}
	return count
}

// ReviewCount returns the number of rows requiring review.
func (r *ImportResult) ReviewCount() int {
	count := 0
	for _, row := range r.Rows {
		if row.Action == ImportActionReview {
			count++
		}
	}
	return count
}

// DuplicateHandling specifies how to treat matched transactions.
type DuplicateHandling string

const (
	// DuplicateHandlingSkip skips matched rows (don't import duplicates).
	DuplicateHandlingSkip DuplicateHandling = "skip"
	// DuplicateHandlingUpdate updates existing matched transactions.
	DuplicateHandlingUpdate DuplicateHandling = "update"
	// DuplicateHandlingNone disables duplicate detection entirely.
	DuplicateHandlingNone DuplicateHandling = "none"
)

// ImportOptions configures the import behavior.
type ImportOptions struct {
	// Format overrides the auto-detected file format.
	Format Format
	// DuplicateHandling specifies how matched transactions are treated.
	DuplicateHandling DuplicateHandling
}

// CategoryResolver looks up a category by its hierarchical name (e.g. "Food:Groceries").
type CategoryResolver interface {
	ResolveCategoryByName(name string) (types.ID, error)
}

// PayeeResolver looks up or creates a payee by name and returns the payee ID
// and its default category ID (if any).
type PayeeResolver interface {
	ResolvePayee(name string) (payeeID types.ID, defaultCategoryID types.NullableID, err error)
}

// TransactionStore provides access to existing transactions for matching.
type TransactionStore interface {
	ListByAccount(accountID types.ID) ([]*transaction.Transaction, error)
	GetPayeeName(payeeID types.ID) string
	GetBankReferenceID(txn *transaction.Transaction) string
}

// TransactionCreator creates or updates transactions during import execution.
type TransactionCreator interface {
	CreateTransaction(txn *transaction.Transaction) error
	CreateTransactionWithSplits(txn *transaction.Transaction, splits []*transaction.Split) error
	UpdateTransaction(txn *transaction.Transaction) error
}

// ImportService orchestrates the import workflow: parse, match, categorize, and apply.
type ImportService struct {
	categoryResolver   CategoryResolver
	payeeResolver      PayeeResolver
	transactionStore   TransactionStore
	transactionCreator TransactionCreator
	matcher            *Matcher
}

// NewImportService creates a new ImportService.
func NewImportService(
	categoryResolver CategoryResolver,
	payeeResolver PayeeResolver,
	transactionStore TransactionStore,
	transactionCreator TransactionCreator,
) *ImportService {
	return &ImportService{
		categoryResolver:   categoryResolver,
		payeeResolver:      payeeResolver,
		transactionStore:   transactionStore,
		transactionCreator: transactionCreator,
		matcher:            NewDefaultMatcher(),
	}
}

// Parse reads and parses an import file, returning the intermediate records.
func (s *ImportService) Parse(r io.Reader, format Format) (*ParseResult, error) {
	switch format {
	case FormatCSV:
		return ParseCSV(r)
	case FormatQIF:
		return ParseQIF(r)
	case FormatOFX:
		return ParseOFX(r)
	default:
		return nil, fmt.Errorf("unsupported import format: %s", format)
	}
}

// Preview parses the file, runs matching and auto-categorization, and returns
// the import result ready for user review. No changes are made to the database.
//
// Callers that need to inspect or filter the parsed records before running
// the preview (e.g. picking one source account out of a multi-account CSV)
// should use Parse + PreviewRecords instead.
func (s *ImportService) Preview(r io.Reader, format Format, accountID types.ID, opts ImportOptions) (*ImportResult, error) {
	parseResult, err := s.Parse(r, format)
	if err != nil {
		return nil, fmt.Errorf("failed to parse import file: %w", err)
	}
	return s.PreviewRecords(parseResult, accountID, opts)
}

// PreviewRecords runs matching and auto-categorization against an already
// parsed (and optionally filtered) ParseResult.
func (s *ImportService) PreviewRecords(parseResult *ParseResult, accountID types.ID, opts ImportOptions) (*ImportResult, error) {
	if parseResult == nil {
		return nil, fmt.Errorf("parseResult is nil")
	}

	if len(parseResult.Records) == 0 && !parseResult.HasErrors() {
		return nil, fmt.Errorf("no transactions found in import file")
	}

	result := &ImportResult{}

	// Collect parse errors
	for _, pe := range parseResult.Errors {
		result.Errors = append(result.Errors, pe.Error())
	}

	if len(parseResult.Records) == 0 {
		return result, nil
	}

	// Get existing transactions for matching
	var matchResults []MatchResult
	if opts.DuplicateHandling != DuplicateHandlingNone {
		existing, err := s.buildExistingTransactions(accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to load existing transactions: %w", err)
		}
		matchResults = s.matcher.MatchAll(parseResult.Records, existing)
	} else {
		// No matching - all records are new
		matchResults = make([]MatchResult, len(parseResult.Records))
		for i := range parseResult.Records {
			matchResults[i] = MatchResult{
				ImportRecord: &parseResult.Records[i],
				Confidence:   MatchConfidenceNone,
			}
		}
	}

	// Build import rows with actions and auto-categorization
	result.Rows = make([]ImportRow, len(matchResults))
	for i, mr := range matchResults {
		row := ImportRow{
			Record: mr.ImportRecord,
			Match:  mr,
		}

		// Determine action based on match confidence and duplicate handling
		switch {
		case mr.Confidence == MatchConfidenceNone:
			row.Action = ImportActionNew
		case mr.Confidence == MatchConfidenceHigh && opts.DuplicateHandling == DuplicateHandlingSkip:
			row.Action = ImportActionSkip
		case mr.Confidence == MatchConfidenceHigh && opts.DuplicateHandling == DuplicateHandlingUpdate:
			row.Action = ImportActionMatch
		case mr.Confidence == MatchConfidenceLow:
			row.Action = ImportActionReview
		default:
			row.Action = ImportActionNew
		}

		// Auto-categorize and resolve payee
		s.autoCategorize(&row)

		result.Rows[i] = row
	}

	// Calculate date range
	s.calculateDateRange(result)

	return result, nil
}

// Execute applies the import actions to the database.
// It processes all non-skipped rows: creates new transactions and updates matched ones.
func (s *ImportService) Execute(result *ImportResult, accountID types.ID) error {
	result.Created = 0
	result.Updated = 0
	result.Skipped = 0

	for i := range result.Rows {
		row := &result.Rows[i]

		switch row.Action {
		case ImportActionSkip:
			result.Skipped++
			continue

		case ImportActionNew, ImportActionReview:
			txn, splits, err := s.buildTransaction(row, accountID)
			if err != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("row %d: failed to build transaction: %v", row.Record.SourceLine, err))
				continue
			}

			if len(splits) > 0 {
				if err := s.transactionCreator.CreateTransactionWithSplits(txn, splits); err != nil {
					result.Errors = append(result.Errors,
						fmt.Sprintf("row %d: failed to create split transaction: %v", row.Record.SourceLine, err))
					continue
				}
			} else {
				if err := s.transactionCreator.CreateTransaction(txn); err != nil {
					result.Errors = append(result.Errors,
						fmt.Sprintf("row %d: failed to create transaction: %v", row.Record.SourceLine, err))
					continue
				}
			}
			result.Created++

		case ImportActionMatch:
			if row.Match.MatchedTransaction == nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("row %d: match action but no matched transaction", row.Record.SourceLine))
				continue
			}

			txn := row.Match.MatchedTransaction
			updated := false

			// Update status to cleared if currently uncleared
			if txn.Status == transaction.StatusUncleared {
				txn.SetStatus(transaction.StatusCleared)
				updated = true
			}

			// Add bank reference ID if available
			if row.Record.BankReferenceID != "" && !txn.HasBankReferenceID() {
				txn.SetBankReferenceID(row.Record.BankReferenceID)
				updated = true
			}

			if updated {
				if err := s.transactionCreator.UpdateTransaction(txn); err != nil {
					result.Errors = append(result.Errors,
						fmt.Sprintf("row %d: failed to update matched transaction: %v", row.Record.SourceLine, err))
					continue
				}
			}
			result.Updated++
		}
	}

	return nil
}

// buildExistingTransactions loads existing transactions and wraps them for matching.
func (s *ImportService) buildExistingTransactions(accountID types.ID) ([]ExistingTransaction, error) {
	txns, err := s.transactionStore.ListByAccount(accountID)
	if err != nil {
		return nil, err
	}

	existing := make([]ExistingTransaction, len(txns))
	for i, txn := range txns {
		existing[i] = ExistingTransaction{
			Transaction:     txn,
			PayeeName:       s.transactionStore.GetPayeeName(txn.PayeeID.ID),
			BankReferenceID: s.transactionStore.GetBankReferenceID(txn),
		}
	}

	return existing, nil
}

// autoCategorize resolves payee and category for an import row.
func (s *ImportService) autoCategorize(row *ImportRow) {
	// Resolve payee
	if row.Record.Payee != "" && s.payeeResolver != nil {
		payeeID, defaultCatID, err := s.payeeResolver.ResolvePayee(row.Record.Payee)
		if err == nil && !payeeID.IsNil() {
			row.PayeeID = types.NullableID{ID: payeeID, Valid: true}

			// Use payee's default category if no category specified
			if row.Record.Category == "" && !row.Record.IsSplit() && defaultCatID.Valid {
				row.CategoryID = defaultCatID
			}
		}
	}

	// Resolve explicit category from import record
	if row.Record.Category != "" && !row.Record.IsSplit() && s.categoryResolver != nil {
		catID, err := s.categoryResolver.ResolveCategoryByName(row.Record.Category)
		if err == nil && !catID.IsNil() {
			row.CategoryID = types.NullableID{ID: catID, Valid: true}
		}
	}
}

// buildTransaction creates a transaction.Transaction from an import row.
func (s *ImportService) buildTransaction(row *ImportRow, accountID types.ID) (*transaction.Transaction, []*transaction.Split, error) {
	txn := transaction.NewTransaction(accountID, row.Record.Date, row.Record.Amount)

	// Set payee
	if row.PayeeID.Valid {
		txn.SetPayee(row.PayeeID.ID)
	}

	// Set category (only for non-split transactions)
	if row.CategoryID.Valid && !row.Record.IsSplit() {
		txn.SetCategory(row.CategoryID.ID)
	}

	// Set memo
	if row.Record.Memo != "" {
		txn.SetMemo(row.Record.Memo)
	}

	// Set check number
	if row.Record.CheckNumber != "" {
		txn.SetCheckNumber(row.Record.CheckNumber)
	}

	// Set bank reference ID
	if row.Record.BankReferenceID != "" {
		txn.SetBankReferenceID(row.Record.BankReferenceID)
	}

	// Set status from import
	if row.Record.Status != "" {
		status := parseImportStatus(row.Record.Status)
		txn.SetStatus(status)
	}

	// Build splits if applicable
	var splits []*transaction.Split
	if row.Record.IsSplit() && s.categoryResolver != nil {
		for _, importSplit := range row.Record.Splits {
			catID, err := s.categoryResolver.ResolveCategoryByName(importSplit.Category)
			if err != nil || catID.IsNil() {
				continue // Skip splits with unresolvable categories
			}

			split := transaction.NewSplit(txn.ID, catID, importSplit.Amount)
			if importSplit.Memo != "" {
				split.SetMemo(importSplit.Memo)
			}
			splits = append(splits, split)
		}

		// If we have splits, clear the category on the parent transaction
		if len(splits) > 0 {
			txn.ClearCategory()
		}
	}

	return txn, splits, nil
}

// calculateDateRange sets the date range on the import result.
func (s *ImportService) calculateDateRange(result *ImportResult) {
	if len(result.Rows) == 0 {
		return
	}

	first := true
	for _, row := range result.Rows {
		if first {
			result.DateFrom = row.Record.Date
			result.DateTo = row.Record.Date
			first = false
			continue
		}
		if row.Record.Date.Before(result.DateFrom) {
			result.DateFrom = row.Record.Date
		}
		if row.Record.Date.After(result.DateTo) {
			result.DateTo = row.Record.Date
		}
	}
}

// parseImportStatus converts a status code from an import file to a
// TransactionStatus. It accepts TMoney's own codes (C, R, U — what the QIF
// parser and the CSV exporter emit) and Quicken's raw marks, where `*` is
// cleared and `X` is reconciled (see qifStatusToImport).
func parseImportStatus(status string) transaction.Status {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "C", "*":
		return transaction.StatusCleared
	case "R", "X":
		return transaction.StatusReconciled
	default:
		return transaction.StatusUncleared
	}
}
