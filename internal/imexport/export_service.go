package imexport

import (
	"fmt"
	"io"

	"github.com/haskovec/tmoney/internal/models"
)

// ExportOptions configures the export behavior.
type ExportOptions struct {
	// Format specifies the output format (csv or qif).
	Format Format
	// AccountID filters transactions to a specific account. Nil means all accounts.
	AccountID *models.ID
	// StartDate filters transactions on or after this date. Nil means no lower bound.
	StartDate *models.Date
	// EndDate filters transactions on or before this date. Nil means no upper bound.
	EndDate *models.Date
}

// ExportResult holds the summary of an export operation.
type ExportResult struct {
	// TransactionCount is the number of transactions exported.
	TransactionCount int
	// AccountCount is the number of accounts included.
	AccountCount int
}

// AccountProvider retrieves accounts for export.
type AccountProvider interface {
	List(activeOnly bool) ([]*models.Account, error)
	GetByID(id models.ID) (*models.Account, error)
}

// TransactionProvider retrieves transactions for export.
type TransactionProvider interface {
	ListByAccount(accountID models.ID) ([]*models.Transaction, error)
	ListByAccountAndDateRange(accountID models.ID, startDate, endDate models.Date) ([]*models.Transaction, error)
}

// SplitProvider retrieves splits for export.
type SplitProvider interface {
	ListByTransaction(transactionID models.ID) ([]*models.Split, error)
}

// PayeeProvider retrieves payee names for export.
type PayeeProvider interface {
	GetByID(id models.ID) (*models.Payee, error)
}

// CategoryProvider retrieves categories for export.
type CategoryProvider interface {
	GetByID(id models.ID) (*models.Category, error)
	GetWithParent(id models.ID) (*models.Category, *models.Category, error)
}

// ExportService orchestrates the export workflow: query, resolve, and write.
type ExportService struct {
	accounts     AccountProvider
	transactions TransactionProvider
	splits       SplitProvider
	payees       PayeeProvider
	categories   CategoryProvider
}

// NewExportService creates a new ExportService.
func NewExportService(
	accounts AccountProvider,
	transactions TransactionProvider,
	splits SplitProvider,
	payees PayeeProvider,
	categories CategoryProvider,
) *ExportService {
	return &ExportService{
		accounts:     accounts,
		transactions: transactions,
		splits:       splits,
		payees:       payees,
		categories:   categories,
	}
}

// Export writes transactions to w in the specified format.
func (s *ExportService) Export(w io.Writer, opts ExportOptions) (*ExportResult, error) {
	if opts.Format != FormatCSV && opts.Format != FormatQIF {
		return nil, fmt.Errorf("unsupported export format: %s (must be csv or qif)", opts.Format)
	}

	// Collect accounts to export
	accounts, err := s.resolveAccounts(opts)
	if err != nil {
		return nil, fmt.Errorf("resolving accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts found for export")
	}

	// Build lookup caches to avoid repeated DB queries
	payeeCache := make(map[string]string)   // payeeID -> name
	categoryCache := make(map[string]string) // categoryID -> full path
	accountNameCache := make(map[string]string) // accountID -> name

	for _, acct := range accounts {
		accountNameCache[acct.ID.String()] = acct.Name
	}

	var allRecords []ExportRecord
	result := &ExportResult{}

	for _, acct := range accounts {
		txns, err := s.queryTransactions(acct.ID, opts)
		if err != nil {
			return nil, fmt.Errorf("querying transactions for account %s: %w", acct.Name, err)
		}

		for _, txn := range txns {
			// Skip void transactions
			if txn.IsVoid() {
				continue
			}

			record, err := s.buildExportRecord(txn, acct, payeeCache, categoryCache, accountNameCache)
			if err != nil {
				return nil, fmt.Errorf("building export record: %w", err)
			}

			allRecords = append(allRecords, *record)
			result.TransactionCount++
		}
	}

	if result.TransactionCount == 0 {
		return nil, fmt.Errorf("no transactions found for the given filters")
	}

	result.AccountCount = len(accounts)

	// Write in the requested format
	switch opts.Format {
	case FormatCSV:
		if err := WriteCSV(w, allRecords); err != nil {
			return nil, fmt.Errorf("writing CSV: %w", err)
		}
	case FormatQIF:
		// For QIF, use the account type of the first (or only) account
		accountType := string(accounts[0].Type)
		if err := WriteQIF(w, allRecords, accountType); err != nil {
			return nil, fmt.Errorf("writing QIF: %w", err)
		}
	}

	return result, nil
}

