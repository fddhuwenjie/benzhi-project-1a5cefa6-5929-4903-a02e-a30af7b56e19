package qualification

import "benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"

func AllPassed(results []domain.RuleResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}
