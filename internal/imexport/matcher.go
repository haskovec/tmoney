package imexport

import (
	"math"
	"strings"
	"unicode"

	"github.com/haskovec/tmoney/internal/transaction"
	"github.com/haskovec/tmoney/internal/types"
)

// MatchConfidence represents the confidence level of a match.
type MatchConfidence string

const (
	// MatchConfidenceHigh indicates a high-confidence automatic match.
	MatchConfidenceHigh MatchConfidence = "high"
	// MatchConfidenceLow indicates a low-confidence match requiring user review.
	MatchConfidenceLow MatchConfidence = "low"
	// MatchConfidenceNone indicates no match was found.
	MatchConfidenceNone MatchConfidence = "none"
)

// MatchResult represents the outcome of matching an import record against
// existing transactions.
type MatchResult struct {
	// ImportRecord is the original imported transaction.
	ImportRecord *ImportRecord

	// MatchedTransaction is the existing transaction that matched, or nil if no match.
	MatchedTransaction *transaction.Transaction

	// Confidence is the confidence level of the match.
	Confidence MatchConfidence

	// Score is the combined match score (0.0 to 1.0). Only meaningful when
	// Confidence is not MatchConfidenceNone.
	Score float64

	// DateScore is the date closeness component (0.0 to 1.0).
	DateScore float64

	// PayeeScore is the payee similarity component (0.0 to 1.0).
	PayeeScore float64

	// MatchedByFITID indicates the match was made via bank reference ID (FITID),
	// which overrides fuzzy matching.
	MatchedByFITID bool
}

// ExistingTransaction wraps a transaction with its payee name for matching.
type ExistingTransaction struct {
	Transaction     *transaction.Transaction
	PayeeName       string
	BankReferenceID string
}

// MatcherConfig holds configuration for the fuzzy matcher.
type MatcherConfig struct {
	// DateWindowDays is the maximum number of days apart for a date match.
	// Default: 7
	DateWindowDays int

	// HighConfidenceDateDays is the date threshold for high-confidence matches.
	// Default: 3
	HighConfidenceDateDays int

	// HighConfidencePayeeThreshold is the minimum payee similarity for
	// high-confidence matches. Default: 0.7
	HighConfidencePayeeThreshold float64
}

// DefaultMatcherConfig returns the default matcher configuration.
func DefaultMatcherConfig() MatcherConfig {
	return MatcherConfig{
		DateWindowDays:               7,
		HighConfidenceDateDays:       3,
		HighConfidencePayeeThreshold: 0.70,
	}
}

// Matcher performs fuzzy matching of import records against existing transactions.
type Matcher struct {
	config MatcherConfig
}

// NewMatcher creates a new Matcher with the given configuration.
func NewMatcher(config MatcherConfig) *Matcher {
	return &Matcher{config: config}
}

// NewDefaultMatcher creates a new Matcher with default configuration.
func NewDefaultMatcher() *Matcher {
	return NewMatcher(DefaultMatcherConfig())
}

