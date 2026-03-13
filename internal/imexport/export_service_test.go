package imexport

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/models"
)

// --- Mock implementations for export ---

type mockAccountProvider struct {
	accounts []*models.Account
}

func (m *mockAccountProvider) List(activeOnly bool) ([]*models.Account, error) {
	if activeOnly {
		var result []*models.Account
		for _, a := range m.accounts {
			if a.Active {
				result = append(result, a)
			}
		}
		return result, nil
	}
	return m.accounts, nil
}

func (m *mockAccountProvider) GetByID(id models.ID) (*models.Account, error) {
	for _, a := range m.accounts {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, fmt.Errorf("account not found: %s", id.String())
}

type mockTransactionProvider struct {
	transactions map[string][]*models.Transaction // accountID -> transactions
}

func (m *mockTransactionProvider) ListByAccount(accountID models.ID) ([]*models.Transaction, error) {
	return m.transactions[accountID.String()], nil
}

func (m *mockTransactionProvider) ListByAccountAndDateRange(accountID models.ID, startDate, endDate models.Date) ([]*models.Transaction, error) {
	var result []*models.Transaction
	for _, txn := range m.transactions[accountID.String()] {
		if !txn.Date.Before(startDate) && !txn.Date.After(endDate) {
			result = append(result, txn)
		}
	}
	return result, nil
}

type mockSplitProvider struct {
	splits map[string][]*models.Split // txnID -> splits
}

func (m *mockSplitProvider) ListByTransaction(transactionID models.ID) ([]*models.Split, error) {
	return m.splits[transactionID.String()], nil
}

type mockPayeeProvider struct {
	payees map[string]*models.Payee // payeeID -> payee
}

func (m *mockPayeeProvider) GetByID(id models.ID) (*models.Payee, error) {
	if p, ok := m.payees[id.String()]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("payee not found: %s", id.String())
}

type mockCategoryProvider struct {
	categories map[string]*models.Category // catID -> category
}

func (m *mockCategoryProvider) GetByID(id models.ID) (*models.Category, error) {
	if c, ok := m.categories[id.String()]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("category not found: %s", id.String())
}

func (m *mockCategoryProvider) GetWithParent(id models.ID) (*models.Category, *models.Category, error) {
	cat, ok := m.categories[id.String()]
	if !ok {
		return nil, nil, fmt.Errorf("category not found: %s", id.String())
	}
	if cat.ParentID.Valid {
		parent, ok := m.categories[cat.ParentID.ID.String()]
		if ok {
			return cat, parent, nil
		}
	}
	return cat, nil, nil
}

// --- Helper functions ---

func makeAccount(name string, accountType models.AccountType) *models.Account {
	return models.NewAccount(name, accountType, "USD", models.ZeroMoney, makeDate("2024-01-01"))
}

func makeExportService(
	accounts []*models.Account,
	transactions map[string][]*models.Transaction,
	splits map[string][]*models.Split,
	payees map[string]*models.Payee,
	categories map[string]*models.Category,
) *ExportService {
	if transactions == nil {
		transactions = make(map[string][]*models.Transaction)
	}
	if splits == nil {
		splits = make(map[string][]*models.Split)
	}
	if payees == nil {
		payees = make(map[string]*models.Payee)
	}
	if categories == nil {
		categories = make(map[string]*models.Category)
	}

	return NewExportService(
		&mockAccountProvider{accounts: accounts},
		&mockTransactionProvider{transactions: transactions},
		&mockSplitProvider{splits: splits},
		&mockPayeeProvider{payees: payees},
		&mockCategoryProvider{categories: categories},
	)
}

// --- Tests ---

func TestExportService_Export_CSV_BasicTransactions(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)
	payeeID := models.NewID()
	catID := models.NewID()

	payee := &models.Payee{Name: "Coffee Shop"}
	payee.ID = payeeID

	cat := models.NewCategory("Food", models.CategoryTypeExpense)
	cat.ID = catID

	txn := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-50.00"))
	txn.SetPayee(payeeID)
	txn.SetCategory(catID)
	txn.SetMemo("Morning coffee")
	txn.Clear()

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn},
	}
	payees := map[string]*models.Payee{payeeID.String(): payee}
	categories := map[string]*models.Category{catID.String(): cat}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, payees, categories)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}
	if result.AccountCount != 1 {
		t.Errorf("AccountCount = %d, want 1", result.AccountCount)
	}

	// Parse CSV output
	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV output: %v", err)
	}

	// Header + 1 data row
	if len(records) != 2 {
		t.Fatalf("CSV rows = %d, want 2", len(records))
	}

	// Verify header
	if records[0][0] != "Date" {
		t.Errorf("header[0] = %q, want %q", records[0][0], "Date")
	}

	// Verify data row
	row := records[1]
	if row[0] != "2024-01-15" {
		t.Errorf("date = %q, want %q", row[0], "2024-01-15")
	}
	if row[1] != "Checking" {
		t.Errorf("account = %q, want %q", row[1], "Checking")
	}
	if row[2] != "Coffee Shop" {
		t.Errorf("payee = %q, want %q", row[2], "Coffee Shop")
	}
	if row[3] != "Food" {
		t.Errorf("category = %q, want %q", row[3], "Food")
	}
	if row[4] != "-50.00" {
		t.Errorf("amount = %q, want %q", row[4], "-50.00")
	}
	if row[5] != "Morning coffee" {
		t.Errorf("memo = %q, want %q", row[5], "Morning coffee")
	}
	if row[7] != "C" {
		t.Errorf("status = %q, want %q", row[7], "C")
	}
}

