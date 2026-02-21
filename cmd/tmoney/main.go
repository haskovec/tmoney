package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/haskovec/tmoney/internal/config"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/tui"
)

// Version information - will be set via build flags in production
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	opts, remaining, err := parseArgs(args)
	if err != nil {
		return err
	}

	// Handle --create
	if opts.createDB != "" {
		return runCreateDB(opts, stdout)
	}

	// Handle --list-accounts
	if opts.listAccounts {
		return runListAccounts(opts, stdout)
	}

	// Handle --add-account
	if opts.addAccount {
		return runAddAccount(opts, stdout)
	}

	// Handle --void
	if opts.voidTxn != "" {
		return runVoidTransaction(opts, stdout)
	}

	// Handle --add-transaction
	if opts.addTransaction {
		return runAddTransaction(opts, stdout)
	}

	// Handle --transfer
	if opts.transfer {
		return runTransfer(opts, stdout)
	}

	// Handle --post-scheduled
	if opts.postScheduled != "" {
		return runPostScheduled(opts, stdout)
	}

	// Handle --skip-scheduled
	if opts.skipScheduled != "" {
		return runSkipScheduled(opts, stdout)
	}

	// Handle --scheduled
	if opts.scheduled {
		return runScheduled(opts, stdout)
	}

	// Handle --report
	if opts.report {
		return runReport(opts, stdout)
	}

	// Handle --search
	if opts.searchTerm != "" {
		return runSearch(opts, stdout)
	}

	// Handle --transactions (check before --account since it uses --account as argument)
	if opts.transactions {
		return runTransactions(opts, stdout)
	}

	// Handle --account <name>
	if opts.accountName != "" {
		return runAccountDetails(opts, stdout)
	}

	// Handle --balance
	if opts.showBalance {
		return runBalance(opts, stdout)
	}

	// If remaining args include a file path, use it as the file
	if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "-") {
		if opts.file == "" {
			opts.file = remaining[0]
		}
	}

	// Default to TUI mode
	return runTUI(opts)
}

// runTUI launches the interactive TUI mode.
func runTUI(opts *cliOptions) error {
	// Load config
	cfg, err := config.Load()
	if err != nil {
		// Non-fatal: use empty config if load fails
		cfg = &config.Config{}
	}

	// Resolve file path: CLI flag > config > default
	if opts.file == "" {
		opts.file = cfg.ResolveDefaultFile()
	}
	if opts.file == "" {
		opts.file = filepath.Join(db.DefaultDirectory(), "default.tdb")
	}

	// Check if file exists, if not create it
	var database *db.DB
	if _, statErr := os.Stat(opts.file); os.IsNotExist(statErr) {
		// Create the directory if needed
		dir := filepath.Dir(opts.file)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		database, err = db.Create(opts.file)
		if err != nil {
			return fmt.Errorf("failed to create database: %w", err)
		}
	} else {
		database, err = db.Open(opts.file)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
	}
	defer database.Close()

	// Update recent files in config
	cfg.AddRecentFile(opts.file)
	_ = cfg.Save() // best-effort

	// Run TUI
	return tui.Run(database, cfg)
}
