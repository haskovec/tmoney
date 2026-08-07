package investment

import (
	"database/sql"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/types"
)

// ValuationService is extracted on the strength of one claim: it is a read
// model. It writes nothing, so it needs no transaction, so it needs no InTx and
// no *db.DB — which is why the phase-4 exit criteria about rebinding fields and
// rolling back partial writes do not apply to it.
//
// A claim that carries that much weight should not rest on reading the code. The
// test below proves it mechanically: every repository the type holds is bound to
// a queryer that refuses to Exec, and then the whole public surface is driven
// against real data. Any write — now or after a future edit — fails the test
// instead of quietly acquiring the ability to mutate.

// refuseWriteQueryer delegates reads and fails every Exec, so a single write
// anywhere behind it surfaces as an error rather than as a silent mutation.
type refuseWriteQueryer struct {
	inner  db.Queryer
	execs  int
	reads  int
	writes []string
}

func (q *refuseWriteQueryer) Exec(query string, args ...any) (sql.Result, error) {
	q.execs++
	q.writes = append(q.writes, query)
	return nil, errReadOnly
}

func (q *refuseWriteQueryer) Query(query string, args ...any) (*sql.Rows, error) {
	q.reads++
	return q.inner.Query(query, args...)
}

func (q *refuseWriteQueryer) QueryRow(query string, args ...any) *sql.Row {
	q.reads++
	return q.inner.QueryRow(query, args...)
}

func TestValuation_WritesNothing(t *testing.T) {
	env := createFullTestService(t)
	acct := createInvAccount(t, env.accountRepo, "Brokerage")
	sec := createSec(t, env.secRepo, "AAPL")
	date := types.NewDate(2024, time.March, 15)

	// Seed real holdings through the WRITE service, so the read model has
	// something substantial to value: cash, a position, a lot, and a price.
	if _, err := env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000"), ""); err != nil {
		t.Fatalf("Deposit(): %v", err)
	}
	total := types.MustNewMoney("1000")
	if _, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &total, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(): %v", err)
	}
	if _, err := env.svc.Dividend(acct.ID, sec.ID, date, types.MustNewMoney("25"), ""); err != nil {
		t.Fatalf("Dividend(): %v", err)
	}
	p := price.NewPrice(sec.ID, types.NewDate(2024, time.March, 20), types.MustNewMoney("120"), price.SourceManual)
	if err := env.priceRepo.Create(p); err != nil {
		t.Fatalf("Create price: %v", err)
	}

	// A second, lot-tracked account so both holdings paths are driven.
	lotAcct := createLotTrackingAccount(t, env.accountRepo, "IRA")
	if _, err := env.svc.Deposit(lotAcct.ID, date, types.MustNewMoney("5000"), ""); err != nil {
		t.Fatalf("Deposit(lot): %v", err)
	}
	lotTotal := types.MustNewMoney("500")
	if _, err := env.svc.Buy(lotAcct.ID, sec.ID, date, types.MustNewQuantity("5"), &lotTotal, nil, types.ZeroMoney, ""); err != nil {
		t.Fatalf("Buy(lot): %v", err)
	}

	// Build a ValuationService whose EVERY field is bound to the refusing
	// queryer. The struct is filled directly rather than through the
	// constructor, because the constructor builds holdingsRepo from the database
	// handle and that field must be covered too.
	q := &refuseWriteQueryer{inner: env.db.Conn()}
	val := &ValuationService{
		repo:                env.invRepo.WithTx(q),
		accountRepo:         env.accountRepo.WithTx(q),
		positionRepo:        env.positionRepo.WithTx(q),
		lotRepo:             env.lotRepo.WithTx(q),
		transactionLotRepo:  env.transactionLotRepo.WithTx(q),
		priceRepo:           env.priceRepo.WithTx(q),
		corporateActionRepo: env.caRepo.WithTx(q),
		holdingsRepo:        NewHoldingsRepository(env.db).WithTx(q),
	}

	asOf := types.NewDate(2024, time.March, 20)

	valuation, err := val.GetAccountValuation(acct.ID, asOf, ValuationOptions{IncludeClosed: true})
	if err != nil {
		t.Fatalf("GetAccountValuation(): %v", err)
	}
	if valuation.CashBalance.String() != "9025" {
		t.Errorf("cash balance = %q, want 9025 — the read model did not see the seeded data",
			valuation.CashBalance.String())
	}
	if len(valuation.Holdings) == 0 {
		t.Error("no holdings returned; the test would prove nothing about a surface it never reached")
	}

	holdings, err := val.GetHoldings(acct.ID, asOf, ValuationOptions{})
	if err != nil {
		t.Fatalf("GetHoldings(): %v", err)
	}
	if len(holdings) == 0 {
		t.Error("GetHoldings returned nothing")
	}

	// The lot-tracked account exercises the other holdings path and is the only
	// one GetLotDetail accepts.
	lots, err := val.GetLotDetail(lotAcct.ID, sec.ID, asOf)
	if err != nil {
		t.Fatalf("GetLotDetail(): %v", err)
	}
	if len(lots) == 0 {
		t.Error("GetLotDetail returned no lots")
	}
	if _, err := val.GetAccountValuation(lotAcct.ID, asOf, ValuationOptions{}); err != nil {
		t.Fatalf("GetAccountValuation(lot-tracked): %v", err)
	}

	if q.execs != 0 {
		t.Errorf("the read model issued %d write(s): %v", q.execs, q.writes)
	}
	// Without this the test would pass vacuously: if WithTx had not put the
	// wrapper in the path, every call would have gone straight to the pool and
	// the write count would be zero for the wrong reason.
	if q.reads == 0 {
		t.Error("the wrapper saw no reads either — it was not in the query path, so the zero write count proves nothing")
	}
}

var errReadOnly = errRefusedWrite{}

type errRefusedWrite struct{}

func (errRefusedWrite) Error() string {
	return "ValuationService attempted a write; it is a read model and must not"
}
