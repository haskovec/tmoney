package loan_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/dbtest"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// setupLoanDB creates a temp DB with a single Checking funding account and
// returns its path. The DB is closed so the CLI can reopen it.
func setupLoanDB(t *testing.T) string {
	t.Helper()
	database, dbPath := dbtest.NewFile(t, "loan.tdb")
	repo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD",
		types.MustNewMoney("50000.00"), types.NewDate(2000, time.January, 1))
	if err := repo.Create(checking); err != nil {
		t.Fatalf("setup: create checking: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("setup: db.Close: %v", err)
	}
	return dbPath
}

// runLoan drives the CLI by argv and returns combined stdout plus the error.
func runLoan(t *testing.T, args ...string) (string, error) {
	t.Helper()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith(args, stdout, stderr)
	return stdout.String(), err
}

func TestLoanAdd_MissingFile(t *testing.T) {
	_, err := runLoan(t, "loan", "add", "--name", "M", "--rate", "6.5",
		"--from-account", "Checking", "--next-payment-date", "2026-08-01", "--payment", "100")
	if err == nil {
		t.Fatal("loan add without --file should error")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention file, got: %v", err)
	}
}

func TestLoanAdd_MissingRequiredFlags(t *testing.T) {
	dbPath := setupLoanDB(t)
	// Missing --rate.
	_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "M",
		"--from-account", "Checking", "--next-payment-date", "2026-08-01", "--payment", "100")
	if err == nil || !strings.Contains(err.Error(), "rate") {
		t.Errorf("expected required-flag error mentioning rate, got: %v", err)
	}
}

