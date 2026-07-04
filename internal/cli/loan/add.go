package loan

import (
	"errors"
	"fmt"
	"io"
	"strings"

	accountdom "github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	categorydom "github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	"github.com/haskovec/tmoney/internal/dberrors"
	loandom "github.com/haskovec/tmoney/internal/loan"
	scheduleddom "github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
	"github.com/haskovec/tmoney/internal/undo"
	"github.com/spf13/cobra"
)

// loanAddOptions are the inputs to `tmoney loan add`.
type loanAddOptions struct {
	file              string
	name              string
	currentBalance    string
	principal         string
	rate              string
	payment           string
	termMonths        int
	openDate          string
	nextPaymentDate   string
	fromAccount       string
	payee             string
	interestCategory  string
	principalCategory string
	escrow            []string
	assetName         string
	assetValue        string
	institution       string
	autoPost          bool
	leadDays          int
}

// newLoanAddCmd registers `tmoney loan add`. The database file is taken from the
// persistent `--file` / `-f` flag inherited from the root command. `--name`,
// `--rate`, `--from-account`, and `--next-payment-date` are required, along with
// a balance (`--current-balance`, or `--principal` when only that is given).
func newLoanAddCmd() *cobra.Command {
	opts := &loanAddOptions{}
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Set up a loan: loan account, optional asset account, and payment schedule",
		Long: "Create an amortized loan in one atomic operation: a loan account " +
			"(liability, stored negative), an optional linked asset account, and a " +
			"monthly loan-shaped payment schedule whose interest/principal split is " +
			"recomputed from the live balance every time it posts.\n\n" +
			"Provide the monthly P&I payment with --payment (escrow-exclusive). " +
			"Omit it and pass --principal and --term-months to have the payment " +
			"computed from the amortization formula (it is printed for comparison " +
			"against your statement). --escrow lines and --interest-category use " +
			"Parent or Parent:Subcategory paths and are created if they don't exist " +
			"(there is no `category add` CLI); the interest category defaults to " +
			"Loan:Interest when --rate is above 0. The principal transfer line is " +
			"labeled Loan:Principal by default (--principal-category to override, " +
			"--principal-category=\"\" to leave it unlabeled).",
		Example: "  tmoney loan add --name Mortgage --current-balance 312450.22 --rate 6.5 \\\n" +
			"    --payment 2401.86 --next-payment-date 2026-08-01 --from-account Checking \\\n" +
			"    --escrow \"Housing:Property Tax=650\" --escrow \"Housing:Home Insurance=120\" \\\n" +
			"    --payee \"Wells Fargo\" --asset-name \"123 Main St\" --asset-value 450000\n" +
			"  tmoney loan add --name \"Car Loan\" --principal 32000 --rate 5.9 \\\n" +
			"    --term-months 60 --open-date 2026-07-01 --next-payment-date 2026-08-01 \\\n" +
			"    --from-account Checking",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.file, _ = cmd.Flags().GetString("file")
			return runLoanAdd(cmd, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.name, "name", "", "Loan account name (required)")
	cmd.Flags().StringVar(&opts.currentBalance, "current-balance", "", "What you owe today, entered positive (required unless --principal is given)")
	cmd.Flags().StringVar(&opts.principal, "principal", "", "Original loan principal; prefills the payment and, at origination, doubles as the balance")
	cmd.Flags().StringVar(&opts.rate, "rate", "", "Annual interest rate / APR, 0-100 (required)")
	cmd.Flags().StringVar(&opts.payment, "payment", "", "Monthly P&I payment, escrow-exclusive (required unless --principal and --term-months are given)")
	cmd.Flags().IntVar(&opts.termMonths, "term-months", 0, "Loan term in months; with --principal, computes the payment")
	cmd.Flags().StringVar(&opts.openDate, "open-date", "", "Origination date YYYY-MM-DD; recorded only for a new loan at origination")
	cmd.Flags().StringVar(&opts.nextPaymentDate, "next-payment-date", "", "First unposted payment date YYYY-MM-DD (required)")
	cmd.Flags().StringVar(&opts.fromAccount, "from-account", "", "Funding account name; active, non-investment (required)")
	cmd.Flags().StringVar(&opts.payee, "payee", "", "Payee / servicer name (auto-created if it doesn't exist)")
	cmd.Flags().StringVar(&opts.interestCategory, "interest-category", "", "Interest category path (default Loan:Interest); created if it doesn't exist")
	cmd.Flags().StringVar(&opts.principalCategory, "principal-category", "", "Principal-line category path (default Loan:Principal); created if it doesn't exist; pass \"\" to leave the principal line unlabeled")
	cmd.Flags().StringArrayVar(&opts.escrow, "escrow", nil, "Escrow line Category=Amount (repeatable), e.g. \"Housing:Property Tax=650\"; categories are created if needed")
	cmd.Flags().StringVar(&opts.assetName, "asset-name", "", "Name of a linked asset account to create (e.g. the house or car)")
	cmd.Flags().StringVar(&opts.assetValue, "asset-value", "", "Current value of the linked asset (required with --asset-name)")
	cmd.Flags().StringVar(&opts.institution, "institution", "", "Lender / institution name")
	cmd.Flags().BoolVar(&opts.autoPost, "auto-post", false, "Post the payment automatically when due (default off)")
	cmd.Flags().IntVar(&opts.leadDays, "lead-days", 0, "Auto-post lead days: 0, 3, or 7 (requires --auto-post)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("rate")
	_ = cmd.MarkFlagRequired("from-account")
	_ = cmd.MarkFlagRequired("next-payment-date")
	return cmd
}

// runLoanAdd validates the flags, resolves names to IDs, and creates the loan
// account, optional asset account, and monthly loan-shaped schedule atomically.
// It reuses the same shared record assembly as the TUI loan wizard
// (scheduled.BuildLoanSchedule) so a loan created from the CLI is identical to
// one created in the TUI.
func runLoanAdd(cmd *cobra.Command, opts *loanAddOptions, w io.Writer) error {
	if err := cmdutil.RequireFile(opts.file); err != nil {
		return err
	}

	apr, err := types.NewMoney(opts.rate)
	if err != nil {
		return fmt.Errorf("invalid --rate: %w", err)
	}
	if apr.Float64() < 0 || apr.Float64() >= 100 {
		return fmt.Errorf("--rate must be between 0 and 100, got %s", apr)
	}

	// Optional original principal (prefill + opening-date rule + payment compute).
	var principal types.Money
	havePrincipal := opts.principal != ""
	if havePrincipal {
		principal, err = types.NewMoney(opts.principal)
		if err != nil {
			return fmt.Errorf("invalid --principal: %w", err)
		}
	}

	// Balance owed: --current-balance, falling back to --principal (origination).
	var owed types.Money
	switch {
	case opts.currentBalance != "":
		owed, err = types.NewMoney(opts.currentBalance)
		if err != nil {
			return fmt.Errorf("invalid --current-balance: %w", err)
		}
	case havePrincipal:
		owed = principal
	default:
		return fmt.Errorf("a balance is required: pass --current-balance (what you owe today) or --principal")
	}
	if !owed.IsPositive() {
		return fmt.Errorf("balance must be positive, got %s", owed)
	}

	nextDate, err := types.ParseDate(opts.nextPaymentDate)
	if err != nil {
		return fmt.Errorf("invalid --next-payment-date: %w", err)
	}

	var openDate types.Date
	haveOpenDate := opts.openDate != ""
	if haveOpenDate {
		openDate, err = types.ParseDate(opts.openDate)
		if err != nil {
			return fmt.Errorf("invalid --open-date: %w", err)
		}
	}

	// P&I payment: --payment, else computed from --principal + --term-months.
	var payment types.Money
	computedPayment := false
	switch {
	case opts.payment != "":
		payment, err = types.NewMoney(opts.payment)
		if err != nil {
			return fmt.Errorf("invalid --payment: %w", err)
		}
		if !payment.IsPositive() {
			return fmt.Errorf("--payment must be positive, got %s", payment)
		}
	case havePrincipal && opts.termMonths > 0:
		payment, err = loandom.Payment(principal, apr, opts.termMonths)
		if err != nil {
			return fmt.Errorf("failed to compute payment: %w", err)
		}
		computedPayment = true
	default:
		return fmt.Errorf("--payment is required unless both --principal and --term-months are given (then it is computed)")
	}

	if cmd.Flags().Changed("lead-days") {
		if !opts.autoPost {
			return fmt.Errorf("--lead-days requires --auto-post")
		}
		if opts.leadDays != 0 && opts.leadDays != 3 && opts.leadDays != 7 {
			return fmt.Errorf("--lead-days must be 0, 3, or 7")
		}
	}

	var assetValue types.Money
	if opts.assetName != "" {
		if opts.assetValue == "" {
			return fmt.Errorf("--asset-value is required when --asset-name is given")
		}
		assetValue, err = types.NewMoney(opts.assetValue)
		if err != nil {
			return fmt.Errorf("invalid --asset-value: %w", err)
		}
	}

	// Negative-amortization guard — validated before opening the DB so a bad
	// payment fails fast without the auto-post side effect of OpenServices.
	if _, _, _, sErr := loandom.SplitPayment(owed, apr, payment); sErr != nil {
		if errors.Is(sErr, loandom.ErrNegativeAmortization) {
			return fmt.Errorf("payment %s does not cover the first month's interest on balance %s at %s%% APR", payment, owed, apr)
		}
		return sErr
	}

	database, svc, err := cmdutil.OpenServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Funding account must be an active, non-investment account.
	fundingAcct, err := svc.Account.GetByName(opts.fromAccount)
	if err != nil {
		return fmt.Errorf("funding account %q not found", opts.fromAccount)
	}
	if !fundingAcct.Active {
		return fmt.Errorf("funding account %q is closed", fundingAcct.Name)
	}
	if fundingAcct.Type.IsInvestmentType() {
		return fmt.Errorf("funding account %q is an investment account; loan payments must be funded from a bank account", fundingAcct.Name)
	}
	if _, err := svc.Account.GetByName(opts.name); err == nil {
		return fmt.Errorf("account %q already exists", opts.name)
	}
	currency := fundingAcct.Currency

	// Interest category (only when APR > 0): explicit path resolves an existing
	// category; the default Loan:Interest is get-or-created.
	interestCatID := types.NilID
	if apr.IsPositive() {
		if opts.interestCategory != "" {
			interestCatID, err = getOrCreateCategoryPath(svc, opts.interestCategory)
			if err != nil {
				return fmt.Errorf("--interest-category: %w", err)
			}
		} else {
			cat, cErr := svc.Category.GetOrCreateLoanInterestCategory()
			if cErr != nil {
				return fmt.Errorf("failed to resolve default interest category: %w", cErr)
			}
			interestCatID = cat.ID
		}
	}

	// Principal category: omitted → default Loan:Principal (get-or-created);
	// explicit path → resolved/created; explicit "" → left unlabeled. Applies at
	// any APR (a 0% loan still labels its principal line).
	principalCatID := types.NilID
	if cmd.Flags().Changed("principal-category") {
		if strings.TrimSpace(opts.principalCategory) != "" {
			principalCatID, err = getOrCreateCategoryPath(svc, opts.principalCategory)
			if err != nil {
				return fmt.Errorf("--principal-category: %w", err)
			}
		}
	} else {
		cat, cErr := svc.Category.GetOrCreateLoanPrincipalCategory()
		if cErr != nil {
			return fmt.Errorf("failed to resolve default principal category: %w", cErr)
		}
		principalCatID = cat.ID
	}

	escrow, err := parseEscrowLines(svc, opts.escrow)
	if err != nil {
		return err
	}

	// Payee get-or-created outside the atomic unit (a shared payee must not be
	// rolled back), mirroring the wizard.
	payeeID := types.NilID
	if opts.payee != "" {
		py, _, pErr := svc.Payee.GetOrCreate(opts.payee)
		if pErr != nil {
			return fmt.Errorf("failed to resolve payee: %w", pErr)
		}
		payeeID = py.ID
	}

	// Opening date: use --open-date only for a new loan at origination (balance
	// equals the original principal); otherwise today (mid-life snapshot).
	openingDate := types.Today()
	if haveOpenDate && havePrincipal && principal.Equal(owed) {
		openingDate = openDate
	}

	loanAcct := accountdom.NewAccount(opts.name, accountdom.TypeLoan, currency, owed.Neg(), openingDate)
	loanAcct.SetInterestRate(apr)
	if opts.institution != "" {
		loanAcct.SetInstitution(opts.institution)
	}

	schedule, _, bErr := scheduleddom.BuildLoanSchedule(fundingAcct.ID, nextDate, payeeID, opts.autoPost, scheduleddom.LoanSnapshotInput{
		LoanAccountID:  loanAcct.ID,
		APR:            apr,
		Owed:           owed,
		PIPayment:      payment,
		InterestCatID:  interestCatID,
		PrincipalCatID: principalCatID,
		Escrow:         escrow,
	})
	if bErr != nil {
		return fmt.Errorf("failed to build loan schedule: %w", bErr)
	}
	if cmd.Flags().Changed("lead-days") {
		schedule.SetPostLeadDays(opts.leadDays)
	}

	// Atomic creation: loan account → optional asset account → schedule. The
	// CompoundCommand rolls back earlier steps if a later one fails, so a failure
	// never strands an orphaned loan account swinging net worth.
	cmds := []undo.Command{undo.NewCreateAccountCommand(svc.Account, loanAcct)}
	var assetAcct *accountdom.Account
	if opts.assetName != "" {
		assetAcct = accountdom.NewAccount(opts.assetName, accountdom.TypeAsset, currency, assetValue, openingDate)
		cmds = append(cmds, undo.NewCreateAccountCommand(svc.Account, assetAcct))
	}
	cmds = append(cmds, undo.NewCreateScheduledTransactionCommand(svc.Scheduled, schedule))

	if err := undo.NewCompoundCommand("Create loan", cmds...).Execute(); err != nil {
		return fmt.Errorf("failed to create loan: %w", err)
	}

	printLoanCreated(w, loanAcct, assetAcct, schedule, payment, computedPayment)
	cmdutil.AutoBackupAfterModification(opts.file)
	return nil
}

// getOrCreateCategoryPath resolves a category by "Parent" or
// "Parent:Subcategory" path, creating any missing level (expense-classified)
// since there is no `category add` CLI command. This mirrors the TUI loan
// wizard's inline category creation and the Loan:Interest get-or-create default,
// so escrow and custom interest categories are usable from a pure-CLI workflow.
// The first ':' separates parent from subcategory (categories are a two-level
// hierarchy). Real lookup errors are surfaced rather than masked by a create.
func getOrCreateCategoryPath(svc *app.Services, path string) (types.ID, error) {
	parent, child, hasChild := strings.Cut(path, ":")
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return types.NilID, fmt.Errorf("empty category name in %q", path)
	}

	topID, err := getOrCreateCategory(svc, parent, nil)
	if err != nil {
		return types.NilID, err
	}
	if !hasChild {
		return topID, nil
	}

	child = strings.TrimSpace(child)
	if child == "" {
		return types.NilID, fmt.Errorf("invalid category path %q (missing subcategory name)", path)
	}
	return getOrCreateCategory(svc, child, &topID)
}

