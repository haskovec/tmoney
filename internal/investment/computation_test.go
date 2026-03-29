package investment

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// SM-073: ComputePricePerShare
// =============================================================================

func TestComputePricePerShare(t *testing.T) {
	t.Run("basic computation without commission", func(t *testing.T) {
		// total=1850, shares=10 → price=185
		total := types.MustNewMoney("1850")
		shares := types.MustNewQuantity("10")

		price, err := ComputePricePerShare(total, shares, types.ZeroMoney)
		if err != nil {
			t.Fatalf("ComputePricePerShare() error = %v", err)
		}
		if price.String() != "185" {
			t.Errorf("Expected price '185', got %q", price.String())
		}
	})

	t.Run("computation with commission", func(t *testing.T) {
		// total=1850, shares=10, commission=50 → price=(1850-50)/10=180
		total := types.MustNewMoney("1850")
		shares := types.MustNewQuantity("10")
		commission := types.MustNewMoney("50")

		price, err := ComputePricePerShare(total, shares, commission)
		if err != nil {
			t.Fatalf("ComputePricePerShare() error = %v", err)
		}
		if price.String() != "180" {
			t.Errorf("Expected price '180', got %q", price.String())
		}
	})

	t.Run("fractional shares", func(t *testing.T) {
		// total=100, shares=3 → price=33.333...
		total := types.MustNewMoney("100")
		shares := types.MustNewQuantity("3")

		price, err := ComputePricePerShare(total, shares, types.ZeroMoney)
		if err != nil {
			t.Fatalf("ComputePricePerShare() error = %v", err)
		}
		// Should be approximately 33.33
		f := price.Float64()
		if f < 33.33 || f > 33.34 {
			t.Errorf("Expected price ~33.33, got %f", f)
		}
	})

	t.Run("zero shares returns error", func(t *testing.T) {
		total := types.MustNewMoney("1000")
		shares := types.MustNewQuantity("0")

		_, err := ComputePricePerShare(total, shares, types.ZeroMoney)
		if err == nil {
			t.Fatal("Expected error for zero shares")
		}
	})

	t.Run("small amounts", func(t *testing.T) {
		// total=4.95, shares=1 → price=4.95
		total := types.MustNewMoney("4.95")
		shares := types.MustNewQuantity("1")

		price, err := ComputePricePerShare(total, shares, types.ZeroMoney)
		if err != nil {
			t.Fatalf("ComputePricePerShare() error = %v", err)
		}
		if price.String() != "4.95" {
			t.Errorf("Expected price '4.95', got %q", price.String())
		}
	})
}

// =============================================================================
// SM-074: ComputeTotalAmount
// =============================================================================

func TestComputeTotalAmount(t *testing.T) {
	t.Run("basic computation without commission", func(t *testing.T) {
		// shares=10, price=185 → total=1850
		shares := types.MustNewQuantity("10")
		price := types.MustNewMoney("185")

		total := ComputeTotalAmount(shares, price, types.ZeroMoney)
		if total.String() != "1850" {
			t.Errorf("Expected total '1850', got %q", total.String())
		}
	})

	t.Run("computation with commission", func(t *testing.T) {
		// shares=10, price=185, commission=4.95 → total=(10*185)+4.95=1854.95
		shares := types.MustNewQuantity("10")
		price := types.MustNewMoney("185")
		commission := types.MustNewMoney("4.95")

		total := ComputeTotalAmount(shares, price, commission)
		if total.String() != "1854.95" {
			t.Errorf("Expected total '1854.95', got %q", total.String())
		}
	})

	t.Run("fractional shares", func(t *testing.T) {
		// shares=2.5, price=100 → total=250
		shares := types.MustNewQuantity("2.5")
		price := types.MustNewMoney("100")

		total := ComputeTotalAmount(shares, price, types.ZeroMoney)
		if total.String() != "250" {
			t.Errorf("Expected total '250', got %q", total.String())
		}
	})

	t.Run("zero shares gives zero total", func(t *testing.T) {
		shares := types.MustNewQuantity("0")
		price := types.MustNewMoney("185")

		total := ComputeTotalAmount(shares, price, types.ZeroMoney)
		if !total.IsZero() {
			t.Errorf("Expected zero total, got %q", total.String())
		}
	})

	t.Run("penny precision", func(t *testing.T) {
		// shares=1, price=9.99, commission=0.01 → total=10.00
		shares := types.MustNewQuantity("1")
		price := types.MustNewMoney("9.99")
		commission := types.MustNewMoney("0.01")

		total := ComputeTotalAmount(shares, price, commission)
		if total.String() != "10" {
			t.Errorf("Expected total '10', got %q", total.String())
		}
	})
}

