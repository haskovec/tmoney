package imexport

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/models"
)

func makeDate(s string) models.Date {
	d, err := models.ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func makeTransaction(accountID models.ID, date string, amount string, payeeName string) ExistingTransaction {
	d := makeDate(date)
	a := models.MustNewMoney(amount)
	txn := models.NewTransaction(accountID, d, a)
	return ExistingTransaction{
		Transaction: txn,
		PayeeName:   payeeName,
	}
}

func makeImportRecord(date string, amount string, payee string) ImportRecord {
	d := makeDate(date)
	a := models.MustNewMoney(amount)
	return ImportRecord{
		Date:   d,
		Amount: a,
		Payee:  payee,
	}
}

// --- PayeeSimilarity tests ---

func TestPayeeSimilarity_ExactMatch(t *testing.T) {
	score := PayeeSimilarity("Kroger", "Kroger")
	if score != 1.0 {
		t.Errorf("exact match should be 1.0, got %f", score)
	}
}

func TestPayeeSimilarity_CaseInsensitive(t *testing.T) {
	score := PayeeSimilarity("KROGER", "kroger")
	if score != 1.0 {
		t.Errorf("case-insensitive match should be 1.0, got %f", score)
	}
}

func TestPayeeSimilarity_Containment(t *testing.T) {
	score := PayeeSimilarity("STARBUCKS #1234", "Starbucks")
	if score != 0.9 {
		t.Errorf("containment match should be 0.9, got %f", score)
	}
}

func TestPayeeSimilarity_ContainmentReverse(t *testing.T) {
	score := PayeeSimilarity("Shell", "SHELL OIL CO")
	if score != 0.9 {
		t.Errorf("reverse containment match should be 0.9, got %f", score)
	}
}

func TestPayeeSimilarity_Similar(t *testing.T) {
	score := PayeeSimilarity("NETFLIX.COM", "Netflix")
	if score < 0.5 {
		t.Errorf("similar names should score > 0.5, got %f", score)
	}
}

func TestPayeeSimilarity_Different(t *testing.T) {
	score := PayeeSimilarity("Kroger", "Amazon")
	if score > 0.3 {
		t.Errorf("different names should score low, got %f", score)
	}
}

func TestPayeeSimilarity_EmptyBoth(t *testing.T) {
	score := PayeeSimilarity("", "")
	if score != 1.0 {
		t.Errorf("both empty should be 1.0, got %f", score)
	}
}

func TestPayeeSimilarity_OneEmpty(t *testing.T) {
	score := PayeeSimilarity("Kroger", "")
	if score != 0 {
		t.Errorf("one empty should be 0, got %f", score)
	}
}

func TestPayeeSimilarity_SpecialCharacters(t *testing.T) {
	score := PayeeSimilarity("WAL-MART #1234", "Walmart")
	if score < 0.5 {
		t.Errorf("names with special chars should still match reasonably, got %f", score)
	}
}

// --- normalizePayee tests ---

func TestNormalizePayee(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Kroger", "kroger"},
		{"WAL-MART #1234", "walmart 1234"},
		{"  Multiple   Spaces  ", "multiple spaces"},
		{"STARBUCKS #1234 NYC", "starbucks 1234 nyc"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizePayee(tt.input)
		if got != tt.expected {
			t.Errorf("normalizePayee(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// --- diceCoefficient tests ---

func TestDiceCoefficient_Identical(t *testing.T) {
	score := diceCoefficient("hello", "hello")
	if score != 1.0 {
		t.Errorf("identical strings should score 1.0, got %f", score)
	}
}

func TestDiceCoefficient_Completely_Different(t *testing.T) {
	score := diceCoefficient("abc", "xyz")
	if score != 0 {
		t.Errorf("completely different should score 0, got %f", score)
	}
}

func TestDiceCoefficient_Partial(t *testing.T) {
	score := diceCoefficient("night", "nacht")
	if score <= 0 || score >= 1.0 {
		t.Errorf("partial match should be between 0 and 1, got %f", score)
	}
}

func TestDiceCoefficient_BothEmpty(t *testing.T) {
	score := diceCoefficient("", "")
	if score != 1.0 {
		t.Errorf("both empty should be 1.0, got %f", score)
	}
}

func TestDiceCoefficient_OneEmpty(t *testing.T) {
	score := diceCoefficient("hello", "")
	if score != 0 {
		t.Errorf("one empty should be 0, got %f", score)
	}
}

func TestDiceCoefficient_SingleChar(t *testing.T) {
	// Single chars produce no bigrams; both-empty bigram sets are treated as
	// identical (1.0), consistent with the Dice coefficient definition.
	score := diceCoefficient("a", "b")
	if score != 1.0 {
		t.Errorf("single chars with no bigrams should be 1.0 (both empty), got %f", score)
	}
}

// --- dateScore tests ---

func TestDateScore_ExactMatch(t *testing.T) {
	m := NewDefaultMatcher()
	score := m.dateScore(makeDate("2024-01-15"), makeDate("2024-01-15"))
	if score != 1.0 {
		t.Errorf("exact date match should be 1.0, got %f", score)
	}
}

func TestDateScore_OneDayApart(t *testing.T) {
	m := NewDefaultMatcher()
	score := m.dateScore(makeDate("2024-01-15"), makeDate("2024-01-16"))
	if score <= 0 || score >= 1.0 {
		t.Errorf("1 day apart should be between 0 and 1, got %f", score)
	}
}

func TestDateScore_ThreeDaysApart(t *testing.T) {
	m := NewDefaultMatcher()
	score := m.dateScore(makeDate("2024-01-15"), makeDate("2024-01-18"))
	if score <= 0 {
		t.Errorf("3 days apart should be > 0, got %f", score)
	}
}

func TestDateScore_SevenDaysApart(t *testing.T) {
	m := NewDefaultMatcher()
	score := m.dateScore(makeDate("2024-01-15"), makeDate("2024-01-22"))
	if score <= 0 {
		t.Errorf("7 days apart (at window edge) should still be > 0, got %f", score)
	}
}

func TestDateScore_EightDaysApart(t *testing.T) {
	m := NewDefaultMatcher()
	score := m.dateScore(makeDate("2024-01-15"), makeDate("2024-01-23"))
	if score != 0 {
		t.Errorf("8 days apart should be 0 (outside window), got %f", score)
	}
}

func TestDateScore_Decreasing(t *testing.T) {
	m := NewDefaultMatcher()
	base := makeDate("2024-01-15")
	prev := 2.0 // larger than any score
	for i := 0; i <= 7; i++ {
		d := models.Date(time.Time(base).AddDate(0, 0, i))
		score := m.dateScore(base, d)
		if score > prev {
			t.Errorf("date score should decrease with distance: day %d score %f > previous %f", i, score, prev)
		}
		prev = score
	}
}

// --- Match tests ---

func TestMatch_NoExisting(t *testing.T) {
	m := NewDefaultMatcher()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	result := m.Match(&record, nil)

	if result.Confidence != MatchConfidenceNone {
		t.Errorf("expected no match, got %s", result.Confidence)
	}
	if result.MatchedTransaction != nil {
		t.Error("expected nil matched transaction")
	}
}

func TestMatch_NoAmountMatch(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-15", "-75.00", "Kroger"),
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceNone {
		t.Errorf("expected no match when amounts differ, got %s", result.Confidence)
	}
}

func TestMatch_HighConfidence(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-15", "-50.00", "Kroger"),
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceHigh {
		t.Errorf("expected high confidence, got %s", result.Confidence)
	}
	if result.MatchedTransaction == nil {
		t.Fatal("expected matched transaction")
	}
	if result.Score <= 0 {
		t.Errorf("expected positive score, got %f", result.Score)
	}
}

func TestMatch_HighConfidence_SimilarPayee(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-5.75", "STARBUCKS #1234")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-16", "-5.75", "Starbucks"),
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceHigh {
		t.Errorf("expected high confidence for similar payee, got %s (payeeScore=%f)", result.Confidence, result.PayeeScore)
	}
}

func TestMatch_LowConfidence_DateFar(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-20", "-50.00", "Kroger"),
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceLow {
		t.Errorf("expected low confidence when date > 3 days apart, got %s", result.Confidence)
	}
}

func TestMatch_LowConfidence_PayeeDifferent(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-15", "-50.00", "Amazon"),
	}

	result := m.Match(&record, existing)
	// Amount matches and date is exact, but payee is completely different
	if result.Confidence == MatchConfidenceNone {
		t.Error("expected some match (amount + date match)")
	}
}

func TestMatch_FITID_Override(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()

	record := ImportRecord{
		Date:            makeDate("2024-01-15"),
		Amount:          models.MustNewMoney("-50.00"),
		Payee:           "Some Bank Name",
		BankReferenceID: "FITID123456",
	}

	existing := []ExistingTransaction{
		{
			Transaction:     models.NewTransaction(accountID, makeDate("2024-06-15"), models.MustNewMoney("-999.00")),
			PayeeName:       "Completely Different",
			BankReferenceID: "FITID123456",
		},
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceHigh {
		t.Errorf("FITID match should be high confidence, got %s", result.Confidence)
	}
	if !result.MatchedByFITID {
		t.Error("expected MatchedByFITID to be true")
	}
	if result.Score != 1.0 {
		t.Errorf("FITID match score should be 1.0, got %f", result.Score)
	}
}

func TestMatch_FITID_NoMatch(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()

	record := ImportRecord{
		Date:            makeDate("2024-01-15"),
		Amount:          models.MustNewMoney("-50.00"),
		Payee:           "Kroger",
		BankReferenceID: "FITID999",
	}

	existing := []ExistingTransaction{
		{
			Transaction:     models.NewTransaction(accountID, makeDate("2024-01-15"), models.MustNewMoney("-50.00")),
			PayeeName:       "Kroger",
			BankReferenceID: "FITID000",
		},
	}

	// FITID doesn't match, falls through to fuzzy matching
	result := m.Match(&record, existing)
	if result.MatchedByFITID {
		t.Error("should not match by FITID when IDs differ")
	}
	// But should still fuzzy-match
	if result.Confidence == MatchConfidenceNone {
		t.Error("expected fuzzy match fallback")
	}
}

func TestMatch_BestCandidateSelected(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")

	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-20", "-50.00", "Kroger"),  // far date
		makeTransaction(accountID, "2024-01-15", "-50.00", "Kroger"),  // exact match
		makeTransaction(accountID, "2024-01-18", "-50.00", "Walmart"), // wrong payee
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceHigh {
		t.Errorf("expected high confidence for best match, got %s", result.Confidence)
	}
	// Should pick the exact date/payee match (index 1)
	if result.MatchedTransaction != existing[1].Transaction {
		t.Error("should have selected the exact date+payee match")
	}
}

func TestMatch_OutsideDateWindow(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()
	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-02-15", "-50.00", "Kroger"),
	}

	result := m.Match(&record, existing)
	if result.Confidence != MatchConfidenceNone {
		t.Errorf("expected no match when date is way outside window, got %s", result.Confidence)
	}
}

// --- MatchAll tests ---

func TestMatchAll_Empty(t *testing.T) {
	m := NewDefaultMatcher()
	results := m.MatchAll(nil, nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMatchAll_NoExisting(t *testing.T) {
	m := NewDefaultMatcher()
	records := []ImportRecord{
		makeImportRecord("2024-01-15", "-50.00", "Kroger"),
		makeImportRecord("2024-01-16", "-25.00", "Amazon"),
	}

	results := m.MatchAll(records, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Confidence != MatchConfidenceNone {
			t.Errorf("result[%d]: expected no match, got %s", i, r.Confidence)
		}
	}
}

func TestMatchAll_OneToOneMatch(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()

	records := []ImportRecord{
		makeImportRecord("2024-01-15", "-50.00", "Kroger"),
		makeImportRecord("2024-01-16", "-25.00", "Amazon"),
	}

	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-15", "-50.00", "Kroger"),
		makeTransaction(accountID, "2024-01-16", "-25.00", "Amazon"),
	}

	results := m.MatchAll(records, existing)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for i, r := range results {
		if r.Confidence == MatchConfidenceNone {
			t.Errorf("result[%d]: expected match, got none", i)
		}
		if r.MatchedTransaction == nil {
			t.Errorf("result[%d]: expected matched transaction", i)
		}
	}
}

func TestMatchAll_NoDoubleMatching(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()

	// Two identical import records
	records := []ImportRecord{
		makeImportRecord("2024-01-15", "-50.00", "Kroger"),
		makeImportRecord("2024-01-15", "-50.00", "Kroger"),
	}

	// Only one existing transaction
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-15", "-50.00", "Kroger"),
	}

	results := m.MatchAll(records, existing)
	matchCount := 0
	for _, r := range results {
		if r.Confidence != MatchConfidenceNone {
			matchCount++
		}
	}

	if matchCount != 1 {
		t.Errorf("expected exactly 1 match (no double matching), got %d", matchCount)
	}
}

func TestMatchAll_FITID_Matching(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()

	records := []ImportRecord{
		{
			Date:            makeDate("2024-01-15"),
			Amount:          models.MustNewMoney("-50.00"),
			Payee:           "Bank Transfer",
			BankReferenceID: "FIT001",
		},
		{
			Date:            makeDate("2024-01-16"),
			Amount:          models.MustNewMoney("-30.00"),
			Payee:           "Bank Transfer 2",
			BankReferenceID: "FIT002",
		},
	}

	existing := []ExistingTransaction{
		{
			Transaction:     models.NewTransaction(accountID, makeDate("2024-01-15"), models.MustNewMoney("-50.00")),
			PayeeName:       "Some Name",
			BankReferenceID: "FIT001",
		},
		{
			Transaction:     models.NewTransaction(accountID, makeDate("2024-01-16"), models.MustNewMoney("-30.00")),
			PayeeName:       "Other Name",
			BankReferenceID: "FIT002",
		},
	}

	results := m.MatchAll(records, existing)
	for i, r := range results {
		if !r.MatchedByFITID {
			t.Errorf("result[%d]: expected FITID match", i)
		}
		if r.Confidence != MatchConfidenceHigh {
			t.Errorf("result[%d]: expected high confidence, got %s", i, r.Confidence)
		}
	}
}

func TestMatchAll_MixedMatching(t *testing.T) {
	m := NewDefaultMatcher()
	accountID := models.NewID()

	records := []ImportRecord{
		makeImportRecord("2024-01-15", "-50.00", "Kroger"),
		makeImportRecord("2024-01-16", "-999.00", "Unknown"),
		makeImportRecord("2024-01-17", "-25.00", "Amazon"),
	}

	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-15", "-50.00", "Kroger"),
		makeTransaction(accountID, "2024-01-17", "-25.00", "Amazon"),
	}

	results := m.MatchAll(records, existing)
	if results[0].Confidence == MatchConfidenceNone {
		t.Error("first record should match")
	}
	if results[1].Confidence != MatchConfidenceNone {
		t.Error("second record should not match (no amount match)")
	}
	if results[2].Confidence == MatchConfidenceNone {
		t.Error("third record should match")
	}
}

// --- absDaysDiff tests ---

func TestAbsDaysDiff(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"2024-01-15", "2024-01-15", 0},
		{"2024-01-15", "2024-01-16", 1},
		{"2024-01-16", "2024-01-15", 1},
		{"2024-01-15", "2024-01-22", 7},
		{"2024-01-01", "2024-02-01", 31},
	}

	for _, tt := range tests {
		got := absDaysDiff(makeDate(tt.a), makeDate(tt.b))
		if got != tt.expected {
			t.Errorf("absDaysDiff(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.expected)
		}
	}
}

// --- bigrams tests ---

func TestBigrams(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"ab", []string{"ab"}},
		{"abc", []string{"ab", "bc"}},
		{"a", nil},
		{"", nil},
	}

	for _, tt := range tests {
		got := bigrams(tt.input)
		if len(got) != len(tt.expected) {
			t.Errorf("bigrams(%q) = %v, want %v", tt.input, got, tt.expected)
			continue
		}
		for i := range got {
			if got[i] != tt.expected[i] {
				t.Errorf("bigrams(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
			}
		}
	}
}

// --- MatcherConfig tests ---

func TestCustomConfig(t *testing.T) {
	config := MatcherConfig{
		DateWindowDays:               3,
		HighConfidenceDateDays:       1,
		HighConfidencePayeeThreshold: 0.9,
	}
	m := NewMatcher(config)
	accountID := models.NewID()

	record := makeImportRecord("2024-01-15", "-50.00", "Kroger")
	existing := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-17", "-50.00", "Kroger"),
	}

	result := m.Match(&record, existing)
	// 2 days apart with strict 1-day high confidence threshold → low confidence
	if result.Confidence != MatchConfidenceLow {
		t.Errorf("expected low confidence with strict config, got %s", result.Confidence)
	}

	// 4 days outside narrow 3-day window → no match
	existing2 := []ExistingTransaction{
		makeTransaction(accountID, "2024-01-19", "-50.00", "Kroger"),
	}
	result2 := m.Match(&record, existing2)
	if result2.Confidence != MatchConfidenceNone {
		t.Errorf("expected no match outside narrow window, got %s", result2.Confidence)
	}
}

// --- sortCandidates tests ---

func TestSortCandidates(t *testing.T) {
	candidates := []matchCandidate{
		{recordIdx: 0, result: MatchResult{Score: 0.5}},
		{recordIdx: 1, result: MatchResult{Score: 0.9}},
		{recordIdx: 2, result: MatchResult{Score: 0.7}},
	}

	sortCandidates(candidates)

	if candidates[0].result.Score != 0.9 {
		t.Errorf("expected highest score first, got %f", candidates[0].result.Score)
	}
	if candidates[1].result.Score != 0.7 {
		t.Errorf("expected second highest score second, got %f", candidates[1].result.Score)
	}
	if candidates[2].result.Score != 0.5 {
		t.Errorf("expected lowest score last, got %f", candidates[2].result.Score)
	}
}
