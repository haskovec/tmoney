package transaction_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli"
	"github.com/haskovec/tmoney/internal/dbtest"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// TestTransactionSearch_ByCategory_MatchesCategorizedTransfer confirms that a
// categorized transfer is matched by the search --category filter and that its
// category label is shown in the output (Phase 6: transfers may carry a
// category, and search joins category_id).
func TestTransactionSearch_ByCategory_MatchesCategorizedTransfer(t *testing.T) {
	database, dbPath := dbtest.NewFile(t, "test.tdb")

	acctRepo := account.NewRepository(database)
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.Today())
	if err := acctRepo.Create(checking); err != nil {
		t.Fatalf("create checking: %v", err)
	}
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.MustNewMoney("500.00"), types.Today())
	if err := acctRepo.Create(savings); err != nil {
		t.Fatalf("create savings: %v", err)
	}

	catRepo := category.NewRepository(database)
	bills := category.NewCategory("Bills", category.TypeExpense)
	if err := catRepo.Create(bills); err != nil {
		t.Fatalf("create category: %v", err)
	}

	// Hand-build a categorized transfer pair (both legs carry the category and
	// a searchable memo), mirroring what `transfer add --category` produces.
	txnRepo := transactiondom.NewRepository(database)
	transferID := types.NewID()
	from := transactiondom.NewTransaction(checking.ID, types.Today(), types.MustNewMoney("-500.00"))
	from.SetTransfer(transferID, savings.ID)
	from.SetCategory(bills.ID)
	from.SetMemo("rent transfer")
	if err := txnRepo.Create(from); err != nil {
		t.Fatalf("create from leg: %v", err)
	}
	to := transactiondom.NewTransaction(savings.ID, types.Today(), types.MustNewMoney("500.00"))
	to.SetTransfer(transferID, checking.ID)
	to.SetCategory(bills.ID)
	to.SetMemo("rent transfer")
	if err := txnRepo.Create(to); err != nil {
		t.Fatalf("create to leg: %v", err)
	}

	database.Close()

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if err := cli.ExecuteWith([]string{
		"transaction", "search", "rent", "--file", dbPath, "--category", "Bills",
	}, stdout, stderr); err != nil {
		t.Fatalf("transaction search --category: %v\nstderr=%s", err, stderr)
	}

	output := stdout.String()
	if !strings.Contains(output, "[Transfer]") {
		t.Errorf("expected the categorized transfer in results, got:\n%s", output)
	}
	if !strings.Contains(output, "Bills") {
		t.Errorf("expected the category label shown for the transfer, got:\n%s", output)
	}
	if strings.Contains(output, "Found 0") {
		t.Errorf("category filter dropped the categorized transfer, got:\n%s", output)
	}
}