// =============================================================================
// SM-075: Smart computation integration
// =============================================================================

func TestSmartCompute(t *testing.T) {
	t.Run("shares+total auto-fills price_per_share", func(t *testing.T) {
		shares := types.MustNewQuantity("10")
		total := types.MustNewMoney("1850")

		result, err := SmartCompute(shares, &total, nil, types.ZeroMoney)
		if err != nil {
			t.Fatalf("SmartCompute() error = %v", err)
		}
		if result.TotalAmount.String() != "1850" {
			t.Errorf("Expected total '1850', got %q", result.TotalAmount.String())
		}
		if result.PricePerShare.String() != "185" {
			t.Errorf("Expected price '185', got %q", result.PricePerShare.String())
		}
	})

	t.Run("shares+price auto-fills total", func(t *testing.T) {
		shares := types.MustNewQuantity("10")
		price := types.MustNewMoney("185")

		result, err := SmartCompute(shares, nil, &price, types.ZeroMoney)
		if err != nil {
			t.Fatalf("SmartCompute() error = %v", err)
		}
		if result.TotalAmount.String() != "1850" {
			t.Errorf("Expected total '1850', got %q", result.TotalAmount.String())
		}
		if result.PricePerShare.String() != "185" {
			t.Errorf("Expected price '185', got %q", result.PricePerShare.String())
		}
	})

	t.Run("shares+total+commission auto-fills price_per_share with commission deducted", func(t *testing.T) {
		shares := types.MustNewQuantity("10")
		total := types.MustNewMoney("1850")
		commission := types.MustNewMoney("50")

		result, err := SmartCompute(shares, &total, nil, commission)
		if err != nil {
			t.Fatalf("SmartCompute() error = %v", err)
		}
		// price = (1850-50)/10 = 180
		if result.PricePerShare.String() != "180" {
			t.Errorf("Expected price '180', got %q", result.PricePerShare.String())
		}
		if result.TotalAmount.String() != "1850" {
			t.Errorf("Expected total '1850', got %q", result.TotalAmount.String())
		}
	})

	t.Run("shares+price+commission auto-fills total with commission added", func(t *testing.T) {
		shares := types.MustNewQuantity("10")
		price := types.MustNewMoney("185")
		commission := types.MustNewMoney("4.95")

		result, err := SmartCompute(shares, nil, &price, commission)
		if err != nil {
			t.Fatalf("SmartCompute() error = %v", err)
		}
		// total = (10*185)+4.95 = 1854.95
		if result.TotalAmount.String() != "1854.95" {
			t.Errorf("Expected total '1854.95', got %q", result.TotalAmount.String())
		}
		if result.PricePerShare.String() != "185" {
			t.Errorf("Expected price '185', got %q", result.PricePerShare.String())
		}
	})

	t.Run("both total and price given — total takes precedence", func(t *testing.T) {
		shares := types.MustNewQuantity("10")
		total := types.MustNewMoney("1850")
		price := types.MustNewMoney("999") // should be overridden

		result, err := SmartCompute(shares, &total, &price, types.ZeroMoney)
		if err != nil {
			t.Fatalf("SmartCompute() error = %v", err)
		}
		// price recomputed from total: 1850/10 = 185
		if result.PricePerShare.String() != "185" {
			t.Errorf("Expected price '185' (recomputed from total), got %q", result.PricePerShare.String())
		}
		if result.TotalAmount.String() != "1850" {
			t.Errorf("Expected total '1850', got %q", result.TotalAmount.String())
		}
	})

	t.Run("neither total nor price returns error", func(t *testing.T) {
		shares := types.MustNewQuantity("10")

		_, err := SmartCompute(shares, nil, nil, types.ZeroMoney)
		if err == nil {
			t.Fatal("Expected error when neither total nor price is given")
		}
	})

	t.Run("zero shares returns error", func(t *testing.T) {
		shares := types.MustNewQuantity("0")
		total := types.MustNewMoney("1000")

		_, err := SmartCompute(shares, &total, nil, types.ZeroMoney)
		if err == nil {
			t.Fatal("Expected error for zero shares")
		}
	})

	t.Run("negative shares returns error", func(t *testing.T) {
		shares := types.MustNewQuantity("-5")
		total := types.MustNewMoney("1000")

		_, err := SmartCompute(shares, &total, nil, types.ZeroMoney)
		if err == nil {
			t.Fatal("Expected error for negative shares")
		}
	})
}
