package investment

import (
	"testing"
	"time"

	"github.com/haskovec/tmoney/internal/price"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// hasFieldError checks if ValidationErrors contains an error for a specific field.
func hasFieldError(errs types.ValidationErrors, field string) bool {
	for _, e := range errs {
		if e.Field == field {
			return true
		}
	}
	return false
}

// =============================================================================
// SM-178: Zero-position cleanup
// =============================================================================

func TestSM178_ZeroPositionCleanup(t *testing.T) {
	t.Run("position deleted after selling all shares (non-lot-tracking)", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "Brokerage")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		// Confirm position exists
		positions, err := env.positionRepo.ListByAccount(acct.ID, false)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(positions) != 1 {
			t.Fatalf("Expected 1 position before sell, got %d", len(positions))
		}

		// Sell all shares
		sellTotal := types.MustNewMoney("1200.00")
		_, err = env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Position should be deleted — listing excluding zero should return empty
		positions, err = env.positionRepo.ListByAccount(acct.ID, true)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(positions) != 0 {
			t.Errorf("Expected 0 positions after full sell, got %d", len(positions))
		}

		// GetByAccountAndSecurity returns zero-initialized position (not from DB)
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if !pos.Shares.IsZero() {
			t.Errorf("Expected zero shares, got %s", pos.Shares)
		}
	})

	t.Run("lots marked closed when shares reach zero (lot-tracking)", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "Tax Brokerage")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(lots) != 1 {
			t.Fatalf("Expected 1 open lot, got %d", len(lots))
		}

		// Sell all shares from the lot
		sellTotal := types.MustNewMoney("1500.00")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// No open lots
		openLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)
		if len(openLots) != 0 {
			t.Errorf("Expected 0 open lots after full sell, got %d", len(openLots))
		}

		// Closed lot still exists with zero shares
		allLots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, true)
		if len(allLots) != 1 {
			t.Fatalf("Expected 1 total lot (closed), got %d", len(allLots))
		}
		if !allLots[0].Closed {
			t.Error("Expected lot to be marked as closed")
		}
		if !allLots[0].Shares.IsZero() {
			t.Errorf("Expected zero shares on closed lot, got %s", allLots[0].Shares)
		}
	})

	t.Run("zero-share positions excluded from portfolio holdings view", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "ViewTest")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		// Holdings should contain the security
		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(holdings) != 1 {
			t.Fatalf("Expected 1 holding before sell, got %d", len(holdings))
		}

		// Sell all shares
		sellTotal := types.MustNewMoney("1200.00")
		_, err = env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Holdings view should exclude zero-share positions
		holdings, err = holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(holdings) != 0 {
			t.Errorf("Expected 0 holdings after full sell, got %d", len(holdings))
		}
	})

	t.Run("zero-share lots excluded from portfolio holdings view (lot-tracking)", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createLotTrackingAccount(t, env.accountRepo, "LotViewTest")
		sec := createSec(t, env.secRepo, "TSLA")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		lots, _ := env.lotRepo.ListByAccountAndSecurity(acct.ID, sec.ID, false)

		// Sell all shares
		sellTotal := types.MustNewMoney("2500.00")
		allocs := []SellLotAllocation{{LotID: lots[0].ID, Shares: types.MustNewQuantity("10")}}
		_, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &sellTotal, nil, types.ZeroMoney, "", allocs)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Holdings view should return empty
		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(holdings) != 0 {
			t.Errorf("Expected 0 holdings after full lot sell, got %d", len(holdings))
		}
	})

	t.Run("partial sell leaves position and holdings intact", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "PartialSell")
		sec := createSec(t, env.secRepo, "NVDA")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		// Sell half
		sellTotal := types.MustNewMoney("600.00")
		_, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell() error = %v", err)
		}

		// Position should still exist with 5 shares
		pos, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, sec.ID)
		if err != nil {
			t.Fatalf("GetByAccountAndSecurity() error = %v", err)
		}
		if pos.Shares.String() != "5" {
			t.Errorf("Expected 5 shares remaining, got %s", pos.Shares)
		}

		// Holdings view should show the position
		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(holdings) != 1 {
			t.Errorf("Expected 1 holding after partial sell, got %d", len(holdings))
		}
	})

	t.Run("multiple securities: only fully-sold excluded", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "MultiSec")
		secA := createSec(t, env.secRepo, "AAPL")
		secB := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("20000.00"), "")
		buyA := types.MustNewMoney("1000.00")
		buyB := types.MustNewMoney("2000.00")
		_, _ = env.svc.Buy(acct.ID, secA.ID, date, types.MustNewQuantity("10"), &buyA, nil, types.ZeroMoney, "")
		_, _ = env.svc.Buy(acct.ID, secB.ID, date, types.MustNewQuantity("5"), &buyB, nil, types.ZeroMoney, "")

		// Sell all AAPL shares
		sellA := types.MustNewMoney("1200.00")
		_, err := env.svc.Sell(acct.ID, secA.ID, date, types.MustNewQuantity("10"), &sellA, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell AAPL() error = %v", err)
		}

		// Holdings view should show only GOOG
		holdingsRepo := NewHoldingsRepository(env.svc.db)
		holdings, err := holdingsRepo.ListByAccount(acct.ID)
		if err != nil {
			t.Fatalf("ListByAccount() error = %v", err)
		}
		if len(holdings) != 1 {
			t.Errorf("Expected 1 holding (GOOG only), got %d", len(holdings))
		}
		if len(holdings) > 0 && holdings[0].Ticker != "GOOG" {
			t.Errorf("Expected remaining holding to be GOOG, got %s", holdings[0].Ticker)
		}
	})
}

