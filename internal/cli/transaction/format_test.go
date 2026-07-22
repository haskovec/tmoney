package transaction

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/account"
	transactiondom "github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

func transferAccounts(t *testing.T) (*account.Account, *account.Account) {
	t.Helper()
	checking := account.NewAccount("Checking", account.TypeChecking, "USD", types.MustNewMoney("1000.00"), types.NewDate(2000, time.January, 1))
	savings := account.NewAccount("Savings", account.TypeSavings, "USD", types.ZeroMoney, types.NewDate(2000, time.January, 1))
	return checking, savings
}

// transferLine finds the printed data row for the transfer (the one carrying
// the [Transfer] payee marker).
func transferLine(t *testing.T, out string) string {
	t.Helper()
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "[Transfer]") {
			return line
		}
	}
	t.Fatalf("no [Transfer] row in output:\n%s", out)
	return ""
}

func TestPrintTransactionsTable_CategorizedTransferShowsCategory(t *testing.T) {
	checking, savings := transferAccounts(t)
	catID := types.NewID()

	txn := transactiondom.NewTransaction(checking.ID, types.NewDate(2024, time.January, 15), types.MustNewMoney("-100.00"))
	txn.SetTransfer(types.NewID(), savings.ID)
	txn.SetCategory(catID)

	var buf bytes.Buffer
	printTransactionsTable(&buf, checking,
		[]*transactiondom.Transaction{txn},
		map[types.ID]string{},
		map[types.ID]string{catID: "Credit Card"},
		nil,
		false,
	)

	line := transferLine(t, buf.String())
	if !strings.Contains(line, "Credit Card") {
		t.Errorf("categorized transfer row should show its category, got: %q", line)
	}
}

func TestPrintTransactionsTable_UncategorizedTransferHidesCategory(t *testing.T) {
	checking, savings := transferAccounts(t)
	catID := types.NewID()

	txn := transactiondom.NewTransaction(checking.ID, types.NewDate(2024, time.January, 15), types.MustNewMoney("-100.00"))
	txn.SetTransfer(types.NewID(), savings.ID)

	var buf bytes.Buffer
	printTransactionsTable(&buf, checking,
		[]*transactiondom.Transaction{txn},
		map[types.ID]string{},
		map[types.ID]string{catID: "Credit Card"},
		nil,
		false,
	)

	line := transferLine(t, buf.String())
	if strings.Contains(line, "Credit Card") {
		t.Errorf("uncategorized transfer must not show a category, got: %q", line)
	}
}

func TestPrintSearchResults_CategorizedTransferShowsCategory(t *testing.T) {
	checking, savings := transferAccounts(t)
	catID := types.NewID()

	txn := transactiondom.NewTransaction(checking.ID, types.NewDate(2024, time.January, 15), types.MustNewMoney("-100.00"))
	txn.SetTransfer(types.NewID(), savings.ID)
	txn.SetCategory(catID)

	var buf bytes.Buffer
	printSearchResults(&buf, "x",
		[]*transactiondom.Transaction{txn},
		map[types.ID]string{checking.ID: "Checking"},
		map[types.ID]string{checking.ID: "USD"},
		map[types.ID]string{},
		map[types.ID]string{catID: "Credit Card"},
		nil,
		false,
	)

	line := transferLine(t, buf.String())
	if !strings.Contains(line, "Credit Card") {
		t.Errorf("categorized transfer search row should show its category, got: %q", line)
	}
}

func TestPrintTransactionsTable_SplitParentShowsMarker(t *testing.T) {
	checking, _ := transferAccounts(t)

	txn := transactiondom.NewTransaction(checking.ID, types.NewDate(2024, time.January, 15), types.MustNewMoney("-100.00"))

	var buf bytes.Buffer
	printTransactionsTable(&buf, checking,
		[]*transactiondom.Transaction{txn},
		map[types.ID]string{},
		map[types.ID]string{},
		map[types.ID]int{txn.ID: 3},
		false,
	)

	if !strings.Contains(buf.String(), "[3 splits]") {
		t.Errorf("split parent row should show the split marker, got:\n%s", buf.String())
	}
}

func TestPrintSearchResults_SplitParentShowsMarker(t *testing.T) {
	checking, _ := transferAccounts(t)

	txn := transactiondom.NewTransaction(checking.ID, types.NewDate(2024, time.January, 15), types.MustNewMoney("-100.00"))

	var buf bytes.Buffer
	printSearchResults(&buf, "x",
		[]*transactiondom.Transaction{txn},
		map[types.ID]string{checking.ID: "Checking"},
		map[types.ID]string{checking.ID: "USD"},
		map[types.ID]string{},
		map[types.ID]string{},
		map[types.ID]int{txn.ID: 2},
		false,
	)

	if !strings.Contains(buf.String(), "[2 splits]") {
		t.Errorf("split parent search row should show the split marker, got:\n%s", buf.String())
	}
}