func TestExportService_Export_CSV_SubcategoryPath(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)
	parentCatID := models.NewID()
	childCatID := models.NewID()

	parentCat := models.NewCategory("Food", models.CategoryTypeExpense)
	parentCat.ID = parentCatID

	childCat := models.NewSubcategory("Groceries", parentCatID, models.CategoryTypeExpense)
	childCat.ID = childCatID

	txn := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-120.00"))
	txn.SetCategory(childCatID)

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn},
	}
	categories := map[string]*models.Category{
		parentCatID.String(): parentCat,
		childCatID.String():  childCat,
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, categories)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV: %v", err)
	}

	if records[1][3] != "Food:Groceries" {
		t.Errorf("category = %q, want %q", records[1][3], "Food:Groceries")
	}
}

func TestExportService_Export_CSV_SplitTransaction(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)
	catID1 := models.NewID()
	catID2 := models.NewID()

	cat1 := models.NewCategory("Food", models.CategoryTypeExpense)
	cat1.ID = catID1
	cat2 := models.NewCategory("Transport", models.CategoryTypeExpense)
	cat2.ID = catID2

	txn := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-150.00"))

	split1 := models.NewSplitWithMemo(txn.ID, catID1, makeMoney("-100.00"), "groceries")
	split2 := models.NewSplit(txn.ID, catID2, makeMoney("-50.00"))

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn},
	}
	splits := map[string][]*models.Split{
		txn.ID.String(): {split1, split2},
	}
	categories := map[string]*models.Category{
		catID1.String(): cat1,
		catID2.String(): cat2,
	}

	svc := makeExportService([]*models.Account{acct}, transactions, splits, nil, categories)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV: %v", err)
	}

	// Header + parent row + 2 split rows = 4
	if len(records) != 4 {
		t.Fatalf("CSV rows = %d, want 4", len(records))
	}

	// Parent row should have empty category
	if records[1][3] != "" {
		t.Errorf("parent category = %q, want empty", records[1][3])
	}

	// Split rows
	if records[2][3] != "Food" {
		t.Errorf("split1 category = %q, want %q", records[2][3], "Food")
	}
	if records[2][4] != "-100.00" {
		t.Errorf("split1 amount = %q, want %q", records[2][4], "-100.00")
	}
	if records[3][3] != "Transport" {
		t.Errorf("split2 category = %q, want %q", records[3][3], "Transport")
	}
}

