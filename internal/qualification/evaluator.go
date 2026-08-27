package qualification

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"fmt"
	"sort"
	"strings"
)

type Evaluator struct{}

func (Evaluator) Evaluate(p *domain.ProtocolBaseline, runs []domain.ExerciseRun) []domain.RuleResult {
	if p == nil {
		return nil
	}
	merged := map[string]domain.RuleResult{}
	for _, run := range runs {
		targeted := map[string]bool{}
		for _, id := range run.TargetRuleIDs {
			targeted[id] = true
		}
		for _, result := range evaluateEvents(p, run.RunID, run.Events) {
			if run.RunKind != "targeted" || targeted[result.RuleID] {
				merged[result.RuleID] = result
			}
		}
	}
	results := make([]domain.RuleResult, 0, len(merged))
	for _, result := range merged {
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].RuleID < results[j].RuleID })
	return results
}

func evaluateEvents(p *domain.ProtocolBaseline, runID string, all []domain.Event) []domain.RuleResult {
	all = append([]domain.Event(nil), all...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].At.Before(all[j].At) })
	deviceRoles := map[string]string{}
	for _, device := range p.Devices {
		deviceRoles[device.ID] = device.Role
	}
	byRole := map[string][]domain.Event{}
	for _, event := range all {
		byRole[deviceRoles[event.DeviceID]] = append(byRole[deviceRoles[event.DeviceID]], event)
	}
	results := make([]domain.RuleResult, 0, len(p.SequenceRules))
	for _, rule := range p.SequenceRules {
		from := byRole[rule.FromRole]
		to := byRole[rule.ToRole]
		result := domain.RuleResult{RuleID: rule.ID, Name: rule.Name}
		var start, end *domain.Event
		if len(from) == 0 {
			result.FailureCategory = "start_missing"
			result.Detail = "缺少起点事件"
			if len(to) > 0 {
				end = &to[0]
			}
		} else if len(to) == 0 {
			start = &from[0]
			result.FailureCategory = "end_missing"
			result.Detail = "缺少终点事件"
		} else {
			start = &from[0]
			for i := range to {
				if !to[i].At.Before(start.At) {
					end = &to[i]
					break
				}
			}
			if end == nil {
				end = &to[len(to)-1]
				result.FailureCategory = "end_before_start"
				result.Detail = "终点事件早于起点事件，顺序倒置"
			} else {
				result.ResponseMS = end.At.Sub(start.At).Milliseconds()
				result.MarginMS = rule.MaxResponseMS - result.ResponseMS
				result.LimitUsageRatio = float64(result.ResponseMS) / float64(rule.MaxResponseMS)
				result.Passed = result.ResponseMS <= rule.MaxResponseMS
				result.Detail = "响应 " + formatMS(result.ResponseMS) + "，限时 " + formatMS(rule.MaxResponseMS)
				if !result.Passed {
					result.FailureCategory = "response_timeout"
				}
				if rule.RequiredEvidence != "" && !hasEvidence(*start, *end, rule.RequiredEvidence) {
					result.Passed = false
					result.FailureCategory = "required_evidence_missing"
					result.Detail += "；缺少必需证据 " + rule.RequiredEvidence
				}
				result.CriticalPass = result.Passed && result.LimitUsageRatio >= 0.9
				if result.CriticalPass {
					result.Detail += "；临界合格"
				}
			}
		}
		for _, event := range []*domain.Event{start, end} {
			if event == nil {
				continue
			}
			stamp := event.DeviceID + "@" + event.At.UTC().Format("2006-01-02T15:04:05.000Z07:00")
			result.CandidateWindow = append(result.CandidateWindow, stamp)
			result.EvidenceSources = append(result.EvidenceSources, domain.RuleEventSource{RunID: runID, DeviceID: event.DeviceID, At: event.At, EvidenceRefs: append([]domain.EvidenceRef(nil), event.EvidenceRefs...)})
		}
		if start != nil && end != nil && !end.At.Before(start.At) {
			result.EvidenceWindow = append([]string(nil), result.CandidateWindow...)
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].RuleID < results[j].RuleID })
	return results
}

func hasEvidence(start, end domain.Event, kind string) bool {
	for _, event := range []domain.Event{start, end} {
		for _, ref := range event.EvidenceRefs {
			if strings.EqualFold(ref.Kind, kind) {
				return true
			}
		}
	}
	return false
}

func formatMS(ms int64) string { return fmt.Sprintf("%dms", ms) }
