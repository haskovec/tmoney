package cli

import (
	"io"
	"os"

	"github.com/haskovec/tmoney/internal/cli/account"
	"github.com/haskovec/tmoney/internal/cli/investment"
	"github.com/haskovec/tmoney/internal/cli/price"
	"github.com/haskovec/tmoney/internal/cli/security"
	"github.com/spf13/cobra"
)

// tuiLauncher launches the bubbletea TUI for the given file path
// (empty file means "use the last-opened file from config or the
// default location"). Overridden in tests.
var tuiLauncher = defaultTUILauncher

func defaultTUILauncher(file string) error {
	return runTUI(file)
}

// SwapTUILauncher replaces the TUI launcher with fn and returns a restore
// func that puts the original back. It is the exported seam tests use to
// intercept TUI launches; noun subpackages drive it through their own
// external `_test` packages instead of poking the unexported tuiLauncher
// var directly. Calls nest correctly: each call captures the launcher in
// effect at call time, so deferred restores unwind in LIFO order.
func SwapTUILauncher(fn func(string) error) (restore func()) {
	orig := tuiLauncher
	tuiLauncher = fn
	return func() {
		tuiLauncher = orig
	}
}

// Execute is the entry point used by main.go. It dispatches to the
// Cobra root command for all invocations.
func Execute() error {
	return ExecuteWith(os.Args[1:], os.Stdout, os.Stderr)
}

// ExecuteWith builds the root command, runs it with args, and writes
// output to stdout/stderr. It is the exported entry point that lets tests
// in noun subpackages drive the full CLI by argv without reaching into the
// unexported command tree.
func ExecuteWith(args []string, stdout, stderr io.Writer) error {
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.Execute()
}

// executeWith is a package-internal shim retained so the existing
// `package cli` tests compile unchanged during the package split. It
// delegates to ExecuteWith and is removed in PS-015.
func executeWith(args []string, stdout, stderr io.Writer) error {
	return ExecuteWith(args, stdout, stderr)
}

func newRootCmd() *cobra.Command {
	var fileFlag string

	cmd := &cobra.Command{
		Use:   "tmoney [file]",
		Short: "Personal finance management in your terminal",
		Long: "TMoney is a keyboard-driven personal finance manager. " +
			"Run with no arguments to launch the TUI, or use a subcommand " +
			"for scripted operations.",
		Example: "  tmoney                          Launch TUI with last opened file\n" +
			"  tmoney finances.tdb             Launch TUI with the given file\n" +
			"  tmoney --file=finances.tdb      Same as above\n" +
			"  tmoney version                  Print version information",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			file := fileFlag
			if file == "" && len(args) > 0 {
				file = args[0]
			}
			return tuiLauncher(file)
		},
	}
	cmd.PersistentFlags().StringVarP(&fileFlag, "file", "f", "", "Database file path")
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newThemeCmd())
	cmd.AddCommand(newDBCmd())
	cmd.AddCommand(account.NewCmd())
	cmd.AddCommand(newTransactionCmd())
	cmd.AddCommand(newTransferCmd())
	cmd.AddCommand(newScheduledCmd())
	cmd.AddCommand(newReconcileCmd())
	cmd.AddCommand(security.NewCmd())
	cmd.AddCommand(price.NewCmd())
	cmd.AddCommand(investment.NewCmd())
	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newReportCmd())
	return cmd
}
