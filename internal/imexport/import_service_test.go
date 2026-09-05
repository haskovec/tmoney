package imexport

import (
	"strings"
	"testing"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// --- Mock implementations ---

type mockCategoryResolver struct {
	categories map[string]types.ID
}

func (m *mockCategoryResolver) ResolveCategoryByName(name string) (types.ID, error) {
	if id, ok := m.categories[name]; ok {
		return id, nil
	}
	return types.ID{}, nil
}

type mockPayeeResolver struct {
	payees map[string]struct {
		id         types.ID
		defaultCat types.NullableID
	}
}

func (m *mockPayeeResolver) ResolvePayee(name string) (types.ID, types.NullableID, error) {
	if p, ok := m.payees[name]; ok {
		return p.id, p.defaultCat, nil
	}
	return types.ID{}, types.NullableID{}, nil
}

type mockTransactionStore struct {
	transactions []*transaction.Transaction
	payeeNames   map[string]string // payeeID string -> name
	bankRefs     map[string]string // txnID string -> bank ref
}

func (m *mockTransactionStore) ListByAccount(_ types.ID) ([]*transaction.Transaction, error) {
	return m.transactions, nil
}

func (m *mockTransactionStore) GetPayeeName(payeeID types.ID) string {
	if name, ok := m.payeeNames[payeeID.String()]; ok {
		return name
	}
	return ""
}

func (m *mockTransactionStore) GetBankReferenceID(txn *transaction.Transaction) string {
	if ref, ok := m.bankRefs[txn.ID.String()]; ok {
		return ref
	}
	if txn.BankReferenceID.Valid {
		return txn.BankReferenceID.String
	}
	return ""
}

type mockTransactionCreator struct {
	created []*transaction.Transaction
	splits  map[string][]*transaction.Split // txnID -> splits
	updated []*transaction.Transaction
	failOn  int // fail on the Nth create (1-based, 0 = don't fail)
	createN int
}

func (m *mockTransactionCreator) CreateTransaction(txn *transaction.Transaction) error {
	m.createN++
	if m.failOn > 0 && m.createN == m.failOn {
		return &types.ValidationError{Field: "test", Message: "deliberate failure"}
	}
	m.created = append(m.created, txn)
	return nil
}

func (m *mockTransactionCreator) CreateTransactionWithSplits(txn *transaction.Transaction, splits []*transaction.Split) error {
	m.createN++
	if m.failOn > 0 && m.createN == m.failOn {
		return &types.ValidationError{Field: "test", Message: "deliberate failure"}
	}
	m.created = append(m.created, txn)
	if m.splits == nil {
		m.splits = make(map[string][]*transaction.Split)
	}
	m.splits[txn.ID.String()] = splits
	return nil
}

func (m *mockTransactionCreator) UpdateTransaction(txn *transaction.Transaction) error {
	m.updated = append(m.updated, txn)
	return nil
}

// --- Helper functions ---

func makeTestImportService(
	categories map[string]types.ID,
	payees map[string]struct {
		id         types.ID
		defaultCat types.NullableID
	},
	existingTxns []*transaction.Transaction,
	payeeNames map[string]string,
	bankRefs map[string]string,
) (*ImportService, *mockTransactionCreator) {
	catResolver := &mockCategoryResolver{categories: categories}
	payeeResolver := &mockPayeeResolver{payees: payees}
	store := &mockTransactionStore{
		transactions: existingTxns,
		payeeNames:   payeeNames,
		bankRefs:     bankRefs,
	}
	creator := &mockTransactionCreator{}

	svc := NewImportService(catResolver, payeeResolver, store, creator)
	return svc, creator
}

func makeMoney(s string) types.Money {
	return types.MustNewMoney(s)
}

func makeTxnWithPayee(accountID types.ID, date string, amount string, payeeID types.ID) *transaction.Transaction {
	txn := transaction.NewTransaction(accountID, makeDate(date), makeMoney(amount))
	if !payeeID.IsNil() {
		txn.SetPayee(payeeID)
	}
	return txn
}

// --- Tests ---

func TestImportService_Parse_CSV(t *testing.T) {
	svc, _ := makeTestImportService(nil, nil, nil, nil, nil)

	csv := "Date,Amount,Payee,Category\n2024-01-15,-50.00,Coffee Shop,Food\n"
	result, err := svc.Parse(strings.NewReader(csv), FormatCSV)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("Parse() got %d records, want 1", len(result.Records))
	}
	if result.Records[0].Payee != "Coffee Shop" {
		t.Errorf("record payee = %q, want %q", result.Records[0].Payee, "Coffee Shop")
	}
}

