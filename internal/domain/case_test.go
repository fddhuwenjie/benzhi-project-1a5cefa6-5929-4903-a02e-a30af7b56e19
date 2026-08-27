package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func baseline() ProtocolBaseline {
	return ProtocolBaseline{Zones: []string{"A"}, Devices: []Device{{ID: "D", Zone: "A", Role: "detector"}, {ID: "F", Zone: "A", Role: "fan"}}, SequenceRules: []SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000}}, ParticipantIDs: []string{"recorder"}}
}

func TestProtocolPrecheckAggregatesIssuesAndStableDigest(t *testing.T) {
	invalid := ProtocolBaseline{
		Zones:                 []string{"A"},
		Devices:               []Device{{ID: "X", Zone: "A", Role: "detector"}, {ID: "X", Zone: "B", Role: "fan"}},
		SequenceRules:         []SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000, RequiredEvidence: "meter"}},
		RequiredEvidenceKinds: []string{"photo"}, ParticipantIDs: []string{"rec"},
	}
	check := PrecheckProtocol(invalid, "lead")
	if check.Valid || len(check.Issues) < 3 {
		t.Fatalf("预检未聚合问题: %#v", check.Issues)
	}
	codes := map[string]bool{}
	for _, issue := range check.Issues {
		codes[issue.Code] = true
	}
	if !codes["duplicate_device"] || !codes["unknown_zone"] || !codes["unknown_evidence_kind"] {
		t.Fatalf("缺少预期问题类型: %#v", check.Issues)
	}
	valid := baseline()
	reordered := valid
	reordered.Devices = []Device{valid.Devices[1], valid.Devices[0]}
	if PrecheckProtocol(valid, "lead").Summary.Digest != PrecheckProtocol(reordered, "lead").Summary.Digest {
		t.Fatal("相同方案的规范化摘要不稳定")
	}
}

func TestRunValidationAggregatesSemanticAndReferenceConflicts(t *testing.T) {
	p := baseline()
	now := time.Now().UTC()
	run := ExerciseRun{RunID: "RUN", RunKind: "initial", RecordedBy: "rec", Events: []Event{
		{DeviceID: "F", EventType: "detected", At: now, EvidenceRefs: []EvidenceRef{{Kind: "photo", URI: "evidence://one", SHA256: strings.Repeat("A", 64)}}},
		{DeviceID: "F", EventType: "detected", At: now, EvidenceRefs: []EvidenceRef{{Kind: "photo", URI: "evidence://one", SHA256: strings.Repeat("b", 64)}}},
	}}
	err := ValidateRun(p, run)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("预期结构化校验错误，实际 %v", err)
	}
	codes := map[string]bool{}
	for _, issue := range validation.Issues {
		codes[issue.Code] = true
	}
	if !codes["role_event_mismatch"] || !codes["duplicate_event"] || !codes["evidence_uri_conflict"] {
		t.Fatalf("未同时发现语义和重复问题: %#v", validation.Issues)
	}
}

func TestFreezeAndEventInvariants(t *testing.T) {
	c := NewCase("C", "楼宇", "recorder", time.Now())
	if err := c.FreezeProtocol(baseline(), "lead"); err != nil {
		t.Fatal(err)
	}
	if err := c.StartCollection(); err != nil {
		t.Fatal(err)
	}
	if err := c.AddRun(ExerciseRun{RunID: "R", Events: []Event{{DeviceID: "unknown", EventType: "started", At: time.Now()}}}); err == nil {
		t.Fatal("未知设备未被拒绝")
	}
	now := time.Now()
	if err := c.AddRun(ExerciseRun{RunID: "R", Events: []Event{{DeviceID: "D", EventType: "detected", At: now}, {DeviceID: "F", EventType: "started", At: now.Add(-time.Second)}}}); err == nil {
		t.Fatal("非单调事件未被拒绝")
	}
}

func TestSeparationOfDuties(t *testing.T) {
	c := NewCase("C", "楼宇", "recorder", time.Now())
	c.Status = StatusReviewPending
	if err := c.ReviewDecision("recorder", "approved", ""); err == nil {
		t.Fatal("记录员不应能批准")
	}
	c.FrozenBy = "lead"
	if err := c.ReviewDecision("lead", "approved", ""); err == nil {
		t.Fatal("冻结者不应能批准")
	}
	if err := c.ReviewDecision("reviewer", "approved", ""); err != nil {
		t.Fatal(err)
	}
}
