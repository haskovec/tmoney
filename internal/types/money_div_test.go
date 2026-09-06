package types

import "testing"

func TestMoneyDiv(t *testing.T) {
	t.Run("divides by a non-zero divisor", func(t *testing.T) {
		got := MustNewMoney("10.00").Div(4)
		if want := MustNewMoney("2.5"); !got.Equal(want) {
			t.Errorf("Div(4) = %s, want %s", got, want)
		}
	})

	t.Run("panics on a zero divisor instead of returning zero", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("Div(0) should panic; a silent zero hides a caller bug as a wrong amount")
			}
		}()
		_ = MustNewMoney("10.00").Div(0)
	})
}
