package cli

import (
	"fmt"
	"os"
	"testing"
)

// runAsCLIEnv, when set to "1" in the environment of the test binary, makes
// TestMain run the tmoney CLI with the binary's arguments instead of the
// tests, as main.go does. A test starts its own binary with it to run tmoney
// in a child process, for example on a pseudo-terminal that it then closes
// (see hangup_unix_test.go), without a separate `go build`.
const runAsCLIEnv = "TMONEY_TEST_RUN_CLI"

func TestMain(m *testing.M) {
	if os.Getenv(runAsCLIEnv) == "1" {
		if err := Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
