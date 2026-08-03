package transfer

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/payee"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// harness bundles the transfer service under test with the two ledger services
// used to seed fixtures, plus the repositories tests poke directly.
//
// It deliberately does NOT go through internal/app: app imports internal/transfer,
// so an app import here would be a cycle. Wiring by hand also keeps the test
// honest about what transfer.NewService actually needs.
type harness struct {
	t *testing.T

	db  *db.DB
	svc *Service

	txnSvc *transaction.Service
	invSvc *investment.Service

	accountRepo  *account.Repository
	txnRepo      *transaction.Repository
	invRepo      *investment.Repository
	splitRepo    *transaction.SplitRepository
	categoryRepo *category.Repository
	securityRepo *security.Repository

	// Fixture accounts, one per type the four transfer shapes exercise.
	checking  *account.Account
	savings   *account.Account
	brokerage *account.Account
	ira       *account.Account
	hsa       *account.Account
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	database := dbtest.New(t)

	h := &harness{
		t:            t,
		db:           database,
		accountRepo:  account.NewRepository(database),
		txnRepo:      transaction.NewRepository(database),
		invRepo:      investment.NewRepository(database),
		splitRepo:    transaction.NewSplitRepository(database),
		categoryRepo: category.NewRepository(database),
		securityRepo: security.NewRepository(database),
	}

	payeeRepo := payee.NewRepository(database)
	transferRepo := transaction.NewTransferRepository(database, h.txnRepo)
	positionRepo := investment.NewPositionRepository(database)
	lotRepo := investment.NewLotRepository(database)
	transactionLotRepo := investment.NewTransactionLotRepository(database)
	priceRepo := price.NewRepository(database)
	corporateActionRepo := investment.NewCorporateActionRepository(database)

	h.txnSvc = transaction.NewService(h.txnRepo, h.splitRepo, transferRepo, payeeRepo, h.accountRepo, database)
	h.invSvc = investment.NewService(h.invRepo, h.accountRepo, positionRepo, lotRepo,
		transactionLotRepo, priceRepo, h.txnRepo, corporateActionRepo, database)
	// The transaction service mints investment-side counterparts for transfer
	// LINES inside splits through this port; without it a transfer line
	// targeting an investment account is refused.
	h.txnSvc.SetInvestmentCounterpart(h.invSvc)

	h.svc = NewService(h.txnRepo, h.invRepo, h.splitRepo, h.accountRepo, h.categoryRepo, database)

	open := types.NewDate(2000, time.January, 1)
	h.checking = h.newAccount("Checking", account.TypeChecking, "10000.00", open)
	h.savings = h.newAccount("Savings", account.TypeSavings, "5000.00", open)
	h.brokerage = h.newAccount("Brokerage", account.TypeInvestment, "0.00", open)
	h.ira = h.newAccount("Rollover IRA", account.TypeInvestment, "0.00", open)
	h.hsa = h.newAccount("HSA", account.TypeHSA, "0.00", open)

	return h
}

func (h *harness) newAccount(name string, t account.Type, opening string, openDate types.Date) *account.Account {
	h.t.Helper()
	a := account.NewAccount(name, t, "USD", types.MustNewMoney(opening), openDate)
	if err := h.accountRepo.Create(a); err != nil {
		h.t.Fatalf("setup: create account %q: %v", name, err)
	}
	return a
}

func (h *harness) newCategory(name string, t category.Type) *category.Category {
	h.t.Helper()
	c := category.NewCategory(name, t)
	if err := h.categoryRepo.Create(c); err != nil {
		h.t.Fatalf("setup: create category %q: %v", name, err)
	}
	return c
}

// testDate is a fixed date inside every fixture account's open window.
func testDate() types.Date { return types.NewDate(2024, time.June, 15) }

// seedRegToReg creates a bank↔bank transfer via the legacy path and returns its
// transfer_id plus both leg row IDs.
func (h *harness) seedRegToReg(amount string, categoryID types.NullableID) (types.ID, types.ID, types.ID) {
	h.t.Helper()
	pair, err := h.txnSvc.CreateTransfer(
		h.checking.ID, h.savings.ID, testDate(),
		types.MustNewMoney(amount), "rent reserve", categoryID,
	)
	if err != nil {
		h.t.Fatalf("seed reg↔reg: %v", err)
	}
	return pair.FromTransaction.TransferID.ID, pair.FromTransaction.ID, pair.ToTransaction.ID
}

// seedInvToReg creates a brokerage→checking cash transfer (investment leg
// negative) and returns transfer_id, investment leg ID, regular leg ID.
func (h *harness) seedInvToReg(amount string, categoryID types.NullableID) (types.ID, types.ID, types.ID) {
	h.t.Helper()
	res, err := h.invSvc.TransferCash(
		h.brokerage.ID, h.checking.ID, testDate(),
		types.MustNewMoney(amount), "cash sweep out", categoryID,
	)
	if err != nil {
		h.t.Fatalf("seed inv↔reg: %v", err)
	}
	return res.TransferID, res.InvestmentTransaction.ID, res.RegularTransaction.ID
}