// resolveAccounts determines which accounts to export.
func (s *ExportService) resolveAccounts(opts ExportOptions) ([]*models.Account, error) {
	if opts.AccountID != nil {
		acct, err := s.accounts.GetByID(*opts.AccountID)
		if err != nil {
			return nil, err
		}
		return []*models.Account{acct}, nil
	}

	// All accounts (including closed, since they may have historical transactions)
	return s.accounts.List(false)
}

// queryTransactions fetches transactions for an account, optionally filtered by date range.
func (s *ExportService) queryTransactions(accountID models.ID, opts ExportOptions) ([]*models.Transaction, error) {
	if opts.StartDate != nil && opts.EndDate != nil {
		return s.transactions.ListByAccountAndDateRange(accountID, *opts.StartDate, *opts.EndDate)
	}

	txns, err := s.transactions.ListByAccount(accountID)
	if err != nil {
		return nil, err
	}

	// Apply partial date filters
	if opts.StartDate != nil || opts.EndDate != nil {
		var filtered []*models.Transaction
		for _, txn := range txns {
			if opts.StartDate != nil && txn.Date.Before(*opts.StartDate) {
				continue
			}
			if opts.EndDate != nil && txn.Date.After(*opts.EndDate) {
				continue
			}
			filtered = append(filtered, txn)
		}
		return filtered, nil
	}

	return txns, nil
}

// buildExportRecord converts a transaction to an ExportRecord, resolving related entities.
func (s *ExportService) buildExportRecord(
	txn *models.Transaction,
	acct *models.Account,
	payeeCache map[string]string,
	categoryCache map[string]string,
	accountNameCache map[string]string,
) (*ExportRecord, error) {
	record := &ExportRecord{
		Date:    txn.Date.String(),
		Account: acct.Name,
		Amount:  fmt.Sprintf("%.2f", txn.Amount.Float64()),
		Status:  txn.Status.Code(),
	}

	// Resolve payee
	if txn.HasPayee() {
		name, err := s.resolvePayeeName(txn.PayeeID.ID, payeeCache)
		if err == nil {
			record.Payee = name
		}
	}

	// Resolve memo
	if txn.Memo.Valid {
		record.Memo = txn.Memo.String
	}

	// Resolve check number
	if txn.CheckNumber.Valid {
		record.CheckNumber = txn.CheckNumber.String
	}

	// Resolve transfer account
	if txn.IsTransfer() {
		name, err := s.resolveAccountName(txn.TransferAccountID.ID, accountNameCache)
		if err == nil {
			record.TransferAccount = name
		}
	}

	// Resolve category or splits
	splits, err := s.splits.ListByTransaction(txn.ID)
	if err != nil {
		return nil, fmt.Errorf("listing splits for transaction %s: %w", txn.ID.String(), err)
	}

	if len(splits) > 0 {
		// Split transaction: leave Category empty on parent, add ExportSplits
		for _, split := range splits {
			catName, err := s.resolveCategoryPath(split.CategoryID, categoryCache)
			if err != nil {
				catName = ""
			}

			splitMemo := ""
			if split.Memo.Valid {
				splitMemo = split.Memo.String
			}

			record.Splits = append(record.Splits, ExportSplit{
				Category: catName,
				Amount:   fmt.Sprintf("%.2f", split.Amount.Float64()),
				Memo:     splitMemo,
			})
		}
	} else if txn.HasCategory() {
		// Regular transaction with category
		catName, err := s.resolveCategoryPath(txn.CategoryID.ID, categoryCache)
		if err == nil {
			record.Category = catName
		}
	}

	return record, nil
}

// resolvePayeeName returns the payee name, using cache.
func (s *ExportService) resolvePayeeName(payeeID models.ID, cache map[string]string) (string, error) {
	key := payeeID.String()
	if name, ok := cache[key]; ok {
		return name, nil
	}

	payee, err := s.payees.GetByID(payeeID)
	if err != nil {
		return "", err
	}

	cache[key] = payee.Name
	return payee.Name, nil
}

// resolveCategoryPath returns the full category path (e.g. "Food:Groceries"), using cache.
func (s *ExportService) resolveCategoryPath(categoryID models.ID, cache map[string]string) (string, error) {
	key := categoryID.String()
	if path, ok := cache[key]; ok {
		return path, nil
	}

	cat, parent, err := s.categories.GetWithParent(categoryID)
	if err != nil {
		return "", err
	}

	path := cat.Name
	if parent != nil {
		path = parent.Name + ":" + cat.Name
	}

	cache[key] = path
	return path, nil
}

// resolveAccountName returns the account name, using cache.
func (s *ExportService) resolveAccountName(accountID models.ID, cache map[string]string) (string, error) {
	key := accountID.String()
	if name, ok := cache[key]; ok {
		return name, nil
	}

	acct, err := s.accounts.GetByID(accountID)
	if err != nil {
		return "", err
	}

	cache[key] = acct.Name
	return acct.Name, nil
}
