package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type DeviationDraft struct {
	DeviationID      string
	RuleID           string
	RootCause        string
	CorrectiveAction string
	ScopeRuleIDs     []string
}

type DeviationCoverage struct {
	MissingRuleIDs []string          `json:"missing_rule_ids"`
	ExtraRuleIDs   []string          `json:"extra_rule_ids"`
	Issues         []ValidationIssue `json:"issues"`
}

func ValidateDeviationCoverage(c *ExerciseCase, drafts []DeviationDraft) (DeviationCoverage, error) {
	coverage := DeviationCoverage{}
	failed := map[string]struct{}{}
	known := map[string]struct{}{}
	for _, rule := range c.Protocol.SequenceRules {
		known[rule.ID] = struct{}{}
	}
	for _, result := range c.Evaluations {
		if !result.Passed {
			failed[result.RuleID] = struct{}{}
		}
	}
	seen := map[string]int{}
	for i, draft := range drafts {
		row := i + 1
		draft.RuleID = strings.TrimSpace(draft.RuleID)
		if first, ok := seen[draft.RuleID]; ok {
			coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "duplicate_deviation_rule", Field: "deviations.rule_id", Row: row, Message: fmt.Sprintf("规则 %s 重复提交偏差（首次位于第 %d 项）", draft.RuleID, first)})
		} else {
			seen[draft.RuleID] = row
		}
		if _, ok := failed[draft.RuleID]; !ok {
			coverage.ExtraRuleIDs = append(coverage.ExtraRuleIDs, draft.RuleID)
		}
		if strings.TrimSpace(draft.DeviationID) == "" {
			coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "required", Field: "deviations.deviation_id", Row: row, Message: "偏差编号不能为空"})
		}
		if strings.TrimSpace(draft.RootCause) == "" {
			coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "required", Field: "deviations.root_cause", Row: row, Message: fmt.Sprintf("规则 %s 的原因不能为空", draft.RuleID)})
		}
		if strings.TrimSpace(draft.CorrectiveAction) == "" {
			coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "required", Field: "deviations.corrective_action", Row: row, Message: fmt.Sprintf("规则 %s 的纠正措施不能为空", draft.RuleID)})
		}
		scopeSeen := map[string]struct{}{}
		containsSelf := false
		for _, id := range draft.ScopeRuleIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, duplicate := scopeSeen[id]; duplicate {
				coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "duplicate_scope_rule", Field: "deviations.scope_rule_ids", Row: row, Message: fmt.Sprintf("规则 %s 的复演范围重复包含 %s", draft.RuleID, id)})
				continue
			}
			scopeSeen[id] = struct{}{}
			if id == draft.RuleID {
				containsSelf = true
			}
			if _, ok := known[id]; !ok {
				coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "unknown_scope_rule", Field: "deviations.scope_rule_ids", Row: row, Message: fmt.Sprintf("规则 %s 的复演范围包含未知规则 %s", draft.RuleID, id)})
			} else if _, ok := failed[id]; !ok {
				coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "passed_scope_rule", Field: "deviations.scope_rule_ids", Row: row, Message: fmt.Sprintf("规则 %s 的复演范围包含已合格规则 %s", draft.RuleID, id)})
			}
		}
		if len(scopeSeen) == 0 {
			coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "required", Field: "deviations.scope_rule_ids", Row: row, Message: fmt.Sprintf("规则 %s 的复演范围不能为空", draft.RuleID)})
		} else if !containsSelf {
			coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "scope_missing_owner", Field: "deviations.scope_rule_ids", Row: row, Message: fmt.Sprintf("规则 %s 的复演范围必须包含自身", draft.RuleID)})
		}
	}
	for id := range failed {
		if _, ok := seen[id]; !ok {
			coverage.MissingRuleIDs = append(coverage.MissingRuleIDs, id)
		}
	}
	sort.Strings(coverage.MissingRuleIDs)
	sort.Strings(coverage.ExtraRuleIDs)
	if len(coverage.MissingRuleIDs) > 0 {
		coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "missing_deviations", Field: "deviations", Message: "遗漏失败规则: " + strings.Join(coverage.MissingRuleIDs, ", ")})
	}
	if len(coverage.ExtraRuleIDs) > 0 {
		coverage.Issues = append(coverage.Issues, ValidationIssue{Code: "extra_deviations", Field: "deviations", Message: "多余规则: " + strings.Join(coverage.ExtraRuleIDs, ", ")})
	}
	if len(coverage.Issues) > 0 {
		return coverage, &ValidationError{Issues: coverage.Issues}
	}
	return coverage, nil
}

