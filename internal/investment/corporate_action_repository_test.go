package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// SM-145: CorporateActionRepository.Create
// =============================================================================

func TestCorporateActionRepository_Create(t *testing.T) {
	t.Run("creates a stock split action and verifies all fields", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		params := SplitParams{Numerator: 4, Denominator: 1}
		paramsJSON, err := params.ToJSON()
		if err != nil {
			t.Fatalf("ToJSON() error = %v", err)
		}

		ca := NewCorporateAction(ActionTypeSplit, sec.ID, types.NewDate(2024, 8, 9), paramsJSON)

		err = caRepo.Create(ca)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.ID != ca.ID {
			t.Errorf("Expected ID %v, got %v", ca.ID, retrieved.ID)
		}
		if retrieved.ActionType != ActionTypeSplit {
			t.Errorf("Expected action_type %q, got %q", ActionTypeSplit, retrieved.ActionType)
		}
		if retrieved.SecurityID != sec.ID {
			t.Errorf("Expected security_id %v, got %v", sec.ID, retrieved.SecurityID)
		}
		if retrieved.TargetSecurityID.Valid {
			t.Errorf("Expected target_security_id to be null, got %v", retrieved.TargetSecurityID)
		}
		if retrieved.ActionDate.Time().Format("2006-01-02") != "2024-08-09" {
			t.Errorf("Expected action_date 2024-08-09, got %v", retrieved.ActionDate.Time().Format("2006-01-02"))
		}
		if retrieved.Parameters != paramsJSON {
			t.Errorf("Expected parameters %q, got %q", paramsJSON, retrieved.Parameters)
		}

		// Verify JSON parameters can be parsed back
		parsedParams, err := ParseSplitParams(retrieved.Parameters)
		if err != nil {
			t.Fatalf("ParseSplitParams() error = %v", err)
		}
		if parsedParams.Numerator != 4 || parsedParams.Denominator != 1 {
			t.Errorf("Expected 4:1 split, got %d:%d", parsedParams.Numerator, parsedParams.Denominator)
		}
	})

	t.Run("creates a merger action with target security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		source := createInvestmentSecurityForTest(t, secRepo, "ATVI", "Activision Blizzard")
		target := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		params := MergerParams{ExchangeRatio: 0.9851, CashPerShare: 1.50}
		paramsJSON, err := params.ToJSON()
		if err != nil {
			t.Fatalf("ToJSON() error = %v", err)
		}

		ca := NewCorporateAction(ActionTypeMerger, source.ID, types.NewDate(2023, 10, 13), paramsJSON)
		ca.SetTargetSecurity(target.ID)

		err = caRepo.Create(ca)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.ActionType != ActionTypeMerger {
			t.Errorf("Expected action_type %q, got %q", ActionTypeMerger, retrieved.ActionType)
		}
		if !retrieved.TargetSecurityID.Valid {
			t.Fatal("Expected target_security_id to be set")
		}
		if retrieved.TargetSecurityID.ID != target.ID {
			t.Errorf("Expected target_security_id %v, got %v", target.ID, retrieved.TargetSecurityID.ID)
		}

		parsedParams, err := ParseMergerParams(retrieved.Parameters)
		if err != nil {
			t.Fatalf("ParseMergerParams() error = %v", err)
		}
		if parsedParams.ExchangeRatio != 0.9851 {
			t.Errorf("Expected exchange_ratio 0.9851, got %v", parsedParams.ExchangeRatio)
		}
		if parsedParams.CashPerShare != 1.50 {
			t.Errorf("Expected cash_per_share 1.50, got %v", parsedParams.CashPerShare)
		}
	})

	t.Run("creates a spin-off action with target security", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		parent := createInvestmentSecurityForTest(t, secRepo, "GE", "General Electric")
		spinoff := createInvestmentSecurityForTest(t, secRepo, "GEV", "GE Vernova")

		params := SpinOffParams{ShareRatio: 0.25, ParentAllocationPct: 80}
		paramsJSON, err := params.ToJSON()
		if err != nil {
			t.Fatalf("ToJSON() error = %v", err)
		}

		ca := NewCorporateAction(ActionTypeSpinOff, parent.ID, types.NewDate(2024, 4, 2), paramsJSON)
		ca.SetTargetSecurity(spinoff.ID)

		err = caRepo.Create(ca)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}

		if retrieved.ActionType != ActionTypeSpinOff {
			t.Errorf("Expected action_type %q, got %q", ActionTypeSpinOff, retrieved.ActionType)
		}
		if !retrieved.TargetSecurityID.Valid || retrieved.TargetSecurityID.ID != spinoff.ID {
			t.Errorf("Expected target_security_id %v, got %v", spinoff.ID, retrieved.TargetSecurityID)
		}

		parsedParams, err := ParseSpinOffParams(retrieved.Parameters)
		if err != nil {
			t.Fatalf("ParseSpinOffParams() error = %v", err)
		}
		if parsedParams.ShareRatio != 0.25 {
			t.Errorf("Expected share_ratio 0.25, got %v", parsedParams.ShareRatio)
		}
		if parsedParams.ParentAllocationPct != 80 {
			t.Errorf("Expected parent_allocation_pct 80, got %v", parsedParams.ParentAllocationPct)
		}
	})

	t.Run("verifies security_id foreign key", func(t *testing.T) {
		database := createTestDB(t)
		caRepo := NewCorporateActionRepository(database)

		params := SplitParams{Numerator: 2, Denominator: 1}
		paramsJSON, _ := params.ToJSON()

		ca := NewCorporateAction(ActionTypeSplit, types.NewID(), types.NewDate(2024, 1, 1), paramsJSON)

		err := caRepo.Create(ca)
		if err == nil {
			t.Fatal("Expected error for non-existent security_id, got nil")
		}
	})

	t.Run("creates a reverse split action", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "GE", "General Electric")

		params := SplitParams{Numerator: 1, Denominator: 8}
		paramsJSON, _ := params.ToJSON()

		ca := NewCorporateAction(ActionTypeReverseSplit, sec.ID, types.NewDate(2021, 8, 2), paramsJSON)

		err := caRepo.Create(ca)
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		retrieved, err := caRepo.GetByID(ca.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if retrieved.ActionType != ActionTypeReverseSplit {
			t.Errorf("Expected action_type %q, got %q", ActionTypeReverseSplit, retrieved.ActionType)
		}
	})
}

