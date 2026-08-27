package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var allowedEventTypes = map[string]struct{}{
	"detected": {}, "started": {}, "opened": {}, "positioned": {}, "closed": {}, "confirmed": {},
}

var roleEventTypes = map[string]map[string]struct{}{
	"detector": {"detected": {}},
	"fan":      {"started": {}},
	"damper":   {"opened": {}, "positioned": {}},
	"door":     {"closed": {}},
	"pressure": {"confirmed": {}},
}

func EventTypesForRole(role string) []string {
	allowed := roleEventTypes[strings.ToLower(strings.TrimSpace(role))]
	out := make([]string, 0, len(allowed))
	for eventType := range allowed {
		out = append(out, eventType)
	}
	sort.Strings(out)
	return out
}

func NormalizeRun(run ExerciseRun) ExerciseRun {
	run.RunID = strings.TrimSpace(run.RunID)
	run.RunKind = strings.ToLower(strings.TrimSpace(run.RunKind))
	run.RecordedBy = strings.TrimSpace(run.RecordedBy)
	for i := range run.TargetRuleIDs {
		run.TargetRuleIDs[i] = strings.TrimSpace(run.TargetRuleIDs[i])
	}
	sort.Strings(run.TargetRuleIDs)
	for i := range run.Events {
		e := &run.Events[i]
		e.DeviceID = strings.TrimSpace(e.DeviceID)
		e.EventType = strings.ToLower(strings.TrimSpace(e.EventType))
		for j := range e.EvidenceRefs {
			e.EvidenceRefs[j] = NormalizeEvidence(e.EvidenceRefs[j])
		}
	}
	return run
}

func ValidateRun(p ProtocolBaseline, run ExerciseRun) error {
	run = NormalizeRun(run)
	issues := []ValidationIssue{}
	add := func(code, field string, row int, message string) {
		issues = append(issues, ValidationIssue{Code: code, Field: field, Row: row, Message: message})
	}
	if run.RunID == "" {
		add("required", "run_id", 0, "run_id不能为空")
	}
	if run.RunKind != "initial" && run.RunKind != "targeted" {
		add("invalid_run_kind", "run_kind", 0, "轮次类型必须是 initial 或 targeted")
	}
	if run.RecordedBy == "" {
		add("required", "recorded_by", 0, "记录人员不能为空")
	}
	if len(run.Events) < 2 {
		add("insufficient_events", "events", 0, "演练轮次至少需要两个事件")
	}
	devices := map[string]Device{}
	for _, device := range p.Devices {
		devices[device.ID] = device
	}
	kinds := map[string]struct{}{}
	for _, kind := range p.RequiredEvidenceKinds {
		kinds[strings.ToLower(kind)] = struct{}{}
	}
	previous := time.Time{}
	duplicates := map[string]int{}
	uriDigest := map[string]string{}
	digestURI := map[string]string{}
	for i, event := range run.Events {
		row := i + 1
		device, known := devices[event.DeviceID]
		if !known {
			add("unknown_device", "events.device_id", row, fmt.Sprintf("第 %d 项事件引用未知设备: %s", row, event.DeviceID))
		}
		if _, ok := allowedEventTypes[event.EventType]; !ok {
			add("invalid_event_type", "events.event_type", row, fmt.Sprintf("第 %d 项事件类型不合法: %s", row, event.EventType))
		} else if known {
			if allowed, constrained := roleEventTypes[device.Role]; constrained {
				if _, ok := allowed[event.EventType]; !ok {
					add("role_event_mismatch", "events.event_type", row, fmt.Sprintf("第 %d 项事件类型 %s 与设备 %s 的角色 %s 不匹配", row, event.EventType, event.DeviceID, device.Role))
				}
			}
		}
		if event.At.IsZero() {
			add("required", "events.at", row, fmt.Sprintf("第 %d 项事件时间不能为空", row))
		} else {
			if !previous.IsZero() && event.At.Before(previous) {
				add("non_monotonic_time", "events.at", row, fmt.Sprintf("第 %d 项事件时间早于前一项", row))
			}
			previous = event.At
			key := event.DeviceID + "|" + event.EventType + "|" + event.At.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)
			if first, ok := duplicates[key]; ok {
				add("duplicate_event", "events", row, fmt.Sprintf("第 %d 项事件与第 %d 项在设备、类型和毫秒时间上重复", row, first))
			} else {
				duplicates[key] = row
			}
		}
		for j, ref := range event.EvidenceRefs {
			if err := ValidateEvidence(ref, kinds); err != nil {
				add("invalid_evidence", fmt.Sprintf("events.evidence_refs.%d", j+1), row, fmt.Sprintf("第 %d 项事件的证据无效: %v", row, err))
				continue
			}
			ref = NormalizeEvidence(ref)
			if old, ok := uriDigest[ref.URI]; ok && old != ref.SHA256 {
				add("evidence_uri_conflict", "events.evidence_refs", row, fmt.Sprintf("第 %d 项事件的证据 URI 对应了不同摘要", row))
			} else {
				uriDigest[ref.URI] = ref.SHA256
			}
			if old, ok := digestURI[ref.SHA256]; ok && old != ref.URI {
				add("evidence_digest_conflict", "events.evidence_refs", row, fmt.Sprintf("第 %d 项事件的证据摘要对应了不同 URI", row))
			} else {
				digestURI[ref.SHA256] = ref.URI
			}
		}
	}
	if run.RunKind == "targeted" {
		if len(run.TargetRuleIDs) == 0 {
			add("required", "target_rule_ids", 0, "定向复演必须指定目标规则")
		}
		rules := RuleIDs(p)
		seen := map[string]int{}
		for i, id := range run.TargetRuleIDs {
			if _, ok := rules[id]; !ok {
				add("unknown_target_rule", "target_rule_ids", i+1, fmt.Sprintf("未知目标规则: %s", id))
			}
			if first, ok := seen[id]; ok {
				add("duplicate_target_rule", "target_rule_ids", i+1, fmt.Sprintf("目标规则重复: %s（首次位于第 %d 项）", id, first))
			} else {
				seen[id] = i + 1
			}
		}
	}
	if len(issues) > 0 {
		return &ValidationError{Issues: issues}
	}
	return nil
}