func TestExportService_Export_CSV_Transfer(t *testing.T) {
	checking := makeAccount("Checking", models.AccountTypeChecking)
	savings := makeAccount("Savings", models.AccountTypeSavings)

	transferID := models.NewID()
	txn := models.NewTransaction(checking.ID, makeDate("2024-01-15"), makeMoney("-500.00"))
	txn.SetTransfer(transferID, savings.ID)

	transactions := map[string][]*models.Transaction{
		checking.ID.String(): {txn},
	}

	svc := makeExportService([]*models.Account{checking, savings}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{
		Format:    FormatCSV,
		AccountID: &checking.ID,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV: %v", err)
	}

	// Transfer Account column (index 8)
	if records[1][8] != "Savings" {
		t.Errorf("transfer account = %q, want %q", records[1][8], "Savings")
	}
}

func TestExportService_Export_CSV_AccountFilter(t *testing.T) {
	checking := makeAccount("Checking", models.AccountTypeChecking)
	savings := makeAccount("Savings", models.AccountTypeSavings)

	txn1 := models.NewTransaction(checking.ID, makeDate("2024-01-15"), makeMoney("-50.00"))
	txn2 := models.NewTransaction(savings.ID, makeDate("2024-01-16"), makeMoney("100.00"))

	transactions := map[string][]*models.Transaction{
		checking.ID.String(): {txn1},
		savings.ID.String():  {txn2},
	}

	svc := makeExportService([]*models.Account{checking, savings}, transactions, nil, nil, nil)

	// Export only checking
	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{
		Format:    FormatCSV,
		AccountID: &checking.ID,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}
	if result.AccountCount != 1 {
		t.Errorf("AccountCount = %d, want 1", result.AccountCount)
	}
}

func TestExportService_Export_CSV_DateRangeFilter(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)

	txn1 := models.NewTransaction(acct.ID, makeDate("2024-01-10"), makeMoney("-10.00"))
	txn2 := models.NewTransaction(acct.ID, makeDate("2024-01-20"), makeMoney("-20.00"))
	txn3 := models.NewTransaction(acct.ID, makeDate("2024-02-15"), makeMoney("-30.00"))

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn1, txn2, txn3},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	start := makeDate("2024-01-01")
	end := makeDate("2024-01-31")
	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{
		Format:    FormatCSV,
		AccountID: &acct.ID,
		StartDate: &start,
		EndDate:   &end,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 2 {
		t.Errorf("TransactionCount = %d, want 2", result.TransactionCount)
	}
}

func TestExportService_Export_CSV_MultipleAccounts(t *testing.T) {
	checking := makeAccount("Checking", models.AccountTypeChecking)
	savings := makeAccount("Savings", models.AccountTypeSavings)

	txn1 := models.NewTransaction(checking.ID, makeDate("2024-01-15"), makeMoney("-50.00"))
	txn2 := models.NewTransaction(savings.ID, makeDate("2024-01-16"), makeMoney("100.00"))

	transactions := map[string][]*models.Transaction{
		checking.ID.String(): {txn1},
		savings.ID.String():  {txn2},
	}

	svc := makeExportService([]*models.Account{checking, savings}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 2 {
		t.Errorf("TransactionCount = %d, want 2", result.TransactionCount)
	}
	if result.AccountCount != 2 {
		t.Errorf("AccountCount = %d, want 2", result.AccountCount)
	}
}

func TestExportService_Export_SkipsVoidTransactions(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)

	txn1 := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-50.00"))
	txn2 := models.NewTransaction(acct.ID, makeDate("2024-01-16"), makeMoney("-25.00"))
	txn2.Void()

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn1, txn2},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1 (void should be skipped)", result.TransactionCount)
	}
}

func TestExportService_Export_QIF(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)
	payeeID := models.NewID()

	payee := &models.Payee{Name: "Coffee Shop"}
	payee.ID = payeeID

	txn := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-50.00"))
	txn.SetPayee(payeeID)
	txn.Clear()

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn},
	}
	payees := map[string]*models.Payee{payeeID.String(): payee}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, payees, nil)

	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{
		Format:    FormatQIF,
		AccountID: &acct.ID,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}

	output := buf.String()
	if !strings.Contains(output, "!Type:Bank") {
		t.Error("QIF output missing type header")
	}
	if !strings.Contains(output, "D01/15/2024") {
		t.Errorf("QIF output missing date, got:\n%s", output)
	}
	if !strings.Contains(output, "T-50.00") {
		t.Errorf("QIF output missing amount, got:\n%s", output)
	}
	if !strings.Contains(output, "PCoffee Shop") {
		t.Errorf("QIF output missing payee, got:\n%s", output)
	}
	if !strings.Contains(output, "CX") {
		t.Errorf("QIF output missing cleared status, got:\n%s", output)
	}
	if !strings.Contains(output, "^") {
		t.Error("QIF output missing record separator")
	}
}