// =============================================================================
// SM-179: Multi-currency securities
// =============================================================================

func TestSM179_MultiCurrencySecurities(t *testing.T) {
	t.Run("same ticker different currencies creates distinct securities", func(t *testing.T) {
		env := createFullTestService(t)

		// Create RY in USD
		ryUSD := security.NewSecurity("RY", "Royal Bank of Canada (USD)", security.TypeStock)
		ryUSD.Currency = "USD"
		if err := env.secRepo.Create(ryUSD); err != nil {
			t.Fatalf("Create RY USD error = %v", err)
		}

		// Create RY in CAD — should succeed since ticker+currency is unique
		ryCAD := security.NewSecurity("RY", "Royal Bank of Canada (CAD)", security.TypeStock)
		ryCAD.Currency = "CAD"
		if err := env.secRepo.Create(ryCAD); err != nil {
			t.Fatalf("Create RY CAD error = %v", err)
		}

		// Both should be retrievable
		gotUSD, err := env.secRepo.GetByTicker("RY", "USD")
		if err != nil {
			t.Fatalf("GetByTicker(RY, USD) error = %v", err)
		}
		if gotUSD.ID != ryUSD.ID {
			t.Error("Expected USD security ID to match")
		}

		gotCAD, err := env.secRepo.GetByTicker("RY", "CAD")
		if err != nil {
			t.Fatalf("GetByTicker(RY, CAD) error = %v", err)
		}
		if gotCAD.ID != ryCAD.ID {
			t.Error("Expected CAD security ID to match")
		}
	})

	t.Run("duplicate ticker+currency is rejected", func(t *testing.T) {
		env := createFullTestService(t)

		sec1 := security.NewSecurity("SHOP", "Shopify USD", security.TypeStock)
		sec1.Currency = "USD"
		if err := env.secRepo.Create(sec1); err != nil {
			t.Fatalf("Create SHOP USD error = %v", err)
		}

		// Same ticker + same currency should fail
		sec2 := security.NewSecurity("SHOP", "Shopify USD Duplicate", security.TypeStock)
		sec2.Currency = "USD"
		err := env.secRepo.Create(sec2)
		if err == nil {
			t.Fatal("Expected error for duplicate ticker+currency")
		}
	})

	t.Run("GetByTicker without currency returns first match", func(t *testing.T) {
		env := createFullTestService(t)

		sec := security.NewSecurity("AMZN", "Amazon", security.TypeStock)
		if err := env.secRepo.Create(sec); err != nil {
			t.Fatalf("Create error = %v", err)
		}

		// GetByTicker with empty currency should find it
		got, err := env.secRepo.GetByTicker("AMZN", "")
		if err != nil {
			t.Fatalf("GetByTicker(AMZN, '') error = %v", err)
		}
		if got.ID != sec.ID {
			t.Error("Expected matching security ID")
		}
	})

	t.Run("positions tracked separately per security ID across currencies", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "MultiCurrency")
		date := types.NewDate(2024, time.March, 15)

		ryUSD := security.NewSecurity("RY", "Royal Bank USD", security.TypeStock)
		ryUSD.Currency = "USD"
		if err := env.secRepo.Create(ryUSD); err != nil {
			t.Fatalf("Create RY USD error = %v", err)
		}

		ryCAD := security.NewSecurity("RY", "Royal Bank CAD", security.TypeStock)
		ryCAD.Currency = "CAD"
		if err := env.secRepo.Create(ryCAD); err != nil {
			t.Fatalf("Create RY CAD error = %v", err)
		}

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("50000.00"), "")

		// Buy 10 shares of RY-USD and 20 shares of RY-CAD
		usdTotal := types.MustNewMoney("1000.00")
		cadTotal := types.MustNewMoney("2000.00")
		_, err := env.svc.Buy(acct.ID, ryUSD.ID, date, types.MustNewQuantity("10"), &usdTotal, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy RY USD error = %v", err)
		}
		_, err = env.svc.Buy(acct.ID, ryCAD.ID, date, types.MustNewQuantity("20"), &cadTotal, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy RY CAD error = %v", err)
		}

		// Each should have its own position
		posUSD, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, ryUSD.ID)
		if err != nil {
			t.Fatalf("GetPosition RY USD error = %v", err)
		}
		if posUSD.Shares.String() != "10" {
			t.Errorf("Expected 10 shares for RY-USD, got %s", posUSD.Shares)
		}

		posCAD, err := env.positionRepo.GetByAccountAndSecurity(acct.ID, ryCAD.ID)
		if err != nil {
			t.Fatalf("GetPosition RY CAD error = %v", err)
		}
		if posCAD.Shares.String() != "20" {
			t.Errorf("Expected 20 shares for RY-CAD, got %s", posCAD.Shares)
		}
	})

	t.Run("prices tracked independently per currency security", func(t *testing.T) {
		env := createFullTestService(t)
		date := types.NewDate(2024, time.March, 15)

		ryUSD := security.NewSecurity("RY", "Royal Bank USD", security.TypeStock)
		ryUSD.Currency = "USD"
		if err := env.secRepo.Create(ryUSD); err != nil {
			t.Fatalf("Create RY USD error = %v", err)
		}

		ryCAD := security.NewSecurity("RY", "Royal Bank CAD", security.TypeStock)
		ryCAD.Currency = "CAD"
		if err := env.secRepo.Create(ryCAD); err != nil {
			t.Fatalf("Create RY CAD error = %v", err)
		}

		// Add different prices for each
		pUSD := price.NewPrice(ryUSD.ID, date, types.MustNewMoney("120.00"), price.SourceManual)
		if err := env.priceRepo.Create(pUSD); err != nil {
			t.Fatalf("Create USD price error = %v", err)
		}

		pCAD := price.NewPrice(ryCAD.ID, date, types.MustNewMoney("160.00"), price.SourceManual)
		if err := env.priceRepo.Create(pCAD); err != nil {
			t.Fatalf("Create CAD price error = %v", err)
		}

		// Each should return its own price
		gotUSD, err := env.priceRepo.GetBySecurityAndDate(ryUSD.ID, date)
		if err != nil {
			t.Fatalf("Get USD price error = %v", err)
		}
		if gotUSD.Price.String() != "120" {
			t.Errorf("Expected USD price 120, got %s", gotUSD.Price)
		}

		gotCAD, err := env.priceRepo.GetBySecurityAndDate(ryCAD.ID, date)
		if err != nil {
			t.Fatalf("Get CAD price error = %v", err)
		}
		if gotCAD.Price.String() != "160" {
			t.Errorf("Expected CAD price 160, got %s", gotCAD.Price)
		}
	})
}