func TestImportService_Parse_QIF(t *testing.T) {
	svc, _ := makeTestImportService(nil, nil, nil, nil, nil)

	qif := "!Type:Bank\nD01/15/2024\nT-50.00\nPCoffee Shop\nLFood\n^\n"
	result, err := svc.Parse(strings.NewReader(qif), FormatQIF)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("Parse() got %d records, want 1", len(result.Records))
	}
}

func TestImportService_Parse_UnsupportedFormat(t *testing.T) {
	svc, _ := makeTestImportService(nil, nil, nil, nil, nil)
	_, err := svc.Parse(strings.NewReader(""), Format("xyz"))
	if err == nil {
		t.Fatal("Parse() expected error for unsupported format")
	}
}

func TestImportService_Preview_NewTransactions(t *testing.T) {
	accountID := types.NewID()
	svc, _ := makeTestImportService(nil, nil, nil, nil, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n2024-01-16,-25.00,Grocery Store\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingNone},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("Preview() got %d rows, want 2", len(result.Rows))
	}

	// All should be new since duplicate handling is none
	for i, row := range result.Rows {
		if row.Action != ImportActionNew {
			t.Errorf("row %d action = %q, want %q", i, row.Action, ImportActionNew)
		}
	}
}

func TestImportService_Preview_WithMatching(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()

	existingTxn := makeTxnWithPayee(accountID, "2024-01-15", "-50.00", payeeID)
	payeeNames := map[string]string{payeeID.String(): "Coffee Shop"}

	svc, _ := makeTestImportService(nil, nil, []*transaction.Transaction{existingTxn}, payeeNames, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n2024-01-20,-75.00,Restaurant\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingUpdate},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("Preview() got %d rows, want 2", len(result.Rows))
	}

	// First row should match the existing transaction (high confidence: same amount, date, payee)
	if result.Rows[0].Action != ImportActionMatch {
		t.Errorf("row 0 action = %q, want %q", result.Rows[0].Action, ImportActionMatch)
	}

	// Second row should be new (no match for -75.00)
	if result.Rows[1].Action != ImportActionNew {
		t.Errorf("row 1 action = %q, want %q", result.Rows[1].Action, ImportActionNew)
	}
}

func TestImportService_Preview_SkipDuplicates(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()

	existingTxn := makeTxnWithPayee(accountID, "2024-01-15", "-50.00", payeeID)
	payeeNames := map[string]string{payeeID.String(): "Coffee Shop"}

	svc, _ := makeTestImportService(nil, nil, []*transaction.Transaction{existingTxn}, payeeNames, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingSkip},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if result.Rows[0].Action != ImportActionSkip {
		t.Errorf("row 0 action = %q, want %q", result.Rows[0].Action, ImportActionSkip)
	}
}