// Match finds the best match for an import record among existing transactions.
func (m *Matcher) Match(record *ImportRecord, existing []ExistingTransaction) MatchResult {
	result := MatchResult{
		ImportRecord: record,
		Confidence:   MatchConfidenceNone,
	}

	if len(existing) == 0 {
		return result
	}

	// Phase 1: FITID matching (overrides fuzzy matching)
	if record.BankReferenceID != "" {
		for i := range existing {
			if existing[i].BankReferenceID != "" && existing[i].BankReferenceID == record.BankReferenceID {
				result.MatchedTransaction = existing[i].Transaction
				result.Confidence = MatchConfidenceHigh
				result.Score = 1.0
				result.DateScore = 1.0
				result.PayeeScore = 1.0
				result.MatchedByFITID = true
				return result
			}
		}
	}

	// Phase 2: Amount-exact matching (required first pass)
	var candidates []ExistingTransaction
	for i := range existing {
		if existing[i].Transaction.Amount.Equal(record.Amount) {
			candidates = append(candidates, existing[i])
		}
	}

	if len(candidates) == 0 {
		return result
	}

	// Phase 3: Score candidates by date closeness and payee similarity
	var bestScore float64
	var bestCandidate *ExistingTransaction
	var bestDateScore, bestPayeeScore float64

	for i := range candidates {
		dateScore := m.dateScore(record.Date, candidates[i].Transaction.Date)
		if dateScore == 0 {
			// Outside date window, skip
			continue
		}

		payeeScore := m.payeeScore(record.Payee, candidates[i].PayeeName)

		// Combined score: date is weighted 60%, payee 40%
		combined := dateScore*0.6 + payeeScore*0.4

		if combined > bestScore {
			bestScore = combined
			bestCandidate = &candidates[i]
			bestDateScore = dateScore
			bestPayeeScore = payeeScore
		}
	}

	if bestCandidate == nil {
		return result
	}

	result.MatchedTransaction = bestCandidate.Transaction
	result.Score = bestScore
	result.DateScore = bestDateScore
	result.PayeeScore = bestPayeeScore

	// Determine confidence level
	daysDiff := absDaysDiff(record.Date, bestCandidate.Transaction.Date)
	if daysDiff <= m.config.HighConfidenceDateDays && bestPayeeScore >= m.config.HighConfidencePayeeThreshold {
		result.Confidence = MatchConfidenceHigh
	} else {
		result.Confidence = MatchConfidenceLow
	}

	return result
}

// matchCandidate represents a potential match between an import record and an
// existing transaction, used during the greedy assignment phase.
type matchCandidate struct {
	recordIdx   int
	existingIdx int
	result      MatchResult
}

// MatchAll finds the best match for each import record against existing
// transactions. Each existing transaction can only be matched once (greedy,
// best-score-first assignment).
func (m *Matcher) MatchAll(records []ImportRecord, existing []ExistingTransaction) []MatchResult {
	results := make([]MatchResult, len(records))
	used := make(map[int]bool) // index into existing that has been matched

	var allCandidates []matchCandidate

	for ri := range records {
		// Build filtered existing list (excluding already-used by FITID)
		record := &records[ri]
		result := MatchResult{
			ImportRecord: record,
			Confidence:   MatchConfidenceNone,
		}

		// FITID matching first
		if record.BankReferenceID != "" {
			for ei := range existing {
				if used[ei] {
					continue
				}
				if existing[ei].BankReferenceID != "" && existing[ei].BankReferenceID == record.BankReferenceID {
					result.MatchedTransaction = existing[ei].Transaction
					result.Confidence = MatchConfidenceHigh
					result.Score = 1.0
					result.DateScore = 1.0
					result.PayeeScore = 1.0
					result.MatchedByFITID = true
					results[ri] = result
					used[ei] = true
					break
				}
			}
			if result.MatchedByFITID {
				continue
			}
		}

		// Fuzzy matching: find all amount-matched candidates
		for ei := range existing {
			if used[ei] {
				continue
			}
			if !existing[ei].Transaction.Amount.Equal(record.Amount) {
				continue
			}

			dateScore := m.dateScore(record.Date, existing[ei].Transaction.Date)
			if dateScore == 0 {
				continue
			}

			payeeScore := m.payeeScore(record.Payee, existing[ei].PayeeName)
			combined := dateScore*0.6 + payeeScore*0.4

			r := MatchResult{
				ImportRecord:       record,
				MatchedTransaction: existing[ei].Transaction,
				Score:              combined,
				DateScore:          dateScore,
				PayeeScore:         payeeScore,
			}

			daysDiff := absDaysDiff(record.Date, existing[ei].Transaction.Date)
			if daysDiff <= m.config.HighConfidenceDateDays && payeeScore >= m.config.HighConfidencePayeeThreshold {
				r.Confidence = MatchConfidenceHigh
			} else {
				r.Confidence = MatchConfidenceLow
			}

			allCandidates = append(allCandidates, matchCandidate{
				recordIdx:   ri,
				existingIdx: ei,
				result:      r,
			})
		}
	}

	// Sort candidates by score descending (greedy assignment)
	sortCandidates(allCandidates)

	// Assign best matches greedily
	usedRecords := make(map[int]bool)
	for _, c := range allCandidates {
		if usedRecords[c.recordIdx] || used[c.existingIdx] {
			continue
		}
		results[c.recordIdx] = c.result
		usedRecords[c.recordIdx] = true
		used[c.existingIdx] = true
	}

	// Fill in unmatched records
	for i := range results {
		if results[i].ImportRecord == nil {
			results[i] = MatchResult{
				ImportRecord: &records[i],
				Confidence:   MatchConfidenceNone,
			}
		}
	}

	return results
}