func TestCorporateActionRepository_GetByID(t *testing.T) {
	t.Run("returns NotFoundError for non-existent ID", func(t *testing.T) {
		database := createTestDB(t)
		caRepo := NewCorporateActionRepository(database)

		_, err := caRepo.GetByID(types.NewID())
		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if _, ok := err.(*dberrors.NotFoundError); !ok {
			t.Fatalf("Expected NotFoundError, got %T: %v", err, err)
		}
	})
}

// =============================================================================
// SM-146: CorporateActionRepository.ListBySecurity
// =============================================================================

func TestCorporateActionRepository_ListBySecurity(t *testing.T) {
	t.Run("lists actions for a security ordered by date", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")

		// Create actions in non-chronological order
		split1Params, _ := SplitParams{Numerator: 7, Denominator: 1}.ToJSON()
		split2Params, _ := SplitParams{Numerator: 4, Denominator: 1}.ToJSON()

		ca1 := NewCorporateAction(ActionTypeSplit, sec.ID, types.NewDate(2014, 6, 9), split1Params)
		ca2 := NewCorporateAction(ActionTypeSplit, sec.ID, types.NewDate(2020, 8, 31), split2Params)

		// Insert in reverse order
		if err := caRepo.Create(ca2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := caRepo.Create(ca1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		results, err := caRepo.ListBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity() error = %v", err)
		}
		if len(results) != 2 {
			t.Fatalf("Expected 2 actions, got %d", len(results))
		}
		// Should be ordered by action_date ASC
		if results[0].ID != ca1.ID {
			t.Errorf("Expected first result to be 2014 split (ID %v), got %v", ca1.ID, results[0].ID)
		}
		if results[1].ID != ca2.ID {
			t.Errorf("Expected second result to be 2020 split (ID %v), got %v", ca2.ID, results[1].ID)
		}
	})

	t.Run("includes actions where security is source or target", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		source := createInvestmentSecurityForTest(t, secRepo, "ATVI", "Activision Blizzard")
		target := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		mergerParams, _ := MergerParams{ExchangeRatio: 0.9851}.ToJSON()
		ca := NewCorporateAction(ActionTypeMerger, source.ID, types.NewDate(2023, 10, 13), mergerParams)
		ca.SetTargetSecurity(target.ID)

		if err := caRepo.Create(ca); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Should appear when querying by source security
		sourceResults, err := caRepo.ListBySecurity(source.ID)
		if err != nil {
			t.Fatalf("ListBySecurity(source) error = %v", err)
		}
		if len(sourceResults) != 1 {
			t.Fatalf("Expected 1 action for source security, got %d", len(sourceResults))
		}

		// Should also appear when querying by target security
		targetResults, err := caRepo.ListBySecurity(target.ID)
		if err != nil {
			t.Fatalf("ListBySecurity(target) error = %v", err)
		}
		if len(targetResults) != 1 {
			t.Fatalf("Expected 1 action for target security, got %d", len(targetResults))
		}

		if sourceResults[0].ID != targetResults[0].ID {
			t.Error("Expected same action returned for both source and target queries")
		}
	})

	t.Run("returns empty list for security with no actions", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		sec := createInvestmentSecurityForTest(t, secRepo, "GOOG", "Alphabet Inc.")

		results, err := caRepo.ListBySecurity(sec.ID)
		if err != nil {
			t.Fatalf("ListBySecurity() error = %v", err)
		}
		if len(results) != 0 {
			t.Errorf("Expected 0 actions, got %d", len(results))
		}
	})

	t.Run("does not include actions for other securities", func(t *testing.T) {
		database := createTestDB(t)
		secRepo := security.NewRepository(database)
		caRepo := NewCorporateActionRepository(database)

		sec1 := createInvestmentSecurityForTest(t, secRepo, "AAPL", "Apple Inc.")
		sec2 := createInvestmentSecurityForTest(t, secRepo, "MSFT", "Microsoft Corp.")

		splitParams, _ := SplitParams{Numerator: 4, Denominator: 1}.ToJSON()

		ca1 := NewCorporateAction(ActionTypeSplit, sec1.ID, types.NewDate(2020, 8, 31), splitParams)
		ca2 := NewCorporateAction(ActionTypeSplit, sec2.ID, types.NewDate(2003, 2, 18), splitParams)

		if err := caRepo.Create(ca1); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		if err := caRepo.Create(ca2); err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		results, err := caRepo.ListBySecurity(sec1.ID)
		if err != nil {
			t.Fatalf("ListBySecurity() error = %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("Expected 1 action for AAPL, got %d", len(results))
		}
		if results[0].ID != ca1.ID {
			t.Errorf("Expected AAPL action, got action with ID %v", results[0].ID)
		}
	})
}