func TestLoanAdd_CreatesLoanScheduleAndAsset(t *testing.T) {
	dbPath := setupLoanDB(t)
	out, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Mortgage", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "2401.86", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking", "--institution", "Wells Fargo Home",
		"--escrow", "Housing:Property Tax=650", "--escrow", "Housing:Home Insurance=120",
		"--payee", "Wells Fargo", "--asset-name", "123 Main St", "--asset-value", "450000")
	if err != nil {
		t.Fatalf("loan add: %v (out: %s)", err, out)
	}
	if !strings.Contains(out, "Loan created successfully") {
		t.Errorf("missing success line; got: %s", out)
	}

	svc := clitest.OpenSvc(t, dbPath)

	// Loan account: liability stored negative, APR + institution set.
	loanAcct, err := svc.Account.GetByName("Mortgage")
	if err != nil {
		t.Fatalf("loan account not created: %v", err)
	}
	if loanAcct.Type != account.TypeLoan {
		t.Errorf("loan account type = %v, want loan", loanAcct.Type)
	}
	if want := types.MustNewMoney("-312450.22"); !loanAcct.OpeningBalance.Equal(want) {
		t.Errorf("loan opening balance = %s, want %s (stored negative)", loanAcct.OpeningBalance, want)
	}
	if !loanAcct.InterestRate.Valid || !loanAcct.InterestRate.Money.Equal(types.MustNewMoney("6.5")) {
		t.Errorf("loan APR = %v, want 6.5", loanAcct.InterestRate)
	}
	if !loanAcct.Institution.Valid || loanAcct.Institution.String != "Wells Fargo Home" {
		t.Errorf("loan institution = %v, want Wells Fargo Home", loanAcct.Institution)
	}

	// Asset account.
	assetAcct, err := svc.Account.GetByName("123 Main St")
	if err != nil {
		t.Fatalf("asset account not created: %v", err)
	}
	if assetAcct.Type != account.TypeAsset {
		t.Errorf("asset account type = %v, want asset", assetAcct.Type)
	}
	if want := types.MustNewMoney("450000"); !assetAcct.OpeningBalance.Equal(want) {
		t.Errorf("asset opening balance = %s, want %s", assetAcct.OpeningBalance, want)
	}

	// Payee auto-created.
	if _, err := svc.Payee.GetByName("Wells Fargo"); err != nil {
		t.Errorf("payee Wells Fargo not created: %v", err)
	}

	// Schedule: loan-shaped, monthly, day-of-month 1, indefinite, auto-post off.
	checking, _ := svc.Account.GetByName("Checking")
	scheds, err := svc.Scheduled.ListByAccount(checking.ID)
	if err != nil {
		t.Fatalf("list schedules: %v", err)
	}
	if len(scheds) != 1 {
		t.Fatalf("got %d schedules, want 1", len(scheds))
	}
	st := scheds[0]
	if !svc.Scheduled.IsLoanShaped(st) {
		t.Error("created schedule is not loan-shaped")
	}
	if st.Frequency != scheduled.FrequencyMonthly {
		t.Errorf("frequency = %v, want monthly", st.Frequency)
	}
	if !st.DayOfMonth.Valid || st.DayOfMonth.Int64 != 1 {
		t.Errorf("day-of-month = %v, want 1", st.DayOfMonth)
	}
	if st.OccurrencesRemaining.Valid {
		t.Errorf("schedule should be indefinite, got occurrences-remaining = %d", st.OccurrencesRemaining.Int64)
	}
	if st.IsAutoPost() {
		t.Error("auto-post should default off")
	}

	// Parent draft = -(P&I 2401.86 + escrow 650 + 120) = -3171.86; lines sum to it.
	if want := types.MustNewMoney("-3171.86"); !st.Amount.Money.Equal(want) {
		t.Errorf("parent amount = %s, want %s", st.Amount.Money, want)
	}
	var interest, principal, escrow int
	sum := types.ZeroMoney
	for _, sp := range st.Splits {
		sum = sum.Add(sp.Amount)
		if !sp.LoanSection.Valid {
			t.Errorf("split %s missing loan_section tag", sp.Amount)
			continue
		}
		switch sp.LoanSection.String {
		case scheduled.LoanSectionInterest:
			interest++
		case scheduled.LoanSectionPrincipal:
			principal++
			if !sp.TransferAccountID.Valid || sp.TransferAccountID.ID != loanAcct.ID {
				t.Errorf("principal transfer target = %v, want loan %v", sp.TransferAccountID, loanAcct.ID)
			}
		case scheduled.LoanSectionEscrow:
			escrow++
		}
	}
	if interest != 1 || principal != 1 || escrow != 2 {
		t.Errorf("split tags = interest %d / principal %d / escrow %d, want 1/1/2", interest, principal, escrow)
	}
	if !sum.Equal(st.Amount.Money) {
		t.Errorf("split sum %s != parent %s", sum, st.Amount.Money)
	}

	// Default Loan:Interest category get-or-created.
	parent, err := svc.CategoryRepo.GetByName("Loan", nil)
	if err != nil {
		t.Fatalf("Loan parent category not created: %v", err)
	}
	if _, err := svc.CategoryRepo.GetByName("Interest", &parent.ID); err != nil {
		t.Errorf("Loan:Interest child not created: %v", err)
	}
}

// principalSplit returns the loan schedule's principal transfer line.
func principalSplit(t *testing.T, st *scheduled.Transaction) *scheduled.Split {
	t.Helper()
	for _, sp := range st.Splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == scheduled.LoanSectionPrincipal {
			return sp
		}
	}
	t.Fatal("no principal split in schedule")
	return nil
}

func TestLoanAdd_DefaultPrincipalCategory(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Mortgage", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "2401.86", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking"); err != nil {
		t.Fatalf("loan add: %v", err)
	}
	svc := clitest.OpenSvc(t, dbPath)

	// Default Loan:Principal category get-or-created.
	loanParent, err := svc.CategoryRepo.GetByName("Loan", nil)
	if err != nil {
		t.Fatalf("Loan parent not created: %v", err)
	}
	principalCat, err := svc.CategoryRepo.GetByName("Principal", &loanParent.ID)
	if err != nil {
		t.Fatalf("Loan:Principal child not created: %v", err)
	}

	// Principal transfer line labeled with it.
	checking, _ := svc.Account.GetByName("Checking")
	scheds, _ := svc.Scheduled.ListByAccount(checking.ID)
	if len(scheds) != 1 {
		t.Fatalf("got %d schedules, want 1", len(scheds))
	}
	p := principalSplit(t, scheds[0])
	if !p.CategoryID.Valid || p.CategoryID.ID != principalCat.ID {
		t.Errorf("principal category = %v, want Loan:Principal %v", p.CategoryID, principalCat.ID)
	}
}

