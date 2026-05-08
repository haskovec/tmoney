package cli

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

// run is a private alias for RunLegacy preserved for the existing test
// suite that predates the cli package extraction.
var run = RunLegacy

// RunLegacy executes the legacy flag-style CLI dispatcher. It parses the
// pre-Cobra `--flag` style invocation and dispatches to the appropriate
// run* handler in commands.go. New subcommands should be added to the Cobra
// router in root.go; RunLegacy stays in place until every legacy flag has
// been migrated.
func RunLegacy(args []string, stdout, stderr io.Writer) error {
	opts, remaining, err := parseArgs(args)
	if err != nil {
		return err
	}

	// Handle --add-scheduled
	if opts.addScheduled {
		return runAddScheduled(opts, stdout)
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

	// Handle --start-reconcile
	if opts.startReconcile {
		return runStartReconcile(opts, stdout)
	}

	// Handle --mark-reconciled
	if len(opts.markReconciled) > 0 {
		return runMarkReconciled(opts, stdout)
	}

	// Handle --finish-reconcile
	if opts.finishReconcile {
		return runFinishReconcile(opts, stdout)
	}

	// Handle --reconcile-status
	if opts.reconcileStatus {
		return runReconcileStatus(opts, stdout)
	}

	// Handle --list-securities
	if opts.listSecurities {
		return runListSecurities(opts, stdout)
	}

	// Handle --add-security
	if opts.addSecurity {
		return runAddSecurity(opts, stdout)
	}

	// Handle --edit-security
	if opts.editSecurity != "" {
		return runEditSecurity(opts, stdout)
	}

	// Handle --hide-security
	if opts.hideSecurity != "" {
		return runHideSecurity(opts, stdout)
	}

	// Handle --unhide-security
	if opts.unhideSecurity != "" {
		return runUnhideSecurity(opts, stdout)
	}

	// Handle --delete-security
	if opts.deleteSecurity != "" {
		return runDeleteSecurity(opts, stdout)
	}

	// Handle --security (show detail) — after add/edit/hide/unhide/delete
	if opts.securityTicker != "" {
		return runSecurityDetail(opts, stdout)
	}

	// Handle --prices (list prices for a ticker)
	if opts.listPrices {
		return runListPrices(opts, stdout)
	}

	// Handle --add-price
	if opts.addPrice {
		return runAddPrice(opts, stdout)
	}

	// Handle --current-price
	if opts.currentPrice {
		return runCurrentPrice(opts, stdout)
	}

	// Handle --import-prices
	if opts.importPrices != "" {
		return runImportPrices(opts, stdout)
	}

	// Handle --update-prices
	if opts.updatePrices {
		return runUpdatePrices(opts, stdout)
	}

	// Handle --buy
	if opts.buy {
		return runBuy(opts, stdout)
	}

	// Handle --sell
	if opts.sell {
		return runSell(opts, stdout)
	}

	// Handle --dividend
	if opts.dividend {
		return runDividend(opts, stdout)
	}

	// Handle --reinvest
	if opts.reinvest {
		return runReinvest(opts, stdout)
	}

	// Handle --investment-fee
	if opts.investmentFee {
		return runInvestmentFee(opts, stdout)
	}

	// Handle --invest-deposit
	if opts.investDeposit {
		return runInvestDeposit(opts, stdout)
	}

	// Handle --invest-withdraw
	if opts.investWithdraw {
		return runInvestWithdraw(opts, stdout)
	}

	// Handle --transfer-shares
	if opts.transferShares {
		return runTransferShares(opts, stdout)
	}

	// Handle --split
	if opts.split {
		return runSplit(opts, stdout)
	}

	// Handle --merge-security
	if opts.mergeSecurity {
		return runMergeSecurity(opts, stdout)
	}

	// Handle --spin-off
	if opts.spinOff {
		return runSpinOff(opts, stdout)
	}

	// Handle --portfolio
	if opts.portfolio {
		return runPortfolio(opts, stdout)
	}

	// Handle --import
	if opts.importFile != "" {
		return runImport(opts, stdout)
	}

	// Handle --export
	if opts.exportFile != "" {
		return runExport(opts, stdout)
	}

	// Handle --link-transfers
	if opts.linkTransfers {
		return runLinkTransfers(opts, stdout)
	}

	// Handle --report
	if opts.report {
		return runReport(opts, stdout)
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
