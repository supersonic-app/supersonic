package helpers

import (
	"sort"
	"strings"

	"github.com/deluan/sanitize"
	"github.com/supersonic-app/supersonic/backend/mediaprovider"
)

// name and terms should be pre-converted to the same case
func AllTermsMatch(name string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(name, t) {
			return false
		}
	}
	return true
}

func RankSearchResults(results []*mediaprovider.SearchResult, fullQuery string, queryTerms []string) {
	if len(queryTerms) == 0 || len(results) < 2 {
		return
	}

	sanitizeMemo := make(map[string]string, len(results))
	sanitized := func(s string) string {
		if x, ok := sanitizeMemo[s]; ok {
			return x
		}
		x := strings.ToLower(sanitize.Accents(s))
		sanitizeMemo[s] = x
		return x
	}

	sort.Slice(results, func(i, j int) bool {
		a, b := results[i], results[j]
		aName := sanitized(a.Name)
		bName := sanitized(b.Name)

		// Compare by entire query
		matchesA, matchesB := strings.Contains(aName, fullQuery), strings.Contains(bName, fullQuery)
		if matchesA && !matchesB {
			return true // item A has a direct match with the full query and B does not
		} else if matchesB && !matchesA {
			return false // item B matches but not A
		}

		// Compare by search query terms
		for _, term := range queryTerms {
			firstTermIdxA, firstTermIdxB := strings.Index(aName, term), strings.Index(bName, term)
			if firstTermIdxA >= 0 && firstTermIdxB < 0 {
				return true // item A has a direct match with the query term and B does not
			} else if firstTermIdxB >= 0 && firstTermIdxA < 0 {
				return false // item B matches but not A
			}

			if firstTermIdxA < firstTermIdxB {
				return true // item A matches the query term starting at an earlier position
			} else if firstTermIdxB < firstTermIdxA {
				return false // item B matches first
			}
		}
		// Defer to item type for priority order
		return a.Type < b.Type
	})
}