func (c *ExerciseCase) ApplyCorrections(drafts []DeviationDraft, now time.Time) error {
	if _, err := ValidateDeviationCoverage(c, drafts); err != nil {
		return err
	}
	if err := c.BeginCorrection(); err != nil {
		return err
	}
	results := map[string]RuleResult{}
	for _, result := range c.Evaluations {
		results[result.RuleID] = result
	}
	for _, draft := range drafts {
		scope := normalizeIDs(draft.ScopeRuleIDs)
		attempt := CorrectionAttempt{RootCause: strings.TrimSpace(draft.RootCause), CorrectiveAction: strings.TrimSpace(draft.CorrectiveAction), ScopeRuleIDs: scope, RecordedAt: now.UTC()}
		var deviation *Deviation
		for i := range c.Deviations {
			if c.Deviations[i].RuleID == draft.RuleID && c.Deviations[i].Status == "open" {
				deviation = &c.Deviations[i]
				break
			}
		}
		if deviation == nil {
			for _, existing := range c.Deviations {
				if existing.DeviationID == strings.TrimSpace(draft.DeviationID) {
					return fmt.Errorf("deviation_id 已存在: %s", draft.DeviationID)
				}
			}
			c.Deviations = append(c.Deviations, Deviation{DeviationID: strings.TrimSpace(draft.DeviationID), CaseID: c.CaseID, RuleID: draft.RuleID, FailureDetail: results[draft.RuleID].Detail, Status: "open"})
			deviation = &c.Deviations[len(c.Deviations)-1]
		}
		attempt.Attempt = len(deviation.Attempts) + 1
		deviation.RootCause = attempt.RootCause
		deviation.CorrectiveAction = attempt.CorrectiveAction
		deviation.ScopeRuleIDs = scope
		deviation.ReperformanceScope = strings.Join(scope, ",")
		deviation.FailureDetail = results[draft.RuleID].Detail
		deviation.Attempts = append(deviation.Attempts, attempt)
	}
	return c.OpenReperformance()
}

func (c *ExerciseCase) AllowedTargetRuleIDs() []string {
	allowed := map[string]struct{}{}
	for _, deviation := range c.Deviations {
		if deviation.Status != "open" {
			continue
		}
		for _, id := range deviation.ScopeRuleIDs {
			allowed[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(allowed))
	for id := range allowed {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (c *ExerciseCase) ValidateTargetCoverage(targets []string) error {
	want := c.AllowedTargetRuleIDs()
	got := normalizeIDs(targets)
	if strings.Join(want, "\x00") != strings.Join(got, "\x00") {
		return &ValidationError{Issues: []ValidationIssue{{Code: "target_scope_mismatch", Field: "target_rule_ids", Message: fmt.Sprintf("定向复演目标必须完整匹配核验范围；允许 %s，收到 %s", strings.Join(want, ", "), strings.Join(got, ", "))}}}
	}
	return nil
}

func (c *ExerciseCase) RecordReperformance(runID string, targets []string, results []RuleResult) error {
	resultByID := map[string]RuleResult{}
	for _, result := range results {
		resultByID[result.RuleID] = result
	}
	targetSet := map[string]struct{}{}
	for _, id := range targets {
		targetSet[id] = struct{}{}
	}
	for i := range c.Deviations {
		deviation := &c.Deviations[i]
		if deviation.Status != "open" {
			continue
		}
		if _, targeted := targetSet[deviation.RuleID]; !targeted {
			continue
		}
		result := resultByID[deviation.RuleID]
		if len(deviation.Attempts) > 0 {
			attempt := &deviation.Attempts[len(deviation.Attempts)-1]
			attempt.ReperformanceRunID = runID
			if !result.Passed {
				attempt.ReperformanceFailure = result.Detail
			}
		}
		if result.Passed {
			deviation.Status = "closed"
			deviation.ClosedByRunID = runID
		} else {
			deviation.FailureDetail = result.Detail
		}
	}
	return nil
}

func normalizeIDs(ids []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
