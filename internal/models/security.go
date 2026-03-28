package models

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// SecurityType represents the type of financial security.
type SecurityType string

const (
	SecurityTypeStock      SecurityType = "stock"
	SecurityTypeETF        SecurityType = "etf"
	SecurityTypeMutualFund SecurityType = "mutual_fund"
	SecurityTypeOther      SecurityType = "other"
)

// AllSecurityTypes returns all valid security types.
func AllSecurityTypes() []SecurityType {
	return []SecurityType{
		SecurityTypeStock,
		SecurityTypeETF,
		SecurityTypeMutualFund,
		SecurityTypeOther,
	}
}

// String returns the string representation of the SecurityType.
func (st SecurityType) String() string {
	return string(st)
}

// IsValid returns true if the SecurityType is a valid type.
func (st SecurityType) IsValid() bool {
	switch st {
	case SecurityTypeStock, SecurityTypeETF, SecurityTypeMutualFund, SecurityTypeOther:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the security type.
func (st SecurityType) DisplayName() string {
	switch st {
	case SecurityTypeStock:
		return "Stock"
	case SecurityTypeETF:
		return "ETF"
	case SecurityTypeMutualFund:
		return "Mutual Fund"
	case SecurityTypeOther:
		return "Other"
	default:
		return string(st)
	}
}

// ParseSecurityType parses a string into a SecurityType.
func ParseSecurityType(s string) (SecurityType, error) {
	st := SecurityType(strings.ToLower(s))
	if !st.IsValid() {
		return "", fmt.Errorf("invalid security type: %q", s)
	}
	return st, nil
}

// Value implements the driver.Valuer interface for database storage.
func (st SecurityType) Value() (driver.Value, error) {
	return string(st), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (st *SecurityType) Scan(value any) error {
	if value == nil {
		*st = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*st = SecurityType(v)
	case []byte:
		*st = SecurityType(string(v))
	default:
		return fmt.Errorf("unsupported type for SecurityType: %T", value)
	}
	return nil
}

// AssetClass represents the asset classification of a security.
type AssetClass string

const (
	AssetClassLargeCapStock      AssetClass = "large_cap_stock"
	AssetClassSmallCapStock      AssetClass = "small_cap_stock"
	AssetClassInternationalStock AssetClass = "international_stock"
	AssetClassIndex              AssetClass = "index"
	AssetClassDomesticBond       AssetClass = "domestic_bond"
	AssetClassForeignBond        AssetClass = "foreign_bond"
	AssetClassCash               AssetClass = "cash"
	AssetClassCommodity          AssetClass = "commodity"
	AssetClassCrypto             AssetClass = "crypto"
	AssetClassAssetMixture       AssetClass = "asset_mixture"
	AssetClassUnclassified       AssetClass = "unclassified"
)

// AllAssetClasses returns all valid asset classes.
func AllAssetClasses() []AssetClass {
	return []AssetClass{
		AssetClassLargeCapStock,
		AssetClassSmallCapStock,
		AssetClassInternationalStock,
		AssetClassIndex,
		AssetClassDomesticBond,
		AssetClassForeignBond,
		AssetClassCash,
		AssetClassCommodity,
		AssetClassCrypto,
		AssetClassAssetMixture,
		AssetClassUnclassified,
	}
}

// String returns the string representation of the AssetClass.
func (ac AssetClass) String() string {
	return string(ac)
}

// IsValid returns true if the AssetClass is a valid class.
func (ac AssetClass) IsValid() bool {
	switch ac {
	case AssetClassLargeCapStock, AssetClassSmallCapStock, AssetClassInternationalStock,
		AssetClassIndex, AssetClassDomesticBond, AssetClassForeignBond,
		AssetClassCash, AssetClassCommodity, AssetClassCrypto,
		AssetClassAssetMixture, AssetClassUnclassified:
		return true
	}
	return false
}

// DisplayName returns a human-readable name for the asset class.
func (ac AssetClass) DisplayName() string {
	switch ac {
	case AssetClassLargeCapStock:
		return "Large Cap Stock"
	case AssetClassSmallCapStock:
		return "Small Cap Stock"
	case AssetClassInternationalStock:
		return "International Stock"
	case AssetClassIndex:
		return "Index"
	case AssetClassDomesticBond:
		return "Domestic Bond"
	case AssetClassForeignBond:
		return "Foreign Bond"
	case AssetClassCash:
		return "Cash"
	case AssetClassCommodity:
		return "Commodity"
	case AssetClassCrypto:
		return "Crypto"
	case AssetClassAssetMixture:
		return "Asset Mixture"
	case AssetClassUnclassified:
		return "Unclassified"
	default:
		return string(ac)
	}
}

// ParseAssetClass parses a string into an AssetClass.
func ParseAssetClass(s string) (AssetClass, error) {
	ac := AssetClass(strings.ToLower(s))
	if !ac.IsValid() {
		return "", fmt.Errorf("invalid asset class: %q", s)
	}
	return ac, nil
}

// Value implements the driver.Valuer interface for database storage.
func (ac AssetClass) Value() (driver.Value, error) {
	return string(ac), nil
}

// Scan implements the sql.Scanner interface for database retrieval.
func (ac *AssetClass) Scan(value any) error {
	if value == nil {
		*ac = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*ac = AssetClass(v)
	case []byte:
		*ac = AssetClass(string(v))
	default:
		return fmt.Errorf("unsupported type for AssetClass: %T", value)
	}
	return nil
}

// Security represents a financial instrument in the security master.
type Security struct {
	BaseModel

	Ticker       string         `json:"ticker"`
	Name         string         `json:"name"`
	SecurityType SecurityType   `json:"security_type"`
	AssetClass   AssetClass     `json:"asset_class"`
	Currency     string         `json:"currency"`
	Exchange     NullableString `json:"exchange"`
	Hidden       bool           `json:"hidden"`
}

// NewSecurity creates a new Security with required fields and sensible defaults.
func NewSecurity(ticker, name string, securityType SecurityType) *Security {
	return &Security{
		BaseModel:    NewBaseModel(),
		Ticker:       ticker,
		Name:         name,
		SecurityType: securityType,
		AssetClass:   AssetClassUnclassified,
		Currency:     "USD",
		Hidden:       false,
	}
}

// Validate validates the Security and returns any validation errors.
func (s *Security) Validate() ValidationErrors {
	v := NewValidator()

	v.RequiredString("ticker", s.Ticker)
	v.MaxLength("ticker", s.Ticker, 20)
	v.RequiredString("name", s.Name)
	v.RequiredString("currency", s.Currency)
	v.Currency("currency", s.Currency)

	if !s.SecurityType.IsValid() {
		v.errors.Add("security_type", "must be a valid security type")
	}

	if !s.AssetClass.IsValid() {
		v.errors.Add("asset_class", "must be a valid asset class")
	}

	return v.Errors()
}

// IsValid returns true if the Security passes all validation rules.
func (s *Security) IsValid() bool {
	return !s.Validate().HasErrors()
}

// SetExchange sets the exchange field, clearing it if the value is empty.
func (s *Security) SetExchange(exchange string) {
	if exchange == "" {
		s.Exchange = NullableString{Valid: false}
	} else {
		s.Exchange = NullableString{String: exchange, Valid: true}
	}
}

// CanHide returns true if the security can be hidden.
// Placeholder: returns true when no positions exist. Will be enhanced after positions are built.
func (s *Security) CanHide() bool {
	return true
}

// CanDelete returns true if the security can be deleted.
// Placeholder: returns true. Will be enhanced after transaction/price history checks are built.
func (s *Security) CanDelete() bool {
	return true
}

// Hide marks the security as hidden and updates the timestamp.
func (s *Security) Hide() {
	s.Hidden = true
	s.Touch()
}

// Unhide marks the security as visible and updates the timestamp.
func (s *Security) Unhide() {
	s.Hidden = false
	s.Touch()
}
