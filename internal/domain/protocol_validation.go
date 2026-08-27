package domain

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationIssue struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Row     int    `json:"row,omitempty"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []ValidationIssue `json:"issues"`
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "校验失败"
	}
	if len(e.Issues) == 1 {
		return e.Issues[0].Message
	}
	return fmt.Sprintf("发现 %d 项校验问题：%s", len(e.Issues), e.Issues[0].Message)
}

type ProtocolSummary struct {
	Digest                 string         `json:"digest"`
	ZoneCount              int            `json:"zone_count"`
	DeviceRoleDistribution map[string]int `json:"device_role_distribution"`
	RuleCount              int            `json:"rule_count"`
	RequiredEvidenceKinds  []string       `json:"required_evidence_kinds"`
}

type ProtocolPrecheck struct {
	Valid      bool              `json:"valid"`
	Issues     []ValidationIssue `json:"issues"`
	Normalized ProtocolBaseline  `json:"normalized"`
	Summary    ProtocolSummary   `json:"summary"`
}

func NormalizeProtocol(p ProtocolBaseline) ProtocolBaseline {
	return normalizeProtocol(p, true)
}

func normalizeProtocol(p ProtocolBaseline, stableOrder bool) ProtocolBaseline {
	out := ProtocolBaseline{
		Zones:                 append([]string(nil), p.Zones...),
		Devices:               append([]Device(nil), p.Devices...),
		SequenceRules:         append([]SequenceRule(nil), p.SequenceRules...),
		RequiredEvidenceKinds: append([]string(nil), p.RequiredEvidenceKinds...),
		ParticipantIDs:        append([]string(nil), p.ParticipantIDs...),
		DeadlineRulesMS:       map[string]int64{},
	}
	for i := range out.Zones {
		out.Zones[i] = strings.TrimSpace(out.Zones[i])
	}
	for i := range out.Devices {
		out.Devices[i].ID = strings.TrimSpace(out.Devices[i].ID)
		out.Devices[i].Zone = strings.TrimSpace(out.Devices[i].Zone)
		out.Devices[i].Role = strings.ToLower(strings.TrimSpace(out.Devices[i].Role))
	}
	for i := range out.SequenceRules {
		r := &out.SequenceRules[i]
		r.ID = strings.TrimSpace(r.ID)
		r.Name = strings.TrimSpace(r.Name)
		r.FromRole = strings.ToLower(strings.TrimSpace(r.FromRole))
		r.ToRole = strings.ToLower(strings.TrimSpace(r.ToRole))
		r.RequiredEvidence = strings.ToLower(strings.TrimSpace(r.RequiredEvidence))
		out.DeadlineRulesMS[r.ID] = r.MaxResponseMS
	}
	for i := range out.RequiredEvidenceKinds {
		out.RequiredEvidenceKinds[i] = strings.ToLower(strings.TrimSpace(out.RequiredEvidenceKinds[i]))
	}
	for i := range out.ParticipantIDs {
		out.ParticipantIDs[i] = strings.TrimSpace(out.ParticipantIDs[i])
	}
	if stableOrder {
		sort.Strings(out.Zones)
		sort.SliceStable(out.Devices, func(i, j int) bool {
			if out.Devices[i].ID == out.Devices[j].ID {
				return out.Devices[i].Zone < out.Devices[j].Zone
			}
			return out.Devices[i].ID < out.Devices[j].ID
		})
		sort.SliceStable(out.SequenceRules, func(i, j int) bool { return out.SequenceRules[i].ID < out.SequenceRules[j].ID })
		sort.Strings(out.RequiredEvidenceKinds)
		sort.Strings(out.ParticipantIDs)
	}
	return out
}

func PrecheckProtocol(p ProtocolBaseline, frozenBy string) ProtocolPrecheck {
	normalized := NormalizeProtocol(p)
	issues := validateNormalizedProtocol(normalizeProtocol(p, false), strings.TrimSpace(frozenBy))
	roles := map[string]int{}
	for _, device := range normalized.Devices {
		roles[device.Role]++
	}
	summary := ProtocolSummary{
		ZoneCount:              len(normalized.Zones),
		DeviceRoleDistribution: roles,
		RuleCount:              len(normalized.SequenceRules),
		RequiredEvidenceKinds:  append([]string(nil), normalized.RequiredEvidenceKinds...),
	}
	summary.Digest = Digest(struct {
		Protocol ProtocolBaseline `json:"protocol"`
		FrozenBy string           `json:"frozen_by"`
	}{normalized, strings.TrimSpace(frozenBy)})
	return ProtocolPrecheck{Valid: len(issues) == 0, Issues: issues, Normalized: normalized, Summary: summary}
}

func ValidateProtocol(p ProtocolBaseline, frozenBy string) error {
	check := PrecheckProtocol(p, frozenBy)
	if !check.Valid {
		return &ValidationError{Issues: check.Issues}
	}
	return nil
}

func validateNormalizedProtocol(p ProtocolBaseline, frozenBy string) []ValidationIssue {
	issues := []ValidationIssue{}
	add := func(code, field string, row int, message string) {
		issues = append(issues, ValidationIssue{Code: code, Field: field, Row: row, Message: message})
	}
	if frozenBy == "" {
		add("required", "frozen_by", 0, "冻结人员不能为空")
	}
	if len(p.Zones) == 0 {
		add("required", "zones", 0, "方案必须包含分区")
	}
	if len(p.Devices) == 0 {
		add("required", "devices", 0, "方案必须包含设备")
	}
	if len(p.SequenceRules) == 0 {
		add("required", "rules", 0, "方案必须包含顺序规则")
	}
	zones := map[string]int{}
	for i, zone := range p.Zones {
		if zone == "" {
			add("required", "zones", i+1, "分区名称不能为空")
		} else if first, exists := zones[zone]; exists {
			add("duplicate", "zones", i+1, fmt.Sprintf("分区重复: %s（首次位于第 %d 行）", zone, first))
		} else {
			zones[zone] = i + 1
		}
	}
	devices := map[string]int{}
	roles := map[string]struct{}{}
	for i, device := range p.Devices {
		row := i + 1
		if device.ID == "" {
			add("required", "devices.id", row, "设备编号不能为空")
		} else if first, ok := devices[device.ID]; ok {
			add("duplicate_device", "devices.id", row, fmt.Sprintf("设备编号重复: %s（首次位于第 %d 行）", device.ID, first))
		} else {
			devices[device.ID] = row
		}
		if device.Role == "" {
			add("required", "devices.role", row, fmt.Sprintf("设备 %s 的角色不能为空", device.ID))
		} else {
			roles[device.Role] = struct{}{}
		}
		if _, ok := zones[device.Zone]; !ok {
			add("unknown_zone", "devices.zone", row, fmt.Sprintf("设备 %s 引用了未知分区 %s", device.ID, device.Zone))
		}
	}
	evidenceKinds := map[string]int{}
	for i, kind := range p.RequiredEvidenceKinds {
		if kind == "" {
			add("required", "required_evidence_kinds", i+1, "证据种类不能为空")
		} else if first, ok := evidenceKinds[kind]; ok {
			add("duplicate", "required_evidence_kinds", i+1, fmt.Sprintf("证据种类重复: %s（首次位于第 %d 行）", kind, first))
		} else {
			evidenceKinds[kind] = i + 1
		}
	}
	rules := map[string]int{}
	for i, rule := range p.SequenceRules {
		row := i + 1
		if rule.ID == "" {
			add("required", "rules.id", row, "规则编号不能为空")
		} else if first, ok := rules[rule.ID]; ok {
			add("duplicate", "rules.id", row, fmt.Sprintf("规则编号重复: %s（首次位于第 %d 行）", rule.ID, first))
		} else {
			rules[rule.ID] = row
		}
		if _, ok := roles[rule.FromRole]; !ok {
			add("unknown_role", "rules.from_role", row, fmt.Sprintf("规则 %s 起点角色不存在", rule.ID))
		}
		if _, ok := roles[rule.ToRole]; !ok {
			add("unknown_role", "rules.to_role", row, fmt.Sprintf("规则 %s 终点角色不存在", rule.ID))
		}
		if rule.FromRole != "" && rule.FromRole == rule.ToRole {
			add("same_endpoint", "rules.to_role", row, fmt.Sprintf("规则 %s 起点和终点角色不能相同", rule.ID))
		}
		if rule.MaxResponseMS <= 0 {
			add("invalid_deadline", "rules.max_response_ms", row, fmt.Sprintf("规则 %s 响应时限必须大于零", rule.ID))
		}
		if rule.RequiredEvidence != "" {
			if _, ok := evidenceKinds[rule.RequiredEvidence]; !ok {
				add("unknown_evidence_kind", "rules.required_evidence", row, fmt.Sprintf("规则 %s 引用了未声明的证据种类 %s", rule.ID, rule.RequiredEvidence))
			}
		}
	}
	participants := map[string]int{}
	if len(p.ParticipantIDs) == 0 {
		add("required", "participant_ids", 0, "方案必须包含参与人员")
	}
	for i, participant := range p.ParticipantIDs {
		if participant == "" {
			add("required", "participant_ids", i+1, "参与人员不能为空")
		} else if first, ok := participants[participant]; ok {
			add("duplicate", "participant_ids", i+1, fmt.Sprintf("参与人员重复: %s（首次位于第 %d 行）", participant, first))
		} else {
			participants[participant] = i + 1
		}
	}
	if _, ok := participants[frozenBy]; ok && frozenBy != "" {
		add("duty_conflict", "frozen_by", 0, "方案冻结者不得列为演练参与人员")
	}
	return issues
}

func RuleIDs(p ProtocolBaseline) map[string]struct{} {
	out := make(map[string]struct{}, len(p.SequenceRules))
	for _, rule := range p.SequenceRules {
		out[rule.ID] = struct{}{}
	}
	return out
}
