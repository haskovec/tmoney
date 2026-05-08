package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/haskovec/tmoney/internal/category"
	"github.com/haskovec/tmoney/internal/scheduled"
	"github.com/haskovec/tmoney/internal/types"
)

// runAddScheduled creates a new scheduled transaction.
func runAddScheduled(opts *cliOptions, w io.Writer) error {
	if opts.file == "" {
		return fmt.Errorf("--add-scheduled requires --file to specify a database")
	}
	if opts.accountName == "" {
		return fmt.Errorf("--add-scheduled requires --account to specify an account")
	}
	if opts.stFrequency == "" {
		return fmt.Errorf("--add-scheduled requires --frequency to specify a frequency")
	}

	// Parse frequency
	frequency, err := scheduled.ParseFrequency(opts.stFrequency)
	if err != nil {
		validFreqs := []string{}
		for _, f := range scheduled.AllFrequencies() {
			validFreqs = append(validFreqs, string(f))
		}
		return fmt.Errorf("invalid --frequency %q: valid values are %s", opts.stFrequency, strings.Join(validFreqs, ", "))
	}

	// Parse start date (default to today)
	var startDate types.Date
	if opts.txDate != "" {
		startDate, err = types.ParseDate(opts.txDate)
		if err != nil {
			return fmt.Errorf("invalid --date: %w", err)
		}
	} else {
		startDate = types.Today()
	}

	database, svc, err := openServices(opts.file)
	if err != nil {
		return err
	}
	defer database.Close()

	// Get account by name
	acct, err := svc.Account.GetByName(opts.accountName)
	if err != nil {
		return fmt.Errorf("account %q not found", opts.accountName)
	}

	// Create scheduled transaction
	st := scheduled.NewTransaction(acct.ID, frequency, startDate)

	// Handle amount (optional - null means variable amount)
	if opts.txAmount != "" {
		amount, err := types.NewMoney(opts.txAmount)
		if err != nil {
			return fmt.Errorf("invalid --amount: %w", err)
		}
		st.SetAmount(amount)
	}

	// Handle payee
	var payeeName string
	if opts.txPayee != "" {
		py, _, err := svc.Payee.GetOrCreate(opts.txPayee)
		if err != nil {
			return fmt.Errorf("failed to resolve payee: %w", err)
		}
		st.SetPayee(py.ID)
		payeeName = py.Name
	}

	// Handle category
	var categoryName string
	if opts.txCategory != "" {
		cat, err := svc.CategoryRepo.GetByName(opts.txCategory, nil)
		if err != nil {
			categories, listErr := svc.CategoryRepo.List()
			if listErr != nil {
				return fmt.Errorf("category %q not found", opts.txCategory)
			}
			var found *category.Category
			for _, c := range categories {
				if c.Name == opts.txCategory {
					found = c
					break
				}
			}
			if found == nil {
				return fmt.Errorf("category %q not found", opts.txCategory)
			}
			cat = found
		}
		st.SetCategory(cat.ID)
		categoryName = cat.Name
	}

	// Handle memo
	if opts.txMemo != "" {
		st.SetMemo(opts.txMemo)
	}

	// Handle day of month
	if opts.stDay != "" {
		day, err := strconv.Atoi(opts.stDay)
		if err != nil {
			return fmt.Errorf("invalid --day: %w", err)
		}
		st.SetDayOfMonth(day)
	}

	// Handle occurrences
	if opts.stOccurrences != "" {
		occ, err := strconv.ParseInt(opts.stOccurrences, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid --occurrences: %w", err)
		}
		st.SetOccurrences(occ)
	}

	// Handle end date
	if opts.stEndDate != "" {
		endDate, err := types.ParseDate(opts.stEndDate)
		if err != nil {
			return fmt.Errorf("invalid --end-date: %w", err)
		}
		st.SetEndDate(endDate)
	}

	// Handle auto-post
	if opts.autoPost {
		st.SetAutoPost(true)
	}

	// Handle lead days
	if opts.leadDays != "" {
		days, err := strconv.Atoi(opts.leadDays)
		if err != nil {
			return fmt.Errorf("invalid --lead-days: %w", err)
		}
		if days != 0 && days != 3 && days != 7 {
			return fmt.Errorf("--lead-days must be 0, 3, or 7")
		}
		if !opts.autoPost {
			return fmt.Errorf("--lead-days requires --auto-post")
		}
		st.SetPostLeadDays(days)
	}

	// Save scheduled transaction
	if err := svc.Scheduled.Create(st); err != nil {
		return fmt.Errorf("failed to create scheduled transaction: %w", err)
	}

	// Print confirmation
	fmt.Fprintln(w, "Scheduled transaction created successfully!")
	fmt.Fprintf(w, "  Account:   %s\n", acct.Name)
	fmt.Fprintf(w, "  Frequency: %s\n", frequency.DisplayName())
	fmt.Fprintf(w, "  Next Date: %s\n", st.NextDate.String())
	if st.HasAmount() {
		fmt.Fprintf(w, "  Amount:    %s\n", formatMoney(st.Amount.Money, acct.Currency))
	} else {
		fmt.Fprintf(w, "  Amount:    Variable\n")
	}
	if payeeName != "" {
		fmt.Fprintf(w, "  Payee:     %s\n", payeeName)
	}
	if categoryName != "" {
		fmt.Fprintf(w, "  Category:  %s\n", categoryName)
	}
	if opts.txMemo != "" {
		fmt.Fprintf(w, "  Memo:      %s\n", opts.txMemo)
	}
	if st.AutoPost {
		if st.PostLeadDays > 0 {
			fmt.Fprintf(w, "  Auto-post: Yes (%d days early)\n", st.PostLeadDays)
		} else {
			fmt.Fprintf(w, "  Auto-post: Yes\n")
		}
	}

	autoBackupAfterModification(opts.file)
	return nil
}