// getOrCreateCategory returns the ID of the category named name under parentID
// (nil for a top-level category), creating it (expense-classified) if no such
// category exists. A lookup failure other than not-found is surfaced.
func getOrCreateCategory(svc *app.Services, name string, parentID *types.ID) (types.ID, error) {
	cat, err := svc.CategoryRepo.GetByName(name, parentID)
	if err == nil {
		return cat.ID, nil
	}
	var notFound *dberrors.NotFoundError
	if !errors.As(err, &notFound) {
		return types.NilID, fmt.Errorf("failed to look up category %q: %w", name, err)
	}

	var newCat *categorydom.Category
	if parentID == nil {
		newCat = categorydom.NewCategory(name, categorydom.TypeExpense)
	} else {
		newCat = categorydom.NewSubcategory(name, *parentID, categorydom.TypeExpense)
	}
	if cErr := svc.Category.Create(newCat); cErr != nil {
		return types.NilID, fmt.Errorf("failed to create category %q: %w", name, cErr)
	}
	return newCat.ID, nil
}

// parseEscrowLines parses repeatable --escrow "Category=Amount" specs into
// resolved escrow lines. The amount is the tail after the last '='; the category
// must already exist.
func parseEscrowLines(svc *app.Services, specs []string) ([]scheduleddom.LoanEscrowLine, error) {
	var lines []scheduleddom.LoanEscrowLine
	for _, spec := range specs {
		eq := strings.LastIndex(spec, "=")
		if eq < 0 {
			return nil, fmt.Errorf("invalid --escrow %q: expected Category=Amount", spec)
		}
		catPath := strings.TrimSpace(spec[:eq])
		amtStr := strings.TrimSpace(spec[eq+1:])
		if catPath == "" || amtStr == "" {
			return nil, fmt.Errorf("invalid --escrow %q: expected Category=Amount", spec)
		}
		amt, err := types.NewMoney(amtStr)
		if err != nil {
			return nil, fmt.Errorf("invalid --escrow amount in %q: %w", spec, err)
		}
		if !amt.IsPositive() {
			return nil, fmt.Errorf("invalid --escrow amount in %q: amount must be positive", spec)
		}
		catID, err := getOrCreateCategoryPath(svc, catPath)
		if err != nil {
			return nil, fmt.Errorf("--escrow %q: %w", spec, err)
		}
		lines = append(lines, scheduleddom.LoanEscrowLine{CategoryID: catID, Amount: amt})
	}
	return lines, nil
}
