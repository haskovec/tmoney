package report

import (
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func TestSortCategoriesByAmount(t *testing.T) {
	cats := []CategorySpending{
		{Name: "Groceries", Amount: types.MustNewMoney("50")},
		{Name: "Rent", Amount: types.MustNewMoney("900")},
		{Name: "Dining", Amount: types.MustNewMoney("50")},
		{Name: "Coffee", Amount: types.MustNewMoney("50")},
	}
	sortCategoriesByAmount(cats)

	want := []string{"Rent", "Coffee", "Dining", "Groceries"}
	for i, name := range want {
		if cats[i].Name != name {
			t.Errorf("position %d = %q, want %q (amount desc, then name)", i, cats[i].Name, name)
		}
	}
}