func TestExportService_Export_QIF_CreditCardType(t *testing.T) {
	acct := makeAccount("Visa", models.AccountTypeCreditCard)

	txn := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-50.00"))

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{
		Format:    FormatQIF,
		AccountID: &acct.ID,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if !strings.Contains(buf.String(), "!Type:CCard") {
		t.Errorf("QIF output should have CCard type, got:\n%s", buf.String())
	}
}

func TestExportService_Export_QIF_TransferCategory(t *testing.T) {
	checking := makeAccount("Checking", models.AccountTypeChecking)
	savings := makeAccount("Savings", models.AccountTypeSavings)

	transferID := models.NewID()
	txn := models.NewTransaction(checking.ID, makeDate("2024-01-15"), makeMoney("-500.00"))
	txn.SetTransfer(transferID, savings.ID)

	transactions := map[string][]*models.Transaction{
		checking.ID.String(): {txn},
	}

	svc := makeExportService([]*models.Account{checking, savings}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{
		Format:    FormatQIF,
		AccountID: &checking.ID,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	// QIF transfers use [Account Name] syntax in L field
	if !strings.Contains(buf.String(), "L[Savings]") {
		t.Errorf("QIF output should contain transfer category [Savings], got:\n%s", buf.String())
	}
}

func TestExportService_Export_UnsupportedFormat(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)
	svc := makeExportService([]*models.Account{acct}, nil, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{Format: FormatOFX})
	if err == nil {
		t.Fatal("Export() expected error for OFX format")
	}
}

func TestExportService_Export_NoTransactions(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)
	svc := makeExportService([]*models.Account{acct}, nil, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err == nil {
		t.Fatal("Export() expected error when no transactions")
	}
}

func TestExportService_Export_NoAccounts(t *testing.T) {
	svc := makeExportService(nil, nil, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err == nil {
		t.Fatal("Export() expected error when no accounts")
	}
}

func TestExportService_Export_CheckNumber(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)

	txn := models.NewTransaction(acct.ID, makeDate("2024-01-15"), makeMoney("-200.00"))
	txn.SetCheckNumber("1234")

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV: %v", err)
	}

	// Check Number is column index 6
	if records[1][6] != "1234" {
		t.Errorf("check number = %q, want %q", records[1][6], "1234")
	}
}

func TestExportService_Export_StartDateOnly(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)

	txn1 := models.NewTransaction(acct.ID, makeDate("2024-01-10"), makeMoney("-10.00"))
	txn2 := models.NewTransaction(acct.ID, makeDate("2024-02-15"), makeMoney("-20.00"))

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn1, txn2},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	start := makeDate("2024-02-01")
	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{
		Format:    FormatCSV,
		StartDate: &start,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}
}

func TestExportService_Export_EndDateOnly(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)

	txn1 := models.NewTransaction(acct.ID, makeDate("2024-01-10"), makeMoney("-10.00"))
	txn2 := models.NewTransaction(acct.ID, makeDate("2024-02-15"), makeMoney("-20.00"))

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txn1, txn2},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	end := makeDate("2024-01-31")
	var buf bytes.Buffer
	result, err := svc.Export(&buf, ExportOptions{
		Format:  FormatCSV,
		EndDate: &end,
	})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", result.TransactionCount)
	}
}

func TestExportService_Export_CSV_AllStatuses(t *testing.T) {
	acct := makeAccount("Checking", models.AccountTypeChecking)

	txnU := models.NewTransaction(acct.ID, makeDate("2024-01-01"), makeMoney("-10.00"))
	txnC := models.NewTransaction(acct.ID, makeDate("2024-01-02"), makeMoney("-20.00"))
	txnC.Clear()
	txnR := models.NewTransaction(acct.ID, makeDate("2024-01-03"), makeMoney("-30.00"))
	txnR.Reconcile()

	transactions := map[string][]*models.Transaction{
		acct.ID.String(): {txnU, txnC, txnR},
	}

	svc := makeExportService([]*models.Account{acct}, transactions, nil, nil, nil)

	var buf bytes.Buffer
	_, err := svc.Export(&buf, ExportOptions{Format: FormatCSV})
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("reading CSV: %v", err)
	}

	// Status column (index 7)
	if records[1][7] != "U" {
		t.Errorf("row 1 status = %q, want %q", records[1][7], "U")
	}
	if records[2][7] != "C" {
		t.Errorf("row 2 status = %q, want %q", records[2][7], "C")
	}
	if records[3][7] != "R" {
		t.Errorf("row 3 status = %q, want %q", records[3][7], "R")
	}
}
