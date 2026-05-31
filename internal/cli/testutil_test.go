package cli

import (
	"testing"

	"github.com/haskovec/tmoney/internal/cli/clitest"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// These are package-cli shims delegating to internal/cli/clitest so the
// existing package-cli tests keep compiling unchanged during the split. Each
// new noun PR repoints its tests at clitest directly; the shims are removed in
// PS-015 once nothing references the unexported names.

func createTestDBWithSecurity(t *testing.T) (string, *security.Security) {
	t.Helper()
	return clitest.CreateTestDBWithSecurity(t)
}

func createTestDBWithSecurityAndPrices(t *testing.T) (string, *security.Security) {
	t.Helper()
	return clitest.CreateTestDBWithSecurityAndPrices(t)
}

func createInvestmentTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	return clitest.CreateInvestmentTestDB(t, trackLots)
}

func ptrMoney(s string) *types.Money {
	return clitest.PtrMoney(s)
}

func createCorporateActionTestDB(t *testing.T, trackLots bool, withSecondSecurity bool) string {
	t.Helper()
	return clitest.CreateCorporateActionTestDB(t, trackLots, withSecondSecurity)
}