func TestImportService_Preview_AutoCategorize(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()
	categoryID := types.NewID()

	categories := map[string]types.ID{"Food": categoryID}
	payees := map[string]struct {
		id         types.ID
		defaultCat types.NullableID
	}{
		"Coffee Shop": {
			id:         payeeID,
			defaultCat: types.NullableID{ID: categoryID, Valid: true},
		},
	}

	svc, _ := makeTestImportService(categories, payees, nil, nil, nil)

	// No explicit category - should get payee's default
	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingNone},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if !result.Rows[0].CategoryID.Valid {
		t.Error("expected auto-categorized category, got none")
	} else if result.Rows[0].CategoryID.ID != categoryID {
		t.Errorf("category = %s, want %s", result.Rows[0].CategoryID.ID.String(), categoryID.String())
	}

	if !result.Rows[0].PayeeID.Valid {
		t.Error("expected resolved payee, got none")
	} else if result.Rows[0].PayeeID.ID != payeeID {
		t.Errorf("payee = %s, want %s", result.Rows[0].PayeeID.ID.String(), payeeID.String())
	}
}

func TestImportService_Preview_ExplicitCategoryOverridesDefault(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()
	defaultCatID := types.NewID()
	explicitCatID := types.NewID()

	categories := map[string]types.ID{
		"Food":      defaultCatID,
		"Transport": explicitCatID,
	}
	payees := map[string]struct {
		id         types.ID
		defaultCat types.NullableID
	}{
		"Coffee Shop": {
			id:         payeeID,
			defaultCat: types.NullableID{ID: defaultCatID, Valid: true},
		},
	}

	svc, _ := makeTestImportService(categories, payees, nil, nil, nil)

	csv := "Date,Amount,Payee,Category\n2024-01-15,-50.00,Coffee Shop,Transport\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingNone},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	// Explicit category should override payee default
	if !result.Rows[0].CategoryID.Valid || result.Rows[0].CategoryID.ID != explicitCatID {
		t.Errorf("expected explicit category %s, got %v", explicitCatID.String(), result.Rows[0].CategoryID)
	}
}

func TestImportService_Preview_DateRange(t *testing.T) {
	accountID := types.NewID()
	svc, _ := makeTestImportService(nil, nil, nil, nil, nil)

	csv := "Date,Amount,Payee\n2024-01-10,-10.00,A\n2024-01-20,-20.00,B\n2024-01-05,-5.00,C\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingNone},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if result.DateFrom.String() != "2024-01-05" {
		t.Errorf("DateFrom = %s, want 2024-01-05", result.DateFrom.String())
	}
	if result.DateTo.String() != "2024-01-20" {
		t.Errorf("DateTo = %s, want 2024-01-20", result.DateTo.String())
	}
}

func TestImportService_Preview_EmptyFile(t *testing.T) {
	accountID := types.NewID()
	svc, _ := makeTestImportService(nil, nil, nil, nil, nil)

	csv := "Date,Amount,Payee\n"
	_, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingNone},
	)
	if err == nil {
		t.Fatal("Preview() expected error for empty file")
	}
}

func TestImportService_Preview_FITIDMatching(t *testing.T) {
	accountID := types.NewID()

	txn := transaction.NewTransaction(accountID, makeDate("2024-01-15"), makeMoney("-50.00"))
	txn.SetBankReferenceID("FITID123")

	records := []ImportRecord{
		{
			Date:            makeDate("2024-01-15"),
			Amount:          makeMoney("-50.00"),
			Payee:           "Coffee Shop",
			BankReferenceID: "FITID123",
			SourceLine:      1,
		},
	}

	existing := []ExistingTransaction{
		{
			Transaction:     txn,
			PayeeName:       "Coffee",
			BankReferenceID: "FITID123",
		},
	}

	matcher := NewDefaultMatcher()
	results := matcher.MatchAll(records, existing)

	if len(results) != 1 {
		t.Fatalf("MatchAll() got %d results, want 1", len(results))
	}
	if results[0].Confidence != MatchConfidenceHigh {
		t.Errorf("confidence = %q, want %q", results[0].Confidence, MatchConfidenceHigh)
	}
	if !results[0].MatchedByFITID {
		t.Error("expected MatchedByFITID = true")
	}
}

