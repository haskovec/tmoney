package report

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/types"
)

// A closed account that was still open on the as-of date must count toward net
// worth on that date. Filtering on `active` alone dropped it from every
// historical report, understating net worth for the whole period it was open.
func TestService_NetWorthAsOf_AccountClosedAfterAsOfDate(t *testing.T) {
	svc, accountRepo, txnRepo := createTestReportService(t)

	opened := types.NewDate(2024, time.January, 1)
	acct := account.NewAccount("Old Savings", account.TypeSavings, "USD", types.MustNewMoney("1000.00"), opened)
	// Emptied on May 31 and closed on Jun 1.
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}
	txnRepo.createTransaction(t, acct.ID, types.NewDate(2024, time.May, 31), types.MustNewMoney("-1000.00"))
	acct.Close(types.NewDate(2024, time.June, 1))
	if err := accountRepo.Update(acct); err != nil {
		t.Fatalf("close account: %v", err)
	}

	// As of March 1 the account was open with 1000.
	rpt, err := svc.NetWorthAsOf(time.Date(2024, time.March, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NetWorthAsOf(March) error = %v", err)
	}
	if len(rpt.Assets) != 1 {
		t.Fatalf("March: expected the then-open account, got %d assets", len(rpt.Assets))
	}
	if want := types.MustNewMoney("1000.00"); !rpt.TotalAssets.Equal(want) {
		t.Errorf("March total assets = %s, want %s", rpt.TotalAssets, want)
	}

	// As of July 1 it was closed and is excluded.
	rpt, err = svc.NetWorthAsOf(time.Date(2024, time.July, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NetWorthAsOf(July) error = %v", err)
	}
	if len(rpt.Assets) != 0 {
		t.Errorf("July: expected the closed account to be excluded, got %d assets", len(rpt.Assets))
	}
}

// An account closed before migration 025 has no closed_date. Its close date is
// unknown, so it stays excluded from every dated report rather than being
// guessed into one.
func TestService_NetWorthAsOf_ClosedWithoutDateStaysExcluded(t *testing.T) {
	svc, accountRepo, _ := createTestReportService(t)

	acct := account.NewAccount("Legacy Closed", account.TypeChecking, "USD", types.MustNewMoney("500.00"), types.NewDate(2020, time.January, 1))
	acct.Active = false // closed, date unknown
	if err := accountRepo.Create(acct); err != nil {
		t.Fatalf("create account: %v", err)
	}

	rpt, err := svc.NetWorthAsOf(time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NetWorthAsOf() error = %v", err)
	}
	if len(rpt.Assets) != 0 {
		t.Errorf("expected a closed account with no closed_date to be excluded, got %d assets", len(rpt.Assets))
	}
}