// =============================================================================
// SM-180: Price date validation
// =============================================================================

func TestSM180_PriceDateValidation(t *testing.T) {
	t.Run("price with future date is rejected", func(t *testing.T) {
		futureDate := types.Today().AddYears(1)
		secID := types.NewID()
		p := price.NewPrice(secID, futureDate, types.MustNewMoney("100.00"), price.SourceManual)

		errs := p.Validate()
		if !errs.HasErrors() {
			t.Fatal("Expected validation error for future date price")
		}
		if !hasFieldError(errs, "date") {
			t.Error("Expected 'date' field error")
		}
	})

	t.Run("price with today date is accepted", func(t *testing.T) {
		secID := types.NewID()
		p := price.NewPrice(secID, types.Today(), types.MustNewMoney("100.00"), price.SourceManual)

		errs := p.Validate()
		if hasFieldError(errs, "date") {
			t.Errorf("Today should be valid for price date, got error: %v", errs)
		}
	})

	t.Run("price with past date is accepted", func(t *testing.T) {
		secID := types.NewID()
		pastDate := types.NewDate(2024, time.January, 15)
		p := price.NewPrice(secID, pastDate, types.MustNewMoney("50.00"), price.SourceManual)

		errs := p.Validate()
		if hasFieldError(errs, "date") {
			t.Errorf("Past date should be valid for price, got error: %v", errs)
		}
	})

	t.Run("investment transaction with future date is rejected", func(t *testing.T) {
		futureDate := types.Today().AddYears(1)
		txn := NewTransaction(types.NewID(), futureDate, TransactionTypeDeposit, types.MustNewMoney("100.00"))

		errs := txn.Validate()
		if !errs.HasErrors() {
			t.Fatal("Expected validation error for future date transaction")
		}
		if !hasFieldError(errs, "date") {
			t.Error("Expected 'date' field error")
		}
	})

	t.Run("investment transaction with today date is accepted", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.Today(), TransactionTypeDeposit, types.MustNewMoney("100.00"))

		errs := txn.Validate()
		if hasFieldError(errs, "date") {
			t.Errorf("Today should be valid for transaction date, got error: %v", errs)
		}
	})

	t.Run("buy transaction with future date rejected via service", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "FutureBuy")
		sec := createSec(t, env.secRepo, "AAPL")
		today := types.Today()

		_, _ = env.svc.Deposit(acct.ID, today, types.MustNewMoney("50000.00"), "")

		futureDate := types.Today().AddYears(1)
		buyTotal := types.MustNewMoney("1000.00")
		_, err := env.svc.Buy(acct.ID, sec.ID, futureDate, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")
		if err == nil {
			t.Fatal("Expected error for buy with future date")
		}
	})

	t.Run("deposit with future date rejected via service", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "FutureDeposit")

		futureDate := types.Today().AddYears(1)
		_, err := env.svc.Deposit(acct.ID, futureDate, types.MustNewMoney("1000.00"), "")
		if err == nil {
			t.Fatal("Expected error for deposit with future date")
		}
	})
}

