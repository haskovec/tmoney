package investment

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/haskovec/tmoney/internal/account"
	"github.com/haskovec/tmoney/internal/app"
	"github.com/haskovec/tmoney/internal/cli/cmdutil"
	investmentdom "github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/security"
	"github.com/haskovec/tmoney/internal/types"
)

// formatGainLoss formats a gain/loss value with percentage.
func formatGainLoss(gl types.Money, pct float64, currency string) string {
	s := cmdutil.FormatMoney(gl, currency)
	if pct < 0 {
		return fmt.Sprintf("%s (%.1f%%)", s, pct)
	}
	return fmt.Sprintf("%s (+%.1f%%)", s, pct)
}

// formatReturnPct renders a total-return percent. Nil means the holding
// has no deployed cost (e.g., shares received only via transfer) — render
// the placeholder rather than 0%.
func formatReturnPct(pct *float64) string {
	if pct == nil {
		return "—"
	}
	if *pct < 0 {
		return fmt.Sprintf("%.2f%%", *pct)
	}
	return fmt.Sprintf("+%.2f%%", *pct)
}

// formatFeesPaid renders a fees-paid amount stored as a positive magnitude.
// Fees are subtracted from total return, so display the value with a
// leading minus sign so the subtraction is visually obvious on the row.
func formatFeesPaid(fees types.Money, currency string) string {
	if fees.IsZero() {
		return cmdutil.FormatMoney(fees, currency)
	}
	return cmdutil.FormatMoney(fees.Neg(), currency)
}

// printPortfolioSummary prints the investment portfolio summary with holdings.
func printPortfolioSummary(w io.Writer, acct *account.Account, valuation *investmentdom.AccountValuation, securityMap map[types.ID]*security.Security) {
	fmt.Fprintf(w, "PORTFOLIO: %s\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("PORTFOLIO: ")+len(acct.Name)))
	fmt.Fprintln(w)

	open, closed := partitionHoldings(valuation.Holdings)

	// Holdings table
	fmt.Fprintln(w, "HOLDINGS")
	fmt.Fprintln(w, "--------")

	if len(open) == 0 {
		fmt.Fprintln(w, "(No holdings)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "Ticker\tName\tShares\tAvg Cost\tPrice\tCost Basis\tMarket Value\tUNREAL\tDIV\tREAL\tFEES\tTOTAL RETURN\tRET %")
		fmt.Fprintln(tw, "------\t----\t------\t--------\t-----\t----------\t------------\t------\t---\t----\t----\t------------\t-----")

		for _, h := range open {
			ticker := h.SecurityID.String()[:8]
			name := ""
			if sec, ok := securityMap[h.SecurityID]; ok {
				ticker = sec.Ticker
				name = sec.Name
			}

			priceStr := "N/A"
			if h.HasPricing {
				priceStr = cmdutil.FormatMoney(h.CurrentPrice, acct.Currency)
			}

			realStr := cmdutil.FormatMoney(h.RealizedGain, acct.Currency)
			if h.RealizedGainUnavailable {
				realStr = "unavailable"
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				ticker,
				name,
				h.Shares.String(),
				cmdutil.FormatMoney(h.AvgCost, acct.Currency),
				priceStr,
				cmdutil.FormatMoney(h.CostBasis, acct.Currency),
				cmdutil.FormatMoney(h.MarketValue, acct.Currency),
				cmdutil.FormatMoney(h.GainLoss, acct.Currency),
				cmdutil.FormatMoney(h.DividendsReceived, acct.Currency),
				realStr,
				formatFeesPaid(h.FeesPaid, acct.Currency),
				cmdutil.FormatMoney(h.TotalReturn, acct.Currency),
				formatReturnPct(h.TotalReturnPct),
			)
		}
		tw.Flush()
	}

	printClosedPositions(w, acct, closed, securityMap)

	printAccountTotals(w, acct, valuation)
}

// printPortfolioWithLots prints the portfolio with lot detail for each holding.
func printPortfolioWithLots(w io.Writer, acct *account.Account, valuation *investmentdom.AccountValuation, securityMap map[types.ID]*security.Security, svc *app.Services, asOf types.Date) {
	fmt.Fprintf(w, "PORTFOLIO: %s (with lots)\n", acct.Name)
	fmt.Fprintln(w, strings.Repeat("=", len("PORTFOLIO: ")+len(acct.Name)+len(" (with lots)")))
	fmt.Fprintln(w)

	open, closed := partitionHoldings(valuation.Holdings)

	if len(open) == 0 {
		fmt.Fprintln(w, "(No holdings)")
	} else {
		for _, h := range open {
			ticker := h.SecurityID.String()[:8]
			name := ""
			if sec, ok := securityMap[h.SecurityID]; ok {
				ticker = sec.Ticker
				name = sec.Name
			}

			fmt.Fprintf(w, "%s - %s\n", ticker, name)
			fmt.Fprintf(w, "  Shares: %s  Avg Cost: %s  Market Value: %s  Gain/Loss: %s\n",
				h.Shares.String(),
				cmdutil.FormatMoney(h.AvgCost, acct.Currency),
				cmdutil.FormatMoney(h.MarketValue, acct.Currency),
				formatGainLoss(h.GainLoss, h.GainPct, acct.Currency),
			)

			// Get lot details
			lots, err := svc.InvestmentValuation.GetLotDetail(acct.ID, h.SecurityID, asOf)
			if err != nil {
				fmt.Fprintf(w, "  (could not retrieve lot details: %v)\n", err)
			} else if len(lots) == 0 {
				fmt.Fprintln(w, "  (no lots)")
			} else {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "  Lot\tPurchase Date\tShares\tCost/Share\tCost Basis\tCurrent Value\tGain/Loss")
				fmt.Fprintln(tw, "  ---\t-------------\t------\t----------\t----------\t-------------\t---------")

				for _, ld := range lots {
					lotIDStr := ld.LotID.String()
					if len(lotIDStr) > 8 {
						lotIDStr = lotIDStr[:8]
					}

					fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
						lotIDStr,
						ld.PurchaseDate.String(),
						ld.Shares.String(),
						cmdutil.FormatMoney(ld.CostPerShare, acct.Currency),
						cmdutil.FormatMoney(ld.CostBasis, acct.Currency),
						cmdutil.FormatMoney(ld.CurrentValue, acct.Currency),
						formatGainLoss(ld.GainLoss, ld.GainPct, acct.Currency),
					)
				}
				tw.Flush()
			}
			fmt.Fprintln(w)
		}
	}

	printClosedPositions(w, acct, closed, securityMap)

	printAccountTotals(w, acct, valuation)
}