func TestImportService_Execute_CreateNew(t *testing.T) {
	accountID := types.NewID()
	svc, creator := makeTestImportService(nil, nil, nil, nil, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n2024-01-16,-25.00,Grocery Store\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingNone},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	err = svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Created != 2 {
		t.Errorf("Created = %d, want 2", result.Created)
	}
	if result.Updated != 0 {
		t.Errorf("Updated = %d, want 0", result.Updated)
	}
	if result.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", result.Skipped)
	}
	if len(creator.created) != 2 {
		t.Errorf("creator.created = %d, want 2", len(creator.created))
	}
}

func TestImportService_Execute_UpdateMatched(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()

	existingTxn := makeTxnWithPayee(accountID, "2024-01-15", "-50.00", payeeID)
	payeeNames := map[string]string{payeeID.String(): "Coffee Shop"}

	svc, creator := makeTestImportService(nil, nil, []*transaction.Transaction{existingTxn}, payeeNames, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingUpdate},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	err = svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Created != 0 {
		t.Errorf("Created = %d, want 0", result.Created)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1", result.Updated)
	}

	// The matched transaction should be updated to cleared
	if len(creator.updated) != 1 {
		t.Fatalf("creator.updated = %d, want 1", len(creator.updated))
	}
	if creator.updated[0].Status != transaction.StatusCleared {
		t.Errorf("updated status = %q, want %q", creator.updated[0].Status, transaction.StatusCleared)
	}
}

func TestImportService_Execute_SkipDuplicates(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()

	existingTxn := makeTxnWithPayee(accountID, "2024-01-15", "-50.00", payeeID)
	payeeNames := map[string]string{payeeID.String(): "Coffee Shop"}

	svc, creator := makeTestImportService(nil, nil, []*transaction.Transaction{existingTxn}, payeeNames, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingSkip},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	err = svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.Skipped)
	}
	if len(creator.created) != 0 {
		t.Errorf("creator.created = %d, want 0", len(creator.created))
	}
	if len(creator.updated) != 0 {
		t.Errorf("creator.updated = %d, want 0", len(creator.updated))
	}
}

func TestImportService_Execute_WithBankRefID(t *testing.T) {
	accountID := types.NewID()
	svc, creator := makeTestImportService(nil, nil, nil, nil, nil)

	// Simulate OFX import with FITID
	result := &ImportResult{
		Rows: []ImportRow{
			{
				Record: &ImportRecord{
					Date:            makeDate("2024-01-15"),
					Amount:          makeMoney("-50.00"),
					Payee:           "Coffee Shop",
					BankReferenceID: "FITID123",
					SourceLine:      1,
				},
				Match:  MatchResult{Confidence: MatchConfidenceNone},
				Action: ImportActionNew,
			},
		},
	}

	err := svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(creator.created) != 1 {
		t.Fatalf("created = %d, want 1", len(creator.created))
	}

	txn := creator.created[0]
	if !txn.HasBankReferenceID() {
		t.Error("expected bank reference ID on created transaction")
	}
	if txn.BankReferenceID.String != "FITID123" {
		t.Errorf("bank ref = %q, want %q", txn.BankReferenceID.String, "FITID123")
	}
}

func TestImportService_Execute_WithSplits(t *testing.T) {
	accountID := types.NewID()
	catID1 := types.NewID()
	catID2 := types.NewID()

	categories := map[string]types.ID{
		"Food":      catID1,
		"Transport": catID2,
	}

	svc, creator := makeTestImportService(categories, nil, nil, nil, nil)

	result := &ImportResult{
		Rows: []ImportRow{
			{
				Record: &ImportRecord{
					Date:       makeDate("2024-01-15"),
					Amount:     makeMoney("-100.00"),
					Payee:      "Store",
					SourceLine: 1,
					Splits: []ImportSplit{
						{Category: "Food", Amount: makeMoney("-70.00"), Memo: "groceries"},
						{Category: "Transport", Amount: makeMoney("-30.00"), Memo: "gas"},
					},
				},
				Match:  MatchResult{Confidence: MatchConfidenceNone},
				Action: ImportActionNew,
			},
		},
	}

	err := svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}

	if len(creator.created) != 1 {
		t.Fatalf("created = %d, want 1", len(creator.created))
	}

	txnID := creator.created[0].ID.String()
	splits := creator.splits[txnID]
	if len(splits) != 2 {
		t.Fatalf("splits = %d, want 2", len(splits))
	}
	if splits[0].CategoryID != catID1 {
		t.Errorf("split[0] category = %s, want %s", splits[0].CategoryID.String(), catID1.String())
	}
	if splits[1].CategoryID != catID2 {
		t.Errorf("split[1] category = %s, want %s", splits[1].CategoryID.String(), catID2.String())
	}
}