// =============================================================================
// SM-181: Commission handling edge cases
// =============================================================================

func TestSM181_CommissionEdgeCases(t *testing.T) {
	t.Run("zero commission buy works", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "ZeroComm")
		sec := createSec(t, env.secRepo, "AAPL")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")
		if err != nil {
			t.Fatalf("Buy with zero commission error = %v", err)
		}

		// Commission should not be set when zero
		if txn.Commission.Valid {
			t.Error("Commission should not be set when zero")
		}

		// Price should be 1000/10 = 100
		if txn.PricePerShare.Money.String() != "100" {
			t.Errorf("Expected price_per_share 100, got %s", txn.PricePerShare.Money)
		}
	})

	t.Run("zero commission sell works", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "ZeroCommSell")
		sec := createSec(t, env.secRepo, "GOOG")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")
		buyTotal := types.MustNewMoney("1000.00")
		_, _ = env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, types.ZeroMoney, "")

		sellTotal := types.MustNewMoney("1200.00")
		txn, err := env.svc.Sell(acct.ID, sec.ID, date, types.MustNewQuantity("5"), &sellTotal, nil, types.ZeroMoney, "", nil)
		if err != nil {
			t.Fatalf("Sell with zero commission error = %v", err)
		}

		if txn.Commission.Valid {
			t.Error("Commission should not be set when zero")
		}

		// Price should be 1200/5 = 240
		if txn.PricePerShare.Money.String() != "240" {
			t.Errorf("Expected price_per_share 240, got %s", txn.PricePerShare.Money)
		}
	})

	t.Run("commission exceeding total_amount is rejected", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "BigComm")
		sec := createSec(t, env.secRepo, "TSLA")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		// Buy with commission > total
		buyTotal := types.MustNewMoney("100.00")
		commission := types.MustNewMoney("200.00") // commission > total
		_, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, commission, "")
		if err == nil {
			t.Fatal("Expected error when commission exceeds total amount")
		}
	})

	t.Run("commission equal to total_amount is rejected", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "EqualComm")
		sec := createSec(t, env.secRepo, "MSFT")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		// Commission equals total — net amount is zero, price per share would be zero
		buyTotal := types.MustNewMoney("100.00")
		commission := types.MustNewMoney("100.00")
		_, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, commission, "")
		if err == nil {
			t.Fatal("Expected error when commission equals total amount (zero net)")
		}
	})

	t.Run("valid commission deducted from price calculation", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "ValidComm")
		sec := createSec(t, env.secRepo, "AMD")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("10000.00"), "")

		// Buy 10 shares, total $1004.95, commission $4.95
		// Net = 1004.95 - 4.95 = 1000.00, price per share = 100.00
		buyTotal := types.MustNewMoney("1004.95")
		commission := types.MustNewMoney("4.95")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), &buyTotal, nil, commission, "")
		if err != nil {
			t.Fatalf("Buy with commission error = %v", err)
		}

		if txn.PricePerShare.Money.String() != "100" {
			t.Errorf("Expected price_per_share 100, got %s", txn.PricePerShare.Money)
		}
		if !txn.Commission.Valid || txn.Commission.Money.String() != "4.95" {
			t.Errorf("Expected commission 4.95, got %s", txn.Commission.Money)
		}
	})

	t.Run("commission with sub-cent precision works", func(t *testing.T) {
		// SmartCompute handles arbitrary precision via alpacadecimal
		shares := types.MustNewQuantity("100")
		totalAmount := types.MustNewMoney("1001.50")
		commission := types.MustNewMoney("1.50")

		result, err := SmartCompute(shares, &totalAmount, nil, commission)
		if err != nil {
			t.Fatalf("SmartCompute error = %v", err)
		}

		// Net = 1001.50 - 1.50 = 1000.00, price = 10.00
		if result.PricePerShare.String() != "10" {
			t.Errorf("Expected price 10, got %s", result.PricePerShare)
		}
	})

	t.Run("negative commission rejected by validation", func(t *testing.T) {
		txn := NewTransaction(types.NewID(), types.NewDate(2024, time.March, 15), TransactionTypeBuy, types.MustNewMoney("-100.00"))
		txn.SecurityID = types.NullableID{ID: types.NewID(), Valid: true}
		txn.Shares = types.NullableQuantity{Quantity: types.MustNewQuantity("10"), Valid: true}
		txn.Commission = types.NullableMoney{Money: types.MustNewMoney("-5.00"), Valid: true}

		errs := txn.Validate()
		if !hasFieldError(errs, "commission") {
			t.Error("Expected validation error for negative commission")
		}
	})

	t.Run("buy with price_per_share and commission auto-fills total", func(t *testing.T) {
		env := createFullTestService(t)
		acct := createInvAccount(t, env.accountRepo, "AutoTotal")
		sec := createSec(t, env.secRepo, "NFLX")
		date := types.NewDate(2024, time.March, 15)

		_, _ = env.svc.Deposit(acct.ID, date, types.MustNewMoney("50000.00"), "")

		// Buy 10 shares at $100 each, $9.95 commission → total should be $1009.95
		price := types.MustNewMoney("100.00")
		commission := types.MustNewMoney("9.95")
		txn, err := env.svc.Buy(acct.ID, sec.ID, date, types.MustNewQuantity("10"), nil, &price, commission, "")
		if err != nil {
			t.Fatalf("Buy error = %v", err)
		}

		// Total should be (10 * 100) + 9.95 = 1009.95
		if txn.TotalAmount.String() != "-1009.95" {
			t.Errorf("Expected total amount -1009.95, got %s", txn.TotalAmount)
		}
	})
}