// printAccountTotals renders the account totals block beneath the holdings
// table, one row per total-return component in the order defined by the
// total-return spec. Total return % renders the "—" placeholder when
// TotalReturnPct is nil (no buys ever — denominator is zero).
func printAccountTotals(w io.Writer, acct *account.Account, valuation *investmentdom.AccountValuation) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Account totals")
	fmt.Fprintln(w, "--------------")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	row := func(label, value string) {
		fmt.Fprintf(tw, "  %s\t%s\n", label, value)
	}
	row("Market value", cmdutil.FormatMoney(valuation.MarketValue, acct.Currency))
	row("Cash", cmdutil.FormatMoney(valuation.CashBalance, acct.Currency))
	row("Total value", cmdutil.FormatMoney(valuation.TotalValue, acct.Currency))
	row("Cost basis (open)", cmdutil.FormatMoney(valuation.TotalCostBasis, acct.Currency))
	row("Unrealized gain", cmdutil.FormatMoney(valuation.TotalGainLoss, acct.Currency))
	realizedStr := cmdutil.FormatMoney(valuation.RealizedGain, acct.Currency)
	totalReturnStr := cmdutil.FormatMoney(valuation.TotalReturn, acct.Currency)
	totalReturnPctStr := formatReturnPct(valuation.TotalReturnPct)
	if valuation.AnyRealizedUnavailable {
		realizedStr += " (partial)"
		totalReturnStr += " (partial)"
		totalReturnPctStr += " (partial)"
	}
	row("Realized gain", realizedStr)
	row("Dividends received", cmdutil.FormatMoney(valuation.DividendsReceived, acct.Currency))
	row("Interest received", cmdutil.FormatMoney(valuation.InterestReceived, acct.Currency))
	row("Fees paid", formatFeesPaid(valuation.FeesPaid, acct.Currency))
	row("Total return", totalReturnStr)
	row("Total return %", totalReturnPctStr)
	tw.Flush()
}

// partitionHoldings splits holdings into open (still held) and closed
// (synthesized when ValuationOptions.IncludeClosed is true).
func partitionHoldings(holdings []investmentdom.Holding) (open, closed []investmentdom.Holding) {
	for _, h := range holdings {
		if h.IsClosed {
			closed = append(closed, h)
		} else {
			open = append(open, h)
		}
	}
	return open, closed
}

// printClosedPositions renders the Closed positions section: a tabwriter
// table with one row per fully-sold security showing the total-return
// components. Each closed holding has zero shares/market value but
// populated realized gain, dividends, fees, and total-return numbers.
func printClosedPositions(w io.Writer, acct *account.Account, closed []investmentdom.Holding, securityMap map[types.ID]*security.Security) {
	if len(closed) == 0 {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Closed positions (fully sold, total-return only)")
	fmt.Fprintln(w, "------------------------------------------------")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TICKER\tREALIZED\tDIV\tFEES\tTOTAL RETURN\tRET %")
	fmt.Fprintln(tw, "------\t--------\t---\t----\t------------\t-----")

	for _, h := range closed {
		ticker := h.SecurityID.String()[:8]
		if sec, ok := securityMap[h.SecurityID]; ok {
			ticker = sec.Ticker
		}

		realStr := cmdutil.FormatMoney(h.RealizedGain, acct.Currency)
		if h.RealizedGainUnavailable {
			realStr = "unavailable"
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			ticker,
			realStr,
			cmdutil.FormatMoney(h.DividendsReceived, acct.Currency),
			formatFeesPaid(h.FeesPaid, acct.Currency),
			cmdutil.FormatMoney(h.TotalReturn, acct.Currency),
			formatReturnPct(h.TotalReturnPct),
		)
	}
	tw.Flush()
}