// sortCandidates sorts candidates by score descending using insertion sort
// (typically a small list).
func sortCandidates(candidates []matchCandidate) {
	for i := 1; i < len(candidates); i++ {
		key := candidates[i]
		j := i - 1
		for j >= 0 && candidates[j].result.Score < key.result.Score {
			candidates[j+1] = candidates[j]
			j--
		}
		candidates[j+1] = key
	}
}

// dateScore calculates the date closeness score (0.0 to 1.0).
// Returns 0 if outside the date window.
func (m *Matcher) dateScore(importDate, existingDate types.Date) float64 {
	days := absDaysDiff(importDate, existingDate)
	if days > m.config.DateWindowDays {
		return 0
	}
	if days == 0 {
		return 1.0
	}
	// Linear decay: 1.0 at 0 days, approaching 0 at DateWindowDays
	return 1.0 - float64(days)/float64(m.config.DateWindowDays+1)
}

// payeeScore calculates the payee name similarity score (0.0 to 1.0).
func (m *Matcher) payeeScore(importPayee, existingPayee string) float64 {
	if importPayee == "" || existingPayee == "" {
		if importPayee == "" && existingPayee == "" {
			return 1.0
		}
		return 0
	}
	return PayeeSimilarity(importPayee, existingPayee)
}

// absDaysDiff returns the absolute number of days between two dates.
func absDaysDiff(a, b types.Date) int {
	diff := a.Time().Sub(b.Time())
	days := int(math.Abs(diff.Hours() / 24))
	return days
}

// PayeeSimilarity computes the similarity between two payee names (0.0 to 1.0).
// It normalizes names and uses a combination of techniques:
// 1. Exact match after normalization → 1.0
// 2. One contains the other → 0.9
// 3. Bigram similarity (Dice coefficient)
func PayeeSimilarity(a, b string) float64 {
	na := normalizePayee(a)
	nb := normalizePayee(b)

	if na == nb {
		return 1.0
	}

	if na == "" || nb == "" {
		return 0
	}

	// Containment check
	if strings.Contains(na, nb) || strings.Contains(nb, na) {
		return 0.9
	}

	// Bigram similarity (Dice coefficient)
	return diceCoefficient(na, nb)
}

// normalizePayee normalizes a payee name for comparison:
// - lowercase
// - remove non-alphanumeric characters (except spaces)
// - collapse multiple spaces
// - trim
func normalizePayee(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// diceCoefficient computes the Sørensen–Dice coefficient between two strings
// using character bigrams.
func diceCoefficient(a, b string) float64 {
	bigramsA := bigrams(a)
	bigramsB := bigrams(b)

	if len(bigramsA) == 0 && len(bigramsB) == 0 {
		return 1.0
	}
	if len(bigramsA) == 0 || len(bigramsB) == 0 {
		return 0
	}

	// Count intersection
	intersection := 0
	bCounts := make(map[string]int)
	for _, bg := range bigramsB {
		bCounts[bg]++
	}
	for _, bg := range bigramsA {
		if bCounts[bg] > 0 {
			intersection++
			bCounts[bg]--
		}
	}

	return 2.0 * float64(intersection) / float64(len(bigramsA)+len(bigramsB))
}

// bigrams returns the character bigrams of a string.
func bigrams(s string) []string {
	runes := []rune(s)
	if len(runes) < 2 {
		return nil
	}
	result := make([]string, 0, len(runes)-1)
	for i := 0; i < len(runes)-1; i++ {
		result = append(result, string(runes[i:i+2]))
	}
	return result
}