func TestImportService_Execute_CreateFailureContinues(t *testing.T) {
	accountID := types.NewID()
	svc, creator := makeTestImportService(nil, nil, nil, nil, nil)
	creator.failOn = 1 // Fail on first create

	result := &ImportResult{
		Rows: []ImportRow{
			{
				Record: &ImportRecord{
					Date:       makeDate("2024-01-15"),
					Amount:     makeMoney("-50.00"),
					SourceLine: 1,
				},
				Match:  MatchResult{Confidence: MatchConfidenceNone},
				Action: ImportActionNew,
			},
			{
				Record: &ImportRecord{
					Date:       makeDate("2024-01-16"),
					Amount:     makeMoney("-25.00"),
					SourceLine: 2,
				},
				Match:  MatchResult{Confidence: MatchConfidenceNone},
				Action: ImportActionNew,
			},
		},
	}

	err := svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v (should not fail overall)", err)
	}

	// First row failed, second succeeded
	if result.Created != 1 {
		t.Errorf("Created = %d, want 1", result.Created)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
}

func TestImportService_Execute_MatchWithBankRefUpdate(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()

	existingTxn := makeTxnWithPayee(accountID, "2024-01-15", "-50.00", payeeID)
	// Existing has no bank ref
	payeeNames := map[string]string{payeeID.String(): "Coffee Shop"}

	svc, creator := makeTestImportService(nil, nil, []*transaction.Transaction{existingTxn}, payeeNames, nil)

	csv := "Date,Amount,Payee\n2024-01-15,-50.00,Coffee Shop\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingUpdate},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	// Simulate having bank ref from OFX
	result.Rows[0].Record.BankReferenceID = "FITID456"

	err = svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(creator.updated) != 1 {
		t.Fatalf("updated = %d, want 1", len(creator.updated))
	}

	updated := creator.updated[0]
	if !updated.HasBankReferenceID() {
		t.Error("expected bank reference ID on updated transaction")
	}
	if updated.BankReferenceID.String != "FITID456" {
		t.Errorf("bank ref = %q, want %q", updated.BankReferenceID.String, "FITID456")
	}
}

func TestImportService_Preview_LowConfidenceReview(t *testing.T) {
	accountID := types.NewID()
	payeeID := types.NewID()

	// Same amount but very different date (> 3 days)
	existingTxn := makeTxnWithPayee(accountID, "2024-01-01", "-50.00", payeeID)
	payeeNames := map[string]string{payeeID.String(): "XYZZY Corp"}

	svc, _ := makeTestImportService(nil, nil, []*transaction.Transaction{existingTxn}, payeeNames, nil)

	// Import with same amount but different date (6 days apart) and different payee
	csv := "Date,Amount,Payee\n2024-01-07,-50.00,Totally Different\n"
	result, err := svc.Preview(
		strings.NewReader(csv), FormatCSV, accountID,
		ImportOptions{DuplicateHandling: DuplicateHandlingUpdate},
	)
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	// Low confidence match (amount matches, date within window but > 3 days, different payee)
	if result.Rows[0].Action != ImportActionReview {
		t.Errorf("row 0 action = %q, want %q", result.Rows[0].Action, ImportActionReview)
	}
}

