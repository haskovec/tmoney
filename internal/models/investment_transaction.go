package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// InvestmentTransactionType represents the type of investment transaction.
type InvestmentTransactionType string

const (
	InvestmentTransactionTypeBuy              InvestmentTransactionType = "buy"
	InvestmentTransactionTypeSell             InvestmentTransactionType = "sell"
	InvestmentTransactionTypeDividend         InvestmentTransactionType = "dividend"
	InvestmentTransactionTypeReinvestDividend InvestmentTransactionType = "reinvest_dividend"
	InvestmentTransactionTypeFee              InvestmentTransactionType = "fee"
	InvestmentTransactionTypeFeeLiquidation   InvestmentTransactionType = "fee_liquidation"
	InvestmentTransactionTypeDeposit          InvestmentTransactionType = "deposit"
	InvestmentTransactionTypeWithdrawal       InvestmentTransactionType = "withdrawal"
	InvestmentTransactionTypeInterest         InvestmentTransactionType = "interest"
	InvestmentTransactionTypeTransferShares   InvestmentTransactionType = "transfer_shares"
	InvestmentTransactionTypeTransferCash     InvestmentTransactionType = "transfer_cash"
	InvestmentTransactionTypeExchange         InvestmentTransactionType = "exchange"
)

// AllInvestmentTransactionTypes returns all valid investment transaction types.
func AllInvestmentTransactionTypes() []InvestmentTransactionType {
	return []InvestmentTransactionType{
		InvestmentTransactionTypeBuy,
		InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend,
		InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFee,
		InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeDeposit,
		InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest,
		InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeTransferCash,
		InvestmentTransactionTypeExchange,
	}
}

// String returns the string representation of the InvestmentTransactionType.
func (itt InvestmentTransactionType) String() string {
	return string(itt)
}

// IsValid returns true if the InvestmentTransactionType is a valid type.
func (itt InvestmentTransactionType) IsValid() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend, InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFee, InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeDeposit, InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest, InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeTransferCash, InvestmentTransactionTypeExchange:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the investment transaction type.
func (itt InvestmentTransactionType) DisplayName() string {
	switch itt {
	case InvestmentTransactionTypeBuy:
		return "Buy"
	case InvestmentTransactionTypeSell:
		return "Sell"
	case InvestmentTransactionTypeDividend:
		return "Dividend"
	case InvestmentTransactionTypeReinvestDividend:
		return "Reinvest Dividend"
	case InvestmentTransactionTypeFee:
		return "Fee"
	case InvestmentTransactionTypeFeeLiquidation:
		return "Fee via Liquidation"
	case InvestmentTransactionTypeDeposit:
		return "Deposit"
	case InvestmentTransactionTypeWithdrawal:
		return "Withdrawal"
	case InvestmentTransactionTypeInterest:
		return "Interest"
	case InvestmentTransactionTypeTransferShares:
		return "Transfer Shares"
	case InvestmentTransactionTypeTransferCash:
		return "Transfer Cash"
	case InvestmentTransactionTypeExchange:
		return "Exchange"
	default:
		return string(itt)
	}
}

// RequiresSecurity returns true if this transaction type must reference a security.
func (itt InvestmentTransactionType) RequiresSecurity() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend, InvestmentTransactionTypeReinvestDividend,
		InvestmentTransactionTypeFeeLiquidation, InvestmentTransactionTypeTransferShares,
		InvestmentTransactionTypeExchange:
		return true
	}
	return false
}

// RequiresShares returns true if this transaction type must have a shares value.
func (itt InvestmentTransactionType) RequiresShares() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeReinvestDividend, InvestmentTransactionTypeFeeLiquidation,
		InvestmentTransactionTypeTransferShares, InvestmentTransactionTypeExchange:
		return true
	}
	return false
}

// AffectsCash returns true if this transaction type affects the account cash position.
func (itt InvestmentTransactionType) AffectsCash() bool {
	switch itt {
	case InvestmentTransactionTypeBuy, InvestmentTransactionTypeSell,
		InvestmentTransactionTypeDividend, InvestmentTransactionTypeFee,
		InvestmentTransactionTypeDeposit, InvestmentTransactionTypeWithdrawal,
		InvestmentTransactionTypeInterest, InvestmentTransactionTypeTransferCash:
		return true
	}
	return false
}

// ParseInvestmentTransactionType parses a string into an InvestmentTransactionType.
func ParseInvestmentTransactionType(s string) (InvestmentTransactionType, error) {
	itt := InvestmentTransactionType(strings.ToLower(s))
	if !itt.IsValid() {
		return "", fmt.Errorf("invalid investment transaction type: %q", s)
	}
	return itt, nil
}

// Value implements the driver.Valuer interface for database storage.
func (itt InvestmentTransactionType) Value() (driver.Value, error) {
	return string(itt), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (itt *InvestmentTransactionType) Scan(value any) error {
	if value == nil {
		*itt = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*itt = InvestmentTransactionType(v)
	case []byte:
		*itt = InvestmentTransactionType(string(v))
	default:
		return fmt.Errorf("unsupported type for InvestmentTransactionType: %T", value)
	}
	return nil
}