func TestLoanAdd_ExplicitPrincipalCategory(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Mortgage", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "2401.86", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking", "--principal-category", "Housing:Principal"); err != nil {
		t.Fatalf("loan add: %v", err)
	}
	svc := clitest.OpenSvc(t, dbPath)

	// The explicit path is created…
	housing, err := svc.CategoryRepo.GetByName("Housing", nil)
	if err != nil {
		t.Fatalf("Housing parent not created: %v", err)
	}
	principalCat, err := svc.CategoryRepo.GetByName("Principal", &housing.ID)
	if err != nil {
		t.Fatalf("Housing:Principal not created: %v", err)
	}
	checking, _ := svc.Account.GetByName("Checking")
	scheds, _ := svc.Scheduled.ListByAccount(checking.ID)
	p := principalSplit(t, scheds[0])
	if !p.CategoryID.Valid || p.CategoryID.ID != principalCat.ID {
		t.Errorf("principal category = %v, want Housing:Principal %v", p.CategoryID, principalCat.ID)
	}

	// …and the default Loan:Principal is NOT created (the Loan parent still
	// exists from the interest default, but has no Principal child).
	if loanParent, err := svc.CategoryRepo.GetByName("Loan", nil); err == nil {
		if _, err := svc.CategoryRepo.GetByName("Principal", &loanParent.ID); err == nil {
			t.Error("Loan:Principal should not be created when --principal-category is explicit")
		}
	}
}

func TestLoanAdd_EmptyPrincipalCategoryUnlabeled(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Mortgage", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "2401.86", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking", "--principal-category", ""); err != nil {
		t.Fatalf("loan add: %v", err)
	}
	svc := clitest.OpenSvc(t, dbPath)

	checking, _ := svc.Account.GetByName("Checking")
	scheds, _ := svc.Scheduled.ListByAccount(checking.ID)
	p := principalSplit(t, scheds[0])
	if p.CategoryID.Valid {
		t.Errorf(`principal category = %v, want unset (--principal-category "")`, p.CategoryID)
	}

	// No Loan:Principal was created.
	if loanParent, err := svc.CategoryRepo.GetByName("Loan", nil); err == nil {
		if _, err := svc.CategoryRepo.GetByName("Principal", &loanParent.ID); err == nil {
			t.Error(`Loan:Principal should not be created for --principal-category ""`)
		}
	}
}

func TestLoanAdd_ComputedPayment(t *testing.T) {
	dbPath := setupLoanDB(t)
	// 32000 @ 5.9% / 60mo → 617.16 (Phase 3 fixture).
	out, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Car Loan", "--principal", "32000", "--rate", "5.9",
		"--term-months", "60", "--open-date", "2026-07-01",
		"--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err != nil {
		t.Fatalf("loan add: %v (out: %s)", err, out)
	}
	if !strings.Contains(out, "617.16") {
		t.Errorf("expected computed payment 617.16 in output, got: %s", out)
	}
	if !strings.Contains(out, "computed") {
		t.Errorf("expected 'computed' note in output, got: %s", out)
	}

	svc := clitest.OpenSvc(t, dbPath)
	checking, _ := svc.Account.GetByName("Checking")
	scheds, _ := svc.Scheduled.ListByAccount(checking.ID)
	if len(scheds) != 1 {
		t.Fatalf("got %d schedules, want 1", len(scheds))
	}
	if want := types.MustNewMoney("-617.16"); !scheds[0].Amount.Money.Equal(want) {
		t.Errorf("parent amount = %s, want %s", scheds[0].Amount.Money, want)
	}

	// Origination opening-date rule: balance == principal → open date recorded.
	loanAcct, _ := svc.Account.GetByName("Car Loan")
	if !loanAcct.OpeningDate.Equal(types.NewDate(2026, time.July, 1)) {
		t.Errorf("opening date = %s, want 2026-07-01 (origination)", loanAcct.OpeningDate)
	}
}

