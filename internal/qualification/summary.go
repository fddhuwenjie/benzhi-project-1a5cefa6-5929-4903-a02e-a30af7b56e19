package qualification

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"encoding/json"
	"sort"
)

func StableSummary(results []domain.RuleResult) string {
	ordered := append([]domain.RuleResult(nil), results...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].RuleID < ordered[j].RuleID })
	b, _ := json.Marshal(ordered)
	return domain.Digest(json.RawMessage(b))
}
func Explain(results []domain.RuleResult) map[string]string {
	out := make(map[string]string, len(results))
	for _, r := range results {
		out[r.RuleID] = r.Detail
	}
	return out
}
