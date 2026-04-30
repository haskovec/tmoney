package imexport

import "sort"

// DistinctAccounts returns the unique non-empty source-account names found
// in a parse result, sorted alphabetically. The CSV parser populates
// ImportRecord.Account from the "Account" column when present (which is
// how Quicken Mac's "Register Transactions to CSV" export is structured —
// every account is in one file). For formats that don't carry an account
// column, all records will have an empty Account and this returns an
// empty slice.
func DistinctAccounts(parseResult *ParseResult) []string {
	if parseResult == nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, rec := range parseResult.Records {
		if rec.Account == "" {
			continue
		}
		seen[rec.Account] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// FilterByAccount returns a copy of parseResult containing only the
// records whose Account matches name (exact match, case-sensitive).
// Errors are passed through unchanged so any parse problems still
// surface to the user.
func FilterByAccount(parseResult *ParseResult, name string) *ParseResult {
	if parseResult == nil {
		return nil
	}
	filtered := &ParseResult{Errors: parseResult.Errors}
	for _, rec := range parseResult.Records {
		if rec.Account == name {
			filtered.Records = append(filtered.Records, rec)
		}
	}
	return filtered
}
