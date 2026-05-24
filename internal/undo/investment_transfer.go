package undo

import (
	"fmt"

	"github.com/haskovec/tmoney/internal/investment"
	"github.com/haskovec/tmoney/internal/types"
)

// =============================================================================
// CreateInvestmentTransferCashCommand — inv→reg (cash leaves the investment account)
// =============================================================================

// CreateInvestmentTransferCashCommand wraps investment.Service.TransferCash so
// the create can be undone. Execute creates the pair (investment-side + linked
// regular-side); Undo deletes both legs via the service's existing cascade.
type CreateInvestmentTransferCashCommand struct {
	svc                 *investment.Service
	investmentAccountID types.ID
	regularAccountID    types.ID
	date                types.Date
	amount              types.Money
	memo                string
	result              *investment.CashTransferResult
}

// NewCreateInvestmentTransferCashCommand constructs the command. Amount must
// be positive — the underlying service enforces it. The cash leaves
// investmentAccountID and lands in regularAccountID.
func NewCreateInvestmentTransferCashCommand(svc *investment.Service, investmentAccountID, regularAccountID types.ID, date types.Date, amount types.Money, memo string) *CreateInvestmentTransferCashCommand {
	return &CreateInvestmentTransferCashCommand{
		svc:                 svc,
		investmentAccountID: investmentAccountID,
		regularAccountID:    regularAccountID,
		date:                date,
		amount:              amount,
		memo:                memo,
	}
}

func (c *CreateInvestmentTransferCashCommand) Execute() error {
	result, err := c.svc.TransferCash(c.investmentAccountID, c.regularAccountID, c.date, c.amount, c.memo)
	if err != nil {
		return err
	}
	c.result = result
	return nil
}

func (c *CreateInvestmentTransferCashCommand) Undo() error {
	if c.result == nil {
		return fmt.Errorf("CreateInvestmentTransferCashCommand: cannot undo before Execute")
	}
	return c.svc.DeleteTransaction(c.result.InvestmentTransaction.ID)
}

func (c *CreateInvestmentTransferCashCommand) Description() string {
	return "Create transfer"
}

// Result returns the cash-transfer pair created by Execute. Nil before Execute
// or if Execute failed.
func (c *CreateInvestmentTransferCashCommand) Result() *investment.CashTransferResult {
	return c.result
}

// =============================================================================
// CreateInvestmentDepositCommand — reg→inv (cash arrives at the investment account)
// =============================================================================

// CreateInvestmentDepositCommand wraps investment.Service.DepositFromAccount.
// Execute creates the pair (regular-side withdrawal + investment-side deposit);
// Undo deletes both legs.
type CreateInvestmentDepositCommand struct {
	svc                 *investment.Service
	investmentAccountID types.ID
	regularAccountID    types.ID
	date                types.Date
	amount              types.Money
	memo                string
	result              *investment.CashTransferResult
}

// NewCreateInvestmentDepositCommand constructs the command. Cash leaves
// regularAccountID and lands in investmentAccountID.
func NewCreateInvestmentDepositCommand(svc *investment.Service, investmentAccountID, regularAccountID types.ID, date types.Date, amount types.Money, memo string) *CreateInvestmentDepositCommand {
	return &CreateInvestmentDepositCommand{
		svc:                 svc,
		investmentAccountID: investmentAccountID,
		regularAccountID:    regularAccountID,
		date:                date,
		amount:              amount,
		memo:                memo,
	}
}

func (c *CreateInvestmentDepositCommand) Execute() error {
	result, err := c.svc.DepositFromAccount(c.investmentAccountID, c.regularAccountID, c.date, c.amount, c.memo)
	if err != nil {
		return err
	}
	c.result = result
	return nil
}

func (c *CreateInvestmentDepositCommand) Undo() error {
	if c.result == nil {
		return fmt.Errorf("CreateInvestmentDepositCommand: cannot undo before Execute")
	}
	return c.svc.DeleteTransaction(c.result.InvestmentTransaction.ID)
}

func (c *CreateInvestmentDepositCommand) Description() string {
	return "Create transfer"
}

// Result returns the cash-transfer pair created by Execute. Nil before Execute
// or if Execute failed.
func (c *CreateInvestmentDepositCommand) Result() *investment.CashTransferResult {
	return c.result
}

// =============================================================================
// CreateInvestmentToInvestmentTransferCommand — inv↔inv (e.g. IRA→IRA rollover)
// =============================================================================

// CreateInvestmentToInvestmentTransferCommand wraps
// investment.Service.TransferCashBetweenInvestments. Execute creates a pair of
// linked investment-side rows (one debit on the source, one credit on the
// destination); Undo deletes both legs via the service's cascade.
type CreateInvestmentToInvestmentTransferCommand struct {
	svc             *investment.Service
	sourceAccountID types.ID
	destAccountID   types.ID
	date            types.Date
	amount          types.Money
	memo            string
	result          *investment.InvestmentCashTransferResult
}

// NewCreateInvestmentToInvestmentTransferCommand constructs the command. Cash
// leaves sourceAccountID and lands in destAccountID; both must be investment
// accounts and they must differ — enforced by the underlying service.
func NewCreateInvestmentToInvestmentTransferCommand(svc *investment.Service, sourceAccountID, destAccountID types.ID, date types.Date, amount types.Money, memo string) *CreateInvestmentToInvestmentTransferCommand {
	return &CreateInvestmentToInvestmentTransferCommand{
		svc:             svc,
		sourceAccountID: sourceAccountID,
		destAccountID:   destAccountID,
		date:            date,
		amount:          amount,
		memo:            memo,
	}
}

func (c *CreateInvestmentToInvestmentTransferCommand) Execute() error {
	result, err := c.svc.TransferCashBetweenInvestments(c.sourceAccountID, c.destAccountID, c.date, c.amount, c.memo)
	if err != nil {
		return err
	}
	c.result = result
	return nil
}

func (c *CreateInvestmentToInvestmentTransferCommand) Undo() error {
	if c.result == nil {
		return fmt.Errorf("CreateInvestmentToInvestmentTransferCommand: cannot undo before Execute")
	}
	return c.svc.DeleteTransaction(c.result.SourceTransaction.ID)
}

func (c *CreateInvestmentToInvestmentTransferCommand) Description() string {
	return "Create transfer"
}

// Result returns the inv↔inv cash-transfer pair created by Execute. Nil before
// Execute or if Execute failed.
func (c *CreateInvestmentToInvestmentTransferCommand) Result() *investment.InvestmentCashTransferResult {
	return c.result
}