// seedRegToInv creates a checking→brokerage cash transfer (investment leg
// positive) and returns transfer_id, investment leg ID, regular leg ID.
func (h *harness) seedRegToInv(amount string, categoryID types.NullableID) (types.ID, types.ID, types.ID) {
	h.t.Helper()
	res, err := h.invSvc.DepositFromAccount(
		h.brokerage.ID, h.checking.ID, testDate(),
		types.MustNewMoney(amount), "monthly contribution", categoryID,
	)
	if err != nil {
		h.t.Fatalf("seed reg↔inv: %v", err)
	}
	return res.TransferID, res.InvestmentTransaction.ID, res.RegularTransaction.ID
}

// seedInvToInv creates a brokerage→IRA cash transfer and returns transfer_id,
// source leg ID, destination leg ID.
func (h *harness) seedInvToInv(amount string) (types.ID, types.ID, types.ID) {
	h.t.Helper()
	res, err := h.invSvc.TransferCashBetweenInvestments(
		h.brokerage.ID, h.ira.ID, testDate(),
		types.MustNewMoney(amount), "IRA rollover",
	)
	if err != nil {
		h.t.Fatalf("seed inv↔inv: %v", err)
	}
	return res.TransferID, res.SourceTransaction.ID, res.DestinationTransaction.ID
}

// seedShareTransferPair writes a transfer_shares pair straight through the
// investment repository.
//
// It bypasses investment.Service.TransferShares deliberately: the unit under
// test is Movement classification in the read model, and going through the
// service would drag in positions, lots and FIFO allocation without making the
// two rows under test any different.
func (h *harness) seedShareTransferPair() (types.ID, types.ID, types.ID) {
	h.t.Helper()

	sec := security.NewSecurity("VTI", "Vanguard Total Stock Market ETF", security.TypeETF)
	if err := h.securityRepo.Create(sec); err != nil {
		h.t.Fatalf("seed share transfer: create security: %v", err)
	}

	transferID := types.NewID()
	shares := types.MustNewQuantity("10")

	src := investment.NewTransactionWithSecurity(h.brokerage.ID, testDate(),
		investment.TransactionTypeTransferShares, types.MustNewMoney("-1500.00"), sec.ID, shares)
	src.TransferID = types.NullableID{ID: transferID, Valid: true}
	src.TransferAccountID = types.NullableID{ID: h.ira.ID, Valid: true}
	if err := h.invRepo.Create(src); err != nil {
		h.t.Fatalf("seed share transfer: create source row: %v", err)
	}

	dst := investment.NewTransactionWithSecurity(h.ira.ID, testDate(),
		investment.TransactionTypeTransferShares, types.MustNewMoney("1500.00"), sec.ID, shares)
	dst.TransferID = types.NullableID{ID: transferID, Valid: true}
	dst.TransferAccountID = types.NullableID{ID: h.brokerage.ID, Valid: true}
	if err := h.invRepo.Create(dst); err != nil {
		h.t.Fatalf("seed share transfer: create dest row: %v", err)
	}

	return transferID, src.ID, dst.ID
}

// seedSplitTransferLine creates a real multi-line split in Checking whose
// second line is a transfer line targeting Savings, and returns the minted
// transfer_id, the counterpart leg's row ID, and the parent transaction ID.
func (h *harness) seedSplitTransferLine() (types.ID, types.ID, types.ID) {
	h.t.Helper()

	salary := h.newCategory("Salary", category.TypeIncome)
	savingsCat := h.newCategory("Savings Contribution", category.TypeExpense)

	parent := transaction.NewTransaction(h.checking.ID, testDate(), types.MustNewMoney("1000.00"))
	earnings := transaction.NewSplit(parent.ID, salary.ID, types.MustNewMoney("1200.00"))
	xfer := transaction.NewSplit(parent.ID, savingsCat.ID, types.MustNewMoney("-200.00"))
	xfer.TransferAccountID = types.NullableID{ID: h.savings.ID, Valid: true}

	if err := h.txnSvc.CreateWithSplits(parent, []*transaction.Split{earnings, xfer}); err != nil {
		h.t.Fatalf("seed split transfer line: %v", err)
	}

	splits, err := h.txnSvc.GetSplits(parent.ID)
	if err != nil {
		h.t.Fatalf("seed split transfer line: GetSplits: %v", err)
	}
	for _, s := range splits {
		if s.TransferID.Valid {
			legs, err := h.txnRepo.ListByTransferID(s.TransferID.ID)
			if err != nil {
				h.t.Fatalf("seed split transfer line: list legs: %v", err)
			}
			if len(legs) == 0 {
				h.t.Fatalf("seed split transfer line: no counterpart leg minted")
			}
			return s.TransferID.ID, legs[0].ID, parent.ID
		}
	}
	h.t.Fatalf("seed split transfer line: no transfer line found among %d splits", len(splits))
	return types.ID{}, types.ID{}, types.ID{}
}
