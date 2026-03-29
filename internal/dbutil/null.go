package dbutil

import (
	"github.com/haskovec/tmoney/internal/types"
)

// NullString converts NullableString to a value for database insertion.
func NullString(ns types.NullableString) any {
	if ns.Valid {
		return ns.String
	}
	return nil
}

// NullMoney converts NullableMoney to a value for database insertion.
func NullMoney(nm types.NullableMoney) any {
	if nm.Valid {
		return nm.Money.String()
	}
	return nil
}

// NullID converts NullableID to a value for database insertion.
func NullID(nid types.NullableID) any {
	if nid.Valid {
		return nid.ID.String()
	}
	return nil
}

// NullTimestamp converts NullableTimestamp to a value for database insertion.
func NullTimestamp(nt types.NullableTimestamp) any {
	if nt.Valid {
		return nt.Timestamp.Time()
	}
	return nil
}

// NullDate converts NullableDate to a value for database insertion.
func NullDate(nd types.NullableDate) any {
	if !nd.Valid {
		return nil
	}
	return nd.Date.Time()
}

// NullInt converts NullableInt to a value for database insertion.
func NullInt(ni types.NullableInt) any {
	if !ni.Valid {
		return nil
	}
	return ni.Int64
}

// NullQuantity converts NullableQuantity to a value for database insertion.
func NullQuantity(nq types.NullableQuantity) any {
	if nq.Valid {
		return nq.Quantity.String()
	}
	return nil
}

// NullUUIDCast returns "CAST(? AS UUID)" if the NullableID is valid, or "?" if null.
// This is needed because DuckDB requires UUID casting for non-null UUID values.
func NullUUIDCast(nid types.NullableID) string {
	if nid.Valid {
		return "CAST(? AS UUID)"
	}
	return "?"
}