func TestLoanAdd_ZeroRateOmitsInterestLine(t *testing.T) {
	dbPath := setupLoanDB(t)
	// 0% loan, principal 1200 over 12 months → 100/mo, no interest line.
	out, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "0% Promo", "--principal", "1200", "--rate", "0",
		"--term-months", "12", "--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err != nil {
		t.Fatalf("loan add: %v (out: %s)", err, out)
	}

	svc := clitest.OpenSvc(t, dbPath)
	checking, _ := svc.Account.GetByName("Checking")
	scheds, _ := svc.Scheduled.ListByAccount(checking.ID)
	if len(scheds) != 1 {
		t.Fatalf("got %d schedules, want 1", len(scheds))
	}
	for _, sp := range scheds[0].Splits {
		if sp.LoanSection.Valid && sp.LoanSection.String == scheduled.LoanSectionInterest {
			t.Error("0% loan should have no interest line")
		}
	}
	// A 0% loan books no interest, so no Loan:Interest category is created — but
	// the principal line is still labeled Loan:Principal by default, so the Loan
	// parent + Principal child do exist (spec: "0% loans, principal still
	// labeled").
	loanParent, err := svc.CategoryRepo.GetByName("Loan", nil)
	if err != nil {
		t.Fatalf("Loan parent should exist for the default Loan:Principal: %v", err)
	}
	if _, err := svc.CategoryRepo.GetByName("Interest", &loanParent.ID); err == nil {
		t.Error("0% loan should not create the Loan:Interest category")
	}
	if _, err := svc.CategoryRepo.GetByName("Principal", &loanParent.ID); err != nil {
		t.Errorf("0%% loan should label its principal line Loan:Principal: %v", err)
	}
}

func TestLoanAdd_NegativeAmortizationRejected(t *testing.T) {
	dbPath := setupLoanDB(t)
	// Payment 100 << first month's interest on 312450 @ 6.5%.
	out, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Bad", "--current-balance", "312450.22", "--rate", "6.5",
		"--payment", "100", "--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err == nil {
		t.Fatalf("expected negative-amortization error, got success: %s", out)
	}
	if !strings.Contains(err.Error(), "interest") {
		t.Errorf("expected interest-coverage error, got: %v", err)
	}
	// Nothing created.
	svc := clitest.OpenSvc(t, dbPath)
	if _, err := svc.Account.GetByName("Bad"); err == nil {
		t.Error("loan account should not exist after a rejected add")
	}
}

func TestLoanAdd_MissingBalance(t *testing.T) {
	dbPath := setupLoanDB(t)
	_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "M", "--rate", "6.5",
		"--payment", "100", "--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err == nil || !strings.Contains(err.Error(), "balance") {
		t.Errorf("expected missing-balance error, got: %v", err)
	}
}

func TestLoanAdd_MissingPaymentInputs(t *testing.T) {
	dbPath := setupLoanDB(t)
	// Balance given but neither --payment nor (--principal + --term-months).
	_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "M",
		"--current-balance", "10000", "--rate", "6.5",
		"--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err == nil || !strings.Contains(err.Error(), "payment") {
		t.Errorf("expected missing-payment error, got: %v", err)
	}
}

func TestLoanAdd_FundingAccountErrors(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		dbPath := setupLoanDB(t)
		_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "M",
			"--current-balance", "10000", "--rate", "6.5", "--payment", "200",
			"--next-payment-date", "2026-08-01", "--from-account", "Nope")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected not-found error, got: %v", err)
		}
	})

	t.Run("investment account rejected", func(t *testing.T) {
		database, dbPath := dbtest.NewFile(t, "loan.tdb")
		repo := account.NewRepository(database)
		brokerage := account.NewAccount("Brokerage", account.TypeInvestment, "USD",
			types.ZeroMoney, types.NewDate(2000, time.January, 1))
		if err := repo.Create(brokerage); err != nil {
			t.Fatalf("create brokerage: %v", err)
		}
		database.Close()
		_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "M",
			"--current-balance", "10000", "--rate", "6.5", "--payment", "200",
			"--next-payment-date", "2026-08-01", "--from-account", "Brokerage")
		if err == nil || !strings.Contains(err.Error(), "investment") {
			t.Errorf("expected investment-account error, got: %v", err)
		}
	})

	t.Run("closed account rejected", func(t *testing.T) {
		// Close requires a zero balance, so use a dedicated empty account.
		database, dbPath := dbtest.NewFile(t, "loan.tdb")
		repo := account.NewRepository(database)
		empty := account.NewAccount("Empty", account.TypeChecking, "USD",
			types.ZeroMoney, types.NewDate(2000, time.January, 1))
		if err := repo.Create(empty); err != nil {
			t.Fatalf("create empty: %v", err)
		}
		database.Close()

		svc := clitest.OpenSvc(t, dbPath)
		if err := svc.Account.Close(empty.ID, types.Today()); err != nil {
			t.Fatalf("close account: %v", err)
		}
		_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "M",
			"--current-balance", "10000", "--rate", "6.5", "--payment", "200",
			"--next-payment-date", "2026-08-01", "--from-account", "Empty")
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Errorf("expected closed-account error, got: %v", err)
		}
	})
}

