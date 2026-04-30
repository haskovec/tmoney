package imexport

import (
	"fmt"
	"strings"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// Adapters that wire the import service interfaces to concrete services and
// repositories. Both the CLI and TUI use these so the wiring stays in one
// place.

// NewServiceCategoryResolver returns a CategoryResolver backed by a
// category.Service. Hierarchical names like "Food:Groceries" resolve via
// parent lookup followed by child lookup.
func NewServiceCategoryResolver(svc *category.Service) CategoryResolver {
	return &serviceCategoryResolver{svc: svc}
}

type serviceCategoryResolver struct {
	svc *category.Service
}

func (r *serviceCategoryResolver) ResolveCategoryByName(name string) (types.ID, error) {
	parts := strings.SplitN(name, ":", 2)

	if len(parts) == 1 {
		cat, err := r.svc.GetByName(name, nil)
		if err != nil {
			return types.ID{}, err
		}
		return cat.ID, nil
	}

	parent, err := r.svc.GetByName(parts[0], nil)
	if err != nil {
		return types.ID{}, fmt.Errorf("parent category %q not found: %w", parts[0], err)
	}

	child, err := r.svc.GetByName(parts[1], &parent.ID)
	if err != nil {
		return types.ID{}, fmt.Errorf("subcategory %q not found under %q: %w", parts[1], parts[0], err)
	}

	return child.ID, nil
}

// NewServicePayeeResolver returns a PayeeResolver backed by a payee.Service.
// On lookup, missing payees are auto-created.
func NewServicePayeeResolver(svc *payee.Service) PayeeResolver {
	return &servicePayeeResolver{svc: svc}
}

type servicePayeeResolver struct {
	svc *payee.Service
}

func (r *servicePayeeResolver) ResolvePayee(name string) (types.ID, types.NullableID, error) {
	p, _, err := r.svc.ResolveOrCreate(name)
	if err != nil {
		return types.ID{}, types.NullableID{}, err
	}
	if p == nil {
		return types.ID{}, types.NullableID{}, nil
	}

	var defaultCatID types.NullableID
	if p.DefaultCategoryID.Valid {
		defaultCatID = p.DefaultCategoryID
	}
	return p.ID, defaultCatID, nil
}

// NewRepoTransactionStore returns a TransactionStore backed by transaction
// and payee repositories.
func NewRepoTransactionStore(transactionRepo *transaction.Repository, payeeRepo *payee.Repository) TransactionStore {
	return &repoTransactionStore{transactionRepo: transactionRepo, payeeRepo: payeeRepo}
}

type repoTransactionStore struct {
	transactionRepo *transaction.Repository
	payeeRepo       *payee.Repository
}

func (s *repoTransactionStore) ListByAccount(accountID types.ID) ([]*transaction.Transaction, error) {
	return s.transactionRepo.ListByAccount(accountID)
}

func (s *repoTransactionStore) GetPayeeName(payeeID types.ID) string {
	if payeeID.IsNil() {
		return ""
	}
	p, err := s.payeeRepo.GetByID(payeeID)
	if err != nil {
		return ""
	}
	return p.Name
}

func (s *repoTransactionStore) GetBankReferenceID(txn *transaction.Transaction) string {
	if txn.HasBankReferenceID() {
		return txn.BankReferenceID.String
	}
	return ""
}

// NewServiceTransactionCreator returns a TransactionCreator backed by a
// transaction.Service.
func NewServiceTransactionCreator(svc *transaction.Service) TransactionCreator {
	return &serviceTransactionCreator{svc: svc}
}

type serviceTransactionCreator struct {
	svc *transaction.Service
}

func (c *serviceTransactionCreator) CreateTransaction(txn *transaction.Transaction) error {
	return c.svc.Create(txn)
}

func (c *serviceTransactionCreator) CreateTransactionWithSplits(txn *transaction.Transaction, splits []*transaction.Split) error {
	return c.svc.CreateWithSplits(txn, splits)
}

func (c *serviceTransactionCreator) UpdateTransaction(txn *transaction.Transaction) error {
	return c.svc.Update(txn)
}
