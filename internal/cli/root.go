package cli

import (
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// tuiLauncher launches the bubbletea TUI for the given file path
// (empty file means "use the last-opened file from config or the
// default location"). Overridden in tests.
var tuiLauncher = defaultTUILauncher

// legacyRunner dispatches a legacy `--flag` style invocation. Overridden
// in tests so the dispatcher can be exercised without reaching the real
// command handlers.
var legacyRunner = RunLegacy

func defaultTUILauncher(file string) error {
	return runTUI(&cliOptions{file: file})
}

// Execute is the entry point used by main.go. It inspects os.Args to
// decide whether to dispatch to Cobra (for new subcommands and the
// no-args TUI launch) or fall through to the legacy `--flag` runner.
func Execute() error {
	return executeWith(os.Args[1:], os.Stdout, os.Stderr)
}

func executeWith(args []string, stdout, stderr io.Writer) error {
	if isLegacyInvocation(args) {
		return legacyRunner(args, stdout, stderr)
	}
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd.Execute()
}

// cobraKnownFlags are the long-form flags the Cobra root accepts. Any
// `--flag` outside this set indicates a legacy invocation.
var cobraKnownFlags = map[string]bool{
	"--file": true,
	"--help": true,
}

// cobraSubcommands lists names of subcommands registered on the Cobra
// root. Used by isLegacyInvocation so a legacy-flag-looking arg later
// in the command line cannot misroute a real subcommand call.
var cobraSubcommands = map[string]bool{
	"version":     true,
	"theme":       true,
	"db":          true,
	"account":     true,
	"transaction": true,
}

// isLegacyInvocation reports whether args contains any `--flag` that
// only the legacy dispatcher knows about. A leading subcommand name
// short-circuits to false so `tmoney version --whatever` always goes
// to Cobra.
func isLegacyInvocation(args []string) bool {
	for _, a := range args {
		if cobraSubcommands[a] {
			return false
		}
		flag := a
		if before, _, ok := strings.Cut(a, "="); ok {
			flag = before
		}
		if cobraKnownFlags[flag] {
			continue
		}
		if strings.HasPrefix(flag, "--") {
			return true
		}
	}
	return false
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
	return cmd
}
