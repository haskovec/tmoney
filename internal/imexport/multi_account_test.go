package imexport

import (
	"reflect"
	"testing"

	"github.com/haskovec/tmoney/internal/types"
)

func makeParseResult(accounts ...string) *ParseResult {
	pr := &ParseResult{}
	for i, a := range accounts {
		pr.Records = append(pr.Records, ImportRecord{
			Account:    a,
			Date:       types.MustParseDate("2024-01-01"),
			Amount:     types.MustNewMoney("1.00"),
			SourceLine: i + 1,
		})
	}
	return pr
}

func TestDistinctAccounts(t *testing.T) {
	cases := []struct {
		name string
		in   *ParseResult
		want []string
	}{
		{"nil parse result", nil, nil},
		{"empty", &ParseResult{}, []string{}},
		{"single account", makeParseResult("Checking"), []string{"Checking"}},
		{"duplicates collapse", makeParseResult("Checking", "Checking", "Savings"), []string{"Checking", "Savings"}},
		{"sorted output", makeParseResult("Visa", "Checking", "Savings"), []string{"Checking", "Savings", "Visa"}},
		{"empty Account ignored", makeParseResult("", "Checking", ""), []string{"Checking"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DistinctAccounts(c.in)
			// Treat both nil and empty slices as "no result" for the
			// nil/empty cases.
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("DistinctAccounts = %v, want %v", got, c.want)
			}
		})
	}
}

func TestFilterByAccount(t *testing.T) {
	t.Run("filters to one account", func(t *testing.T) {
		pr := makeParseResult("Checking", "Savings", "Checking", "Visa")
		got := FilterByAccount(pr, "Checking")
		if len(got.Records) != 2 {
			t.Errorf("got %d records, want 2", len(got.Records))
		}
		for _, r := range got.Records {
			if r.Account != "Checking" {
				t.Errorf("unexpected account %q in result", r.Account)
			}
		}
	})

	t.Run("preserves errors", func(t *testing.T) {
		pr := makeParseResult("Checking")
		pr.Errors = []ParseError{{Line: 1, Message: "boom"}}
		got := FilterByAccount(pr, "Checking")
		if len(got.Errors) != 1 {
			t.Errorf("expected 1 error preserved, got %d", len(got.Errors))
		}
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		if got := FilterByAccount(nil, "Anything"); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("no match returns empty records", func(t *testing.T) {
		pr := makeParseResult("Checking")
		got := FilterByAccount(pr, "Savings")
		if len(got.Records) != 0 {
			t.Errorf("expected 0 records, got %d", len(got.Records))
		}
	})
}
