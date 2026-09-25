package account

import (
	"errors"
	"fmt"
	"io"
	"strings"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/backup"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/db"
	"github.com/haskovec/tmoney/internal/dberrors"
	"github.com/haskovec/tmoney/internal/transfer"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/spf13/cobra"
)

// accountEditOptions are the inputs to `tmoney account edit`. The *Changed
// booleans record which editable flags were supplied so the command can apply
// delta semantics (only supplied flags take effect).
type accountEditOptions struct {
	file          string
	name          string
	newName       string
	accountType   string
	currency      string
	openingBal    string
	openingDate   string
	institution   string
	accountNumber string
	notes         string
	creditLimit   string
	interestRate  string
	confirm       bool

	newNameChanged       bool
	typeChanged          bool
	currencyChanged      bool
	openingBalChanged    bool
	openingDateChanged   bool
	institutionChanged   bool
	accountNumberChanged bool
	notesChanged         bool
	creditLimitChanged   bool
	interestRateChanged  bool
}

// newAccountEditCmd registers `tmoney account edit`. The database file is taken
// from the persistent `--file` / `-f` flag; `--name` selects the account and at
// least one delta flag must be supplied. Only supplied flags take effect; pass
// an empty string to a nullable field (`--institution`, `--account-number`,
// `--notes`, `--credit-limit`, `--interest-rate`) to clear it.
func newAccountEditCmd() *cobra.Command {
	opts := &accountEditOptions{}
	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit an existing account",
		Long: "Edit an account selected by `--name`. Only the supplied flags take " +
			"effect; pass an empty string to `--institution`, `--account-number`, " +
			"`--notes`, `--credit-limit`, or `--interest-rate` to clear that field. " +
			"Changing `--type` clears fields the new type doesn't support (credit " +
			"limit outside credit_card; interest rate outside checking/savings/" +
			"credit_card/investment/hsa/hsa_investment/loan; lot tracking outside " +
			"investment/hsa_investment). A type change that crosses ledgers (an " +
			"investment type to a regular one or back) prints a plan and needs " +
			"`--confirm`: investment→regular moves cash-only rows into the register " +
			"and is refused when any security row exists; regular→investment is " +
			"refused when the account has any rows. " +
			"Opening balance and opening date are locked while the account is closed " +
			"— reopen it first (`tmoney account reopen`). Lot tracking is not editable " +
			"here; use `tmoney investment enable-lots` / `disable-lots`.",
		Example: "  tmoney account edit --name Checking --new-name \"Main Checking\"\n" +
			"  tmoney account edit --name Checking --institution \"Acme Bank\" --notes \"\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			opts.newNameChanged = cmd.Flags().Changed("new-name")
			opts.typeChanged = cmd.Flags().Changed("type")
			opts.currencyChanged = cmd.Flags().Changed("currency")
			opts.openingBalChanged = cmd.Flags().Changed("opening-balance")
			opts.openingDateChanged = cmd.Flags().Changed("opening-date")
			opts.institutionChanged = cmd.Flags().Changed("institution")
			opts.accountNumberChanged = cmd.Flags().Changed("account-number")
			opts.notesChanged = cmd.Flags().Changed("notes")
			opts.creditLimitChanged = cmd.Flags().Changed("credit-limit")
			opts.interestRateChanged = cmd.Flags().Changed("interest-rate")
			opts.confirm, _ = cmd.Flags().GetBool("confirm")
			return runAccountEdit(opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.name, "name", "", "Name of the account to edit (required)")
	cmd.Flags().StringVar(&opts.newName, "new-name", "", "Rename the account")
	cmd.Flags().StringVar(&opts.accountType, "type", "", "New account type: checking, savings, credit_card, investment, hsa, hsa_investment, cash, loan, asset")
	cmd.Flags().StringVar(&opts.currency, "currency", "", "New currency code")
	cmd.Flags().StringVar(&opts.openingBal, "opening-balance", "", "New opening balance (locked while closed)")
	cmd.Flags().StringVar(&opts.openingDate, "opening-date", "", "New opening date YYYY-MM-DD (locked while closed)")
	cmd.Flags().StringVar(&opts.institution, "institution", "", "Institution name (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.accountNumber, "account-number", "", "Account number (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.notes, "notes", "", "Free-form notes (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.creditLimit, "credit-limit", "", "Credit limit, credit_card only (pass an empty string to clear)")
	cmd.Flags().StringVar(&opts.interestRate, "interest-rate", "", "Interest rate / APR (pass an empty string to clear)")
	cmd.Flags().BoolVar(&opts.confirm, "confirm", false, "Apply a type change that moves rows between ledgers (without it the plan is printed and nothing changes)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// runAccountEdit applies the supplied delta flags to an existing account.
func runAccountEdit(opts *accountEditOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	if !opts.anyFieldChanged() {
		return fmt.Errorf("at least one editable flag is required (--new-name, --type, --currency, " +
			"--opening-balance, --opening-date, --institution, --account-number, --notes, " +
			"--credit-limit, --interest-rate)")
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	acct, err := svc.Account.GetByName(opts.name)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.name)
	}

	// Parse every flag onto a copy first, so a bad value fails before
	// anything is written.
	edited := *acct
	if err := applyAccountEdits(&edited, opts); err != nil {
		return err
	}

	// A type change that crosses ledgers and moves rows is planned, shown,
	// and applied only with --confirm. The move and every other field edit
	// commit in one transaction (transfer.Service.ChangeAccountType).
	if edited.Type != acct.Type {
		plan, err := svc.Transfer.PlanAccountTypeChange(acct.ID, edited.Type)
		if err != nil {
			return err
		}
		if plan.MovesRows() {
			printLedgerMovePlan(w, plan, acct.Currency)
			if !opts.confirm {
				fmt.Fprintln(w, "Re-run with --confirm to apply (a backup is written first).")
				return nil
			}
			if errs := edited.Validate(); errs.HasErrors() {
				return fmt.Errorf("failed to update account: %w", &types.ServiceValidationError{Errors: errs})
			}
			backupPath, err := backupBeforeLedgerMove(database)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "Backup written: %s\n", backupPath)
			if err := svc.Transfer.ChangeAccountType(plan, &edited); err != nil {
				return updateError(err, &edited)
			}
			fmt.Fprintf(w, "Moved %d row(s) to the register.\n", plan.Total)
			printAccountUpdated(w, &edited)
			cmdutil.AutoBackupAfterModification(database)
			return nil
		}
	}

	if err := svc.Account.Update(&edited); err != nil {
		return updateError(err, &edited)
	}
	printAccountUpdated(w, &edited)
	cmdutil.AutoBackupAfterModification(database)
	return nil
}

// backupBeforeLedgerMove writes a manual backup through db.WithFileClosed, so
// the same handle and services stay valid afterward. A manual backup has its
// own file name, so the auto backup written after the edit cannot overwrite
// it, and retention never deletes it.
func backupBeforeLedgerMove(database *db.DB) (string, error) {
	var backupPath string
	err := database.WithFileClosed(func(path string) error {
		p, err := backup.CreateManualBackup(path)
		backupPath = p
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to write the backup; nothing was changed: %w", err)
	}
	return backupPath, nil
}

func updateError(err error, acct *accountdom.Account) error {
	var dupErr *dberrors.DuplicateError
	if errors.As(err, &dupErr) {
		return fmt.Errorf("account %q already exists", acct.Name)
	}
	return fmt.Errorf("failed to update account: %w", err)
}

func printAccountUpdated(w io.Writer, acct *accountdom.Account) {
	fmt.Fprintln(w, "Account updated.")
	fmt.Fprintf(w, "  Name: %s\n", acct.Name)
	fmt.Fprintf(w, "  Type: %s\n", acct.Type.DisplayName())
}

// anyFieldChanged reports whether at least one editable flag was supplied.
func (o *accountEditOptions) anyFieldChanged() bool {
	return o.newNameChanged || o.typeChanged || o.currencyChanged ||
		o.openingBalChanged || o.openingDateChanged || o.institutionChanged ||
		o.accountNumberChanged || o.notesChanged || o.creditLimitChanged ||
		o.interestRateChanged
}

// applyAccountEdits mutates acct in place per the supplied delta flags. The type
// change is applied first so field gating and clearing use the final type, then
// remaining flags are applied. Validation is left to svc.Account.Update.
func applyAccountEdits(acct *accountdom.Account, opts *accountEditOptions) error {
	// Resolve the final type up front so gating/clearing use it.
	finalType := acct.Type
	if opts.typeChanged {
		t, err := accountdom.ParseType(opts.accountType)
		if err != nil {
			return invalidTypeError(opts.accountType)
		}
		finalType = t
	}

	// Gate type-specific flags against the FINAL type when a non-empty value is
	// supplied (an empty value is a clear and always allowed).
	if opts.creditLimitChanged && opts.creditLimit != "" && finalType != accountdom.TypeCreditCard {
		return fmt.Errorf("--credit-limit is only valid for credit_card accounts")
	}
	if opts.interestRateChanged && opts.interestRate != "" && !accountTypeSupportsInterestRate(finalType) {
		return fmt.Errorf("--interest-rate is only valid for checking, savings, credit_card, investment, hsa, hsa_investment, or loan accounts")
	}

	// Opening balance/date are locked while the account is closed.
	if opts.openingBalChanged && acct.IsClosed() {
		return fmt.Errorf("opening balance is locked while the account is closed; reopen it first (tmoney account reopen)")
	}
	if opts.openingDateChanged && acct.IsClosed() {
		return fmt.Errorf("opening date is locked while the account is closed; reopen it first (tmoney account reopen)")
	}

	// Apply the type change and clear fields the final type doesn't support
	// (mirrors the TUI edit dialog's field-visibility clearing).
	acct.Type = finalType
	if finalType != accountdom.TypeCreditCard {
		acct.ClearCreditLimit()
	}
	if !accountTypeSupportsInterestRate(finalType) {
		acct.ClearInterestRate()
	}
	if !finalType.IsInvestmentType() {
		acct.TrackLots = false
	}

	if opts.newNameChanged {
		if opts.newName == "" {
			return fmt.Errorf("--new-name cannot be empty")
		}
		acct.Name = opts.newName
	}

	if opts.currencyChanged {
		if opts.currency == "" {
			return fmt.Errorf("--currency cannot be empty")
		}
		acct.Currency = strings.ToUpper(opts.currency)
	}

	if opts.openingBalChanged {
		bal, err := types.NewMoney(opts.openingBal)
		if err != nil {
			return fmt.Errorf("invalid --opening-balance: %w", err)
		}
		acct.OpeningBalance = bal
	}

	if opts.openingDateChanged {
		date, err := types.ParseDate(opts.openingDate)
		if err != nil {
			return fmt.Errorf("invalid --opening-date: %w", err)
		}
		acct.OpeningDate = date
	}

	if opts.institutionChanged {
		acct.SetInstitution(opts.institution)
	}
	if opts.accountNumberChanged {
		acct.SetAccountNumber(opts.accountNumber)
	}
	if opts.notesChanged {
		acct.SetNotes(opts.notes)
	}

	if opts.creditLimitChanged {
		if opts.creditLimit == "" {
			acct.ClearCreditLimit()
		} else {
			cl, err := types.NewMoney(opts.creditLimit)
			if err != nil {
				return fmt.Errorf("invalid --credit-limit: %w", err)
			}
			acct.SetCreditLimit(cl)
		}
	}

	if opts.interestRateChanged {
		if opts.interestRate == "" {
			acct.ClearInterestRate()
		} else {
			ir, err := types.NewMoney(opts.interestRate)
			if err != nil {
				return fmt.Errorf("invalid --interest-rate: %w", err)
			}
			acct.SetInterestRate(ir)
		}
	}

	acct.Touch()
	return nil
}

func invalidTypeError(got string) error {
	validTypes := make([]string, 0, len(accountdom.AllTypes()))
	for _, at := range accountdom.AllTypes() {
		validTypes = append(validTypes, string(at))
	}
	return fmt.Errorf("invalid --type %q: valid types are %s", got, strings.Join(validTypes, ", "))
}

// printLedgerMovePlan writes what a cross-ledger type change will do, in the
// shape specs/design-hsa-split.md §5.6 shows.
func printLedgerMovePlan(w io.Writer, plan *transfer.LedgerMovePlan, currency string) {
	fmt.Fprintf(w, "%s: %s -> %s\n", plan.AccountName, plan.From.DisplayName(), plan.To.DisplayName())
	fmt.Fprintf(w, "This moves %d row(s) from the investment ledger to the register:\n", plan.Total)
	for _, t := range plan.SortedTypes() {
		fmt.Fprintf(w, "  %-14s %d\n", t, plan.RowsByType[t])
	}
	fmt.Fprintf(w, "Net worth today: %s before, %s after.\n",
		cmdutil.FormatMoney(plan.NetWorthBefore, currency), cmdutil.FormatMoney(plan.NetWorthAfter, currency))
	if plan.FutureRows > 0 {
		fmt.Fprintf(w, "%d row(s) are dated after today. The register counts each one from its date.\n", plan.FutureRows)
	}
	fmt.Fprintln(w, "Moved rows keep their transfer links. They get no payee and no category.")
	fmt.Fprintln(w, "This cannot be undone.")
}

// accountTypeSupportsInterestRate mirrors the TUI dialog's interest-rate
// visibility set (checking, savings, credit_card, investment, hsa,
// hsa_investment, loan).
func accountTypeSupportsInterestRate(at accountdom.Type) bool {
	switch at {
	case accountdom.TypeChecking, accountdom.TypeSavings, accountdom.TypeCreditCard,
		accountdom.TypeInvestment, accountdom.TypeHSA, accountdom.TypeHSAInvestment, accountdom.TypeLoan:
		return true
	default:
		return false
	}
}
