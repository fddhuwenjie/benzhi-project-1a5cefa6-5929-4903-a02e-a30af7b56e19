package qualification

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"fmt"
	"sort"
)

type ChecklistItem struct {
	Code   string `json:"code"`
	Label  string `json:"label"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type DutyConflict struct {
	Role   string `json:"role"`
	RunID  string `json:"run_id,omitempty"`
	Detail string `json:"detail"`
}

type ReviewReadiness struct {
	Ready           bool            `json:"ready"`
	ReviewerID      string          `json:"reviewer_id"`
	Items           []ChecklistItem `json:"items"`
	DutyConflicts   []DutyConflict  `json:"duty_conflicts"`
	ChecklistDigest string          `json:"checklist_digest"`
}

func BuildReviewReadiness(c *domain.ExerciseCase, reviewer string) ReviewReadiness {
	readiness := ReviewReadiness{ReviewerID: reviewer}
	protocolOK := c.Protocol != nil && c.Protocol.BaselineDigest != "" && c.Protocol.BaselineDigest == domain.PrecheckProtocol(*c.Protocol, c.FrozenBy).Summary.Digest
	readiness.Items = append(readiness.Items, ChecklistItem{Code: "protocol_baseline", Label: "冻结方案摘要", Passed: protocolOK, Detail: digestDetail(c)})
	allRules := c.Protocol != nil && len(c.Evaluations) == len(c.Protocol.SequenceRules) && AllPassed(c.Evaluations)
	readiness.Items = append(readiness.Items, ChecklistItem{Code: "all_rules_passed", Label: "全部规则合格", Passed: allRules, Detail: fmt.Sprintf("%d 项规则结果", len(c.Evaluations))})
	closed := true
	for _, deviation := range c.Deviations {
		closed = closed && deviation.Status == "closed"
	}
	readiness.Items = append(readiness.Items, ChecklistItem{Code: "deviations_closed", Label: "全部偏差关闭", Passed: closed, Detail: fmt.Sprintf("%d 项偏差", len(c.Deviations))})
	covered := targetedCoverageComplete(c)
	readiness.Items = append(readiness.Items, ChecklistItem{Code: "targeted_coverage", Label: "目标复演覆盖", Passed: covered, Detail: "关闭偏差均可追溯到授权复演轮次"})
	evidenceOK, evidenceDetail := requiredEvidenceComplete(c)
	readiness.Items = append(readiness.Items, ChecklistItem{Code: "required_evidence", Label: "必需证据种类齐全", Passed: evidenceOK, Detail: evidenceDetail})
	if reviewer != "" {
		if reviewer == c.CreatedBy {
			readiness.DutyConflicts = append(readiness.DutyConflicts, DutyConflict{Role: "creator", Detail: "候选复核员是案件建档人"})
		}
		if reviewer == c.FrozenBy {
			readiness.DutyConflicts = append(readiness.DutyConflicts, DutyConflict{Role: "freezer", Detail: "候选复核员是方案冻结人"})
		}
		for _, run := range c.Runs {
			if reviewer == run.RecordedBy {
				readiness.DutyConflicts = append(readiness.DutyConflicts, DutyConflict{Role: "run_recorder", RunID: run.RunID, Detail: "候选复核员记录了轮次 " + run.RunID})
			}
		}
	}
	readiness.Ready = len(readiness.DutyConflicts) == 0
	for _, item := range readiness.Items {
		readiness.Ready = readiness.Ready && item.Passed
	}
	readiness.ChecklistDigest = domain.Digest(struct {
		Items         []ChecklistItem `json:"items"`
		DutyConflicts []DutyConflict  `json:"duty_conflicts"`
	}{readiness.Items, readiness.DutyConflicts})
	return readiness
}

func digestDetail(c *domain.ExerciseCase) string {
	if c.Protocol == nil {
		return "方案未冻结"
	}
	return c.Protocol.BaselineDigest
}

func targetedCoverageComplete(c *domain.ExerciseCase) bool {
	runs := map[string]domain.ExerciseRun{}
	for _, run := range c.Runs {
		runs[run.RunID] = run
	}
	for _, deviation := range c.Deviations {
		if deviation.Status != "closed" || deviation.ClosedByRunID == "" {
			return false
		}
		run, ok := runs[deviation.ClosedByRunID]
		if !ok || run.RunKind != "targeted" {
			return false
		}
		found := false
		for _, id := range run.TargetRuleIDs {
			found = found || id == deviation.RuleID
		}
		if !found {
			return false
		}
	}
	return true
}

func requiredEvidenceComplete(c *domain.ExerciseCase) (bool, string) {
	if c.Protocol == nil {
		return false, "方案未冻结"
	}
	have := map[string]struct{}{}
	for _, result := range c.Evaluations {
		if !result.Passed {
			continue
		}
		for _, source := range result.EvidenceSources {
			for _, ref := range source.EvidenceRefs {
				have[ref.Kind] = struct{}{}
			}
		}
	}
	missing := []string{}
	for _, kind := range c.Protocol.RequiredEvidenceKinds {
		if _, ok := have[kind]; !ok {
			missing = append(missing, kind)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return false, "缺少: " + fmt.Sprint(missing)
	}
	return true, fmt.Sprintf("%d 种证据齐全", len(c.Protocol.RequiredEvidenceKinds))
}
