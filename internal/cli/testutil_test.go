package cli

import (
	"testing"

	"github.com/haskovec/tmoney/internal/cli/clitest"
)

// These are package-cli shims delegating to internal/cli/clitest so the
// existing package-cli tests keep compiling unchanged during the split. Each
// new noun PR repoints its tests at clitest directly; the shims are removed in
// PS-015 once nothing references the unexported names. The security-fixture
// shims left with the price noun (PS-007); only createInvestmentTestDB remains,
// consumed by the residual workflow_test.go.

func createInvestmentTestDB(t *testing.T, trackLots bool) string {
	t.Helper()
	return clitest.CreateInvestmentTestDB(t, trackLots)
}