func TestImportResult_Counts(t *testing.T) {
	result := &ImportResult{
		Rows: []ImportRow{
			{Action: ImportActionNew, Record: &ImportRecord{Amount: makeMoney("-10.00")}},
			{Action: ImportActionNew, Record: &ImportRecord{Amount: makeMoney("-20.00")}},
			{Action: ImportActionMatch, Record: &ImportRecord{Amount: makeMoney("-30.00")}},
			{Action: ImportActionSkip, Record: &ImportRecord{Amount: makeMoney("-40.00")}},
			{Action: ImportActionReview, Record: &ImportRecord{Amount: makeMoney("-50.00")}},
		},
	}

	if result.NewCount() != 2 {
		t.Errorf("NewCount = %d, want 2", result.NewCount())
	}
	if result.MatchCount() != 1 {
		t.Errorf("MatchCount = %d, want 1", result.MatchCount())
	}
	if result.SkipCount() != 1 {
		t.Errorf("SkipCount = %d, want 1", result.SkipCount())
	}
	if result.ReviewCount() != 1 {
		t.Errorf("ReviewCount = %d, want 1", result.ReviewCount())
	}

	// Total amount excludes skipped
	total := result.TotalAmount()
	expected := makeMoney("-110.00")
	if !total.Equal(expected) {
		t.Errorf("TotalAmount = %s, want %s", total.String(), expected.String())
	}
}

func TestParseImportStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected transaction.Status
	}{
		{"C", transaction.StatusCleared},
		{"c", transaction.StatusCleared},
		{"*", transaction.StatusCleared},
		{"R", transaction.StatusReconciled},
		{"r", transaction.StatusReconciled},
		{"X", transaction.StatusReconciled},
		{"x", transaction.StatusReconciled},
		{"U", transaction.StatusUncleared},
		{"", transaction.StatusUncleared},
		{"  ", transaction.StatusUncleared},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseImportStatus(tt.input)
			if got != tt.expected {
				t.Errorf("parseImportStatus(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestImportService_Execute_StatusFromImport(t *testing.T) {
	accountID := types.NewID()
	svc, creator := makeTestImportService(nil, nil, nil, nil, nil)

	result := &ImportResult{
		Rows: []ImportRow{
			{
				Record: &ImportRecord{
					Date:       makeDate("2024-01-15"),
					Amount:     makeMoney("-50.00"),
					Status:     "C",
					SourceLine: 1,
				},
				Match:  MatchResult{Confidence: MatchConfidenceNone},
				Action: ImportActionNew,
			},
		},
	}

	err := svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(creator.created) != 1 {
		t.Fatalf("created = %d, want 1", len(creator.created))
	}

	if creator.created[0].Status != transaction.StatusCleared {
		t.Errorf("status = %q, want %q", creator.created[0].Status, transaction.StatusCleared)
	}
}

func TestImportService_Execute_MemoAndCheckNumber(t *testing.T) {
	accountID := types.NewID()
	svc, creator := makeTestImportService(nil, nil, nil, nil, nil)

	result := &ImportResult{
		Rows: []ImportRow{
			{
				Record: &ImportRecord{
					Date:        makeDate("2024-01-15"),
					Amount:      makeMoney("-50.00"),
					Memo:        "Coffee and snacks",
					CheckNumber: "1234",
					SourceLine:  1,
				},
				Match:  MatchResult{Confidence: MatchConfidenceNone},
				Action: ImportActionNew,
			},
		},
	}

	err := svc.Execute(result, accountID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	txn := creator.created[0]
	if !txn.Memo.Valid || txn.Memo.String != "Coffee and snacks" {
		t.Errorf("memo = %v, want %q", txn.Memo, "Coffee and snacks")
	}
	if !txn.CheckNumber.Valid || txn.CheckNumber.String != "1234" {
		t.Errorf("check_number = %v, want %q", txn.CheckNumber, "1234")
	}
}
