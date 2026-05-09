package cli

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

// tuiLauncher launches the bubbletea TUI for the given file path
// (empty file means "use the last-opened file from config or the
// default location"). Overridden in tests.
var tuiLauncher = defaultTUILauncher

func defaultTUILauncher(file string) error {
	return runTUI(file)
}

// Execute is the entry point used by main.go. It dispatches to the
// Cobra root command for all invocations.
func Execute() error {
	return executeWith(os.Args[1:], os.Stdout, os.Stderr)
}

func executeWith(args []string, stdout, stderr io.Writer) error {
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.Execute()
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
	cmd.AddCommand(newAccountCmd())
	cmd.AddCommand(newTransactionCmd())
	cmd.AddCommand(newTransferCmd())
	cmd.AddCommand(newScheduledCmd())
	cmd.AddCommand(newReconcileCmd())
	cmd.AddCommand(newSecurityCmd())
	cmd.AddCommand(newPriceCmd())
	cmd.AddCommand(newInvestmentCmd())
	cmd.AddCommand(newImportCmd())
	cmd.AddCommand(newExportCmd())
	cmd.AddCommand(newReportCmd())
	return cmd
}
