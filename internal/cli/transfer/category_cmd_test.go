package transfer_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// seedTransferCategory creates a top-level expense category on the file and
// returns its ID, using a scoped services handle so the seeding write is
// committed before the command reopens the file.
func seedTransferCategory(t *testing.T, dbPath, name string) types.ID {
	t.Helper()
	var id types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		cat := category.NewCategory(name, category.TypeExpense)
		if err := svc.Category.Create(cat); err != nil {
			t.Fatalf("seed category %q: %v", name, err)
		}
		id = cat.ID
	}()
	return id
}

func TestTransferAdd_WithCategory_MirrorsToBothLegs(t *testing.T) {
	dbPath, checking, savings := clitest.SetupTransferAccounts(t)
	billsID := seedTransferCategory(t, dbPath, "Bills")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Checking", "--to", "Savings", "--amount", "75.00",
		"--category", "Bills",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer add --category: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout.String(), "Category:") || !strings.Contains(stdout.String(), "Bills") {
		t.Errorf("expected Category line in output, got:\n%s", stdout.String())
	}

	svc := clitest.OpenSvc(t, dbPath)
	fromLegs, _ := svc.TransactionRepo.ListByAccount(checking.ID)
	toLegs, _ := svc.TransactionRepo.ListByAccount(savings.ID)
	if len(fromLegs) != 1 || len(toLegs) != 1 {
		t.Fatalf("expected one leg per account, got from=%d to=%d", len(fromLegs), len(toLegs))
	}
	for _, leg := range []*transaction.Transaction{fromLegs[0], toLegs[0]} {
		if !leg.CategoryID.Valid || leg.CategoryID.ID != billsID {
			t.Errorf("leg %s category = %+v, want %s", leg.ID, leg.CategoryID, billsID)
		}
	}
}

func TestTransferAdd_WithSubcategoryPath(t *testing.T) {
	dbPath, checking, _ := clitest.SetupTransferAccounts(t)

	var childID types.ID
	func() {
		svc := clitest.OpenSvc(t, dbPath)
		parent := category.NewCategory("Bills", category.TypeExpense)
		if err := svc.Category.Create(parent); err != nil {
			t.Fatalf("seed parent: %v", err)
		}
		child := category.NewSubcategory("Credit Card", parent.ID, category.TypeExpense)
		if err := svc.Category.Create(child); err != nil {
			t.Fatalf("seed child: %v", err)
		}
		childID = child.ID
	}()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Checking", "--to", "Savings", "--amount", "50.00",
		"--category", "Bills:Credit Card",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer add subcategory: %v\nstderr=%s", err, stderr)
	}

	svc := clitest.OpenSvc(t, dbPath)
	legs, _ := svc.TransactionRepo.ListByAccount(checking.ID)
	if len(legs) != 1 {
		t.Fatalf("expected one leg, got %d", len(legs))
	}
	if !legs[0].CategoryID.Valid || legs[0].CategoryID.ID != childID {
		t.Errorf("leg category = %+v, want subcategory %s", legs[0].CategoryID, childID)
	}
}

func TestTransferAdd_UnknownCategory_Errors(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Checking", "--to", "Savings", "--amount", "50.00",
		"--category", "Nonexistent",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestTransferAdd_SystemCategory_Rejected(t *testing.T) {
	dbPath, _, _ := clitest.SetupTransferAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Checking", "--to", "Savings", "--amount", "50.00",
		"--category", category.ValueAdjustmentCategoryName,
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error for system category")
	}
	if !strings.Contains(err.Error(), "system category") {
		t.Errorf("error = %q, want 'system category'", err.Error())
	}
}

func TestTransferAdd_InvToInv_CategoryRejected(t *testing.T) {
	dbPath, _, _, _, _ := clitest.SetupTransferDispatchAccounts(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Brokerage", "--to", "Rollover IRA", "--amount", "100.00",
		"--category", "Bills",
	}, stdout, stderr)
	if err == nil {
		t.Fatal("expected error: --category unsupported for inv→inv")
	}
	if !strings.Contains(err.Error(), "investment-to-investment") {
		t.Errorf("error = %q, want mention of investment-to-investment", err.Error())
	}
}

func TestTransferAdd_RegToInv_CategoryOnBankLeg(t *testing.T) {
	dbPath, checking, _, _, _ := clitest.SetupTransferDispatchAccounts(t)
	billsID := seedTransferCategory(t, dbPath, "Bills")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Checking", "--to", "Brokerage", "--amount", "500.00",
		"--category", "Bills",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer add reg→inv --category: %v\nstderr=%s", err, stderr)
	}

	svc := clitest.OpenSvc(t, dbPath)
	legs, _ := svc.TransactionRepo.ListByAccount(checking.ID)
	if len(legs) != 1 {
		t.Fatalf("expected one regular leg on Checking, got %d", len(legs))
	}
	if !legs[0].CategoryID.Valid || legs[0].CategoryID.ID != billsID {
		t.Errorf("bank-side leg category = %+v, want %s", legs[0].CategoryID, billsID)
	}
}

func TestTransferAdd_InvToReg_CategoryOnBankLeg(t *testing.T) {
	dbPath, checking, _, _, _ := clitest.SetupTransferDispatchAccounts(t)
	billsID := seedTransferCategory(t, dbPath, "Bills")

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transfer", "add", "--file", dbPath,
		"--from", "Brokerage", "--to", "Checking", "--amount", "300.00",
		"--category", "Bills",
	}, stdout, stderr); err != nil {
		t.Fatalf("transfer add inv→reg --category: %v\nstderr=%s", err, stderr)
	}

	svc := clitest.OpenSvc(t, dbPath)
	legs, _ := svc.TransactionRepo.ListByAccount(checking.ID)
	if len(legs) != 1 {
		t.Fatalf("expected one regular leg on Checking, got %d", len(legs))
	}
	if !legs[0].CategoryID.Valid || legs[0].CategoryID.ID != billsID {
		t.Errorf("bank-side leg category = %+v, want %s", legs[0].CategoryID, billsID)
	}
}