func TestLoanAdd_DuplicateNameRejected(t *testing.T) {
	dbPath := setupLoanDB(t)
	// "Checking" already exists.
	_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "Checking",
		"--current-balance", "10000", "--rate", "6.5", "--payment", "200",
		"--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected already-exists error, got: %v", err)
	}
}

func TestLoanAdd_MidLifeOpeningDateIsToday(t *testing.T) {
	dbPath := setupLoanDB(t)
	// Mid-life: current balance != original principal, so --open-date is ignored.
	_, err := runLoan(t, "loan", "add", "--file", dbPath, "--name", "Mortgage",
		"--current-balance", "300000", "--principal", "380000", "--rate", "6.5",
		"--payment", "2401.86", "--open-date", "2020-01-01",
		"--next-payment-date", "2026-08-01", "--from-account", "Checking")
	if err != nil {
		t.Fatalf("loan add: %v", err)
	}
	svc := clitest.OpenSvc(t, dbPath)
	loanAcct, _ := svc.Account.GetByName("Mortgage")
	if loanAcct.OpeningDate.Equal(types.NewDate(2020, time.January, 1)) {
		t.Error("mid-life loan should not record the origination open date")
	}
	if !loanAcct.OpeningDate.Equal(types.Today()) {
		t.Errorf("mid-life opening date = %s, want today", loanAcct.OpeningDate)
	}
}

// TestLoanAdd_PostRecomputesSplits drives the full CLI path: `loan add` creates
// the schedule, then `scheduled post` computes the interest/principal split from
// the live balance and the loan balance moves toward zero by the principal.
func TestLoanAdd_PostRecomputesSplits(t *testing.T) {
	dbPath := setupLoanDB(t)
	if _, err := runLoan(t, "loan", "add", "--file", dbPath,
		"--name", "Car Loan", "--current-balance", "32000", "--rate", "5.9",
		"--payment", "617.16", "--next-payment-date", "2026-08-01",
		"--from-account", "Checking"); err != nil {
		t.Fatalf("loan add: %v", err)
	}

	// Locate the schedule's full ID and post it through the CLI.
	svc := clitest.OpenSvc(t, dbPath)
	checking, _ := svc.Account.GetByName("Checking")
	scheds, _ := svc.Scheduled.ListByAccount(checking.ID)
	if len(scheds) != 1 {
		t.Fatalf("got %d schedules, want 1", len(scheds))
	}
	schedID := scheds[0].ID.String()

	if _, err := runLoan(t, "scheduled", "post", "--file", dbPath, schedID); err != nil {
		t.Fatalf("scheduled post: %v", err)
	}

	// Reopen: loan balance moved by the computed month-1 principal 459.83
	// (interest = round(32000 * 5.9 / 1200) = 157.33; principal = 617.16 - 157.33).
	svc2 := clitest.OpenSvc(t, dbPath)
	loanAcct, _ := svc2.Account.GetByName("Car Loan")
	bal, err := svc2.Account.GetBalance(loanAcct.ID)
	if err != nil {
		t.Fatalf("get balance: %v", err)
	}
	if want := types.MustNewMoney("-31540.17"); !bal.CurrentBalance.Equal(want) {
		t.Errorf("loan balance after post = %s, want %s", bal.CurrentBalance, want)
	}
}
