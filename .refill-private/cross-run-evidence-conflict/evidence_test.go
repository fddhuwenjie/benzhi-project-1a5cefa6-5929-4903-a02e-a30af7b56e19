package cross_run_evidence_conflict_test

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/application"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	"strings"
	"testing"
	"time"
)

func TestRejectsEvidenceIdentityConflictAcrossRuns(t *testing.T) {
	repo, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := application.New(repo)
	created, err := svc.Create(application.CreateCaseCommand{
		Meta: application.Meta{RequestID: "create"}, CaseID: "CASE", BuildingName: "测试楼宇", CreatedBy: "recorder",
	})
	if err != nil {
		t.Fatal(err)
	}
	freeze := application.FreezeProtocolCommand{
		Meta:                  application.Meta{RequestID: "freeze", ExpectedRevision: created.Revision},
		FrozenBy:              "lead",
		Zones:                 []string{"A区"},
		Devices:               []domain.Device{{ID: "DET", Zone: "A区", Role: "detector"}, {ID: "FAN", Zone: "A区", Role: "fan"}},
		Rules:                 []domain.SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000, RequiredEvidence: "photo"}},
		RequiredEvidenceKinds: []string{"photo"},
		ParticipantIDs:        []string{"recorder"},
	}
	checked, err := svc.PrecheckProtocol("CASE", application.PrecheckProtocolCommand{
		Meta: freeze.Meta, FrozenBy: freeze.FrozenBy, Zones: freeze.Zones, Devices: freeze.Devices, Rules: freeze.Rules,
		RequiredEvidenceKinds: freeze.RequiredEvidenceKinds, ParticipantIDs: freeze.ParticipantIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	freeze.PrecheckDigest = checked.Check.Summary.Digest
	state, err := svc.Freeze("CASE", freeze)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	refA := domain.EvidenceRef{Kind: "photo", URI: "evidence://shared", SHA256: strings.Repeat("a", 64)}
	state, err = svc.SubmitRun("CASE", application.SubmitRunCommand{
		Meta: application.Meta{RequestID: "initial", ExpectedRevision: state.Revision}, RunID: "RUN-1", RunKind: "initial", RecordedBy: "recorder",
		Events: []domain.Event{{DeviceID: "DET", EventType: "detected", At: now, EvidenceRefs: []domain.EvidenceRef{refA}}, {DeviceID: "FAN", EventType: "started", At: now.Add(2 * time.Second)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err = svc.Correct("CASE", application.CorrectCommand{
		Meta:       application.Meta{RequestID: "correct", ExpectedRevision: state.Revision},
		Deviations: []application.DeviationInput{{DeviationID: "DEV", RuleID: "R", RootCause: "响应超时", CorrectiveAction: "调整控制参数", ScopeRuleIDs: []string{"R"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	refB := domain.EvidenceRef{Kind: "photo", URI: refA.URI, SHA256: strings.Repeat("b", 64)}
	_, err = svc.SubmitRun("CASE", application.SubmitRunCommand{
		Meta: application.Meta{RequestID: "targeted", ExpectedRevision: state.Revision}, RunID: "RUN-2", RunKind: "targeted", RecordedBy: "recorder", TargetRuleIDs: []string{"R"},
		Events: []domain.Event{{DeviceID: "DET", EventType: "detected", At: now.Add(10 * time.Second), EvidenceRefs: []domain.EvidenceRef{refB}}, {DeviceID: "FAN", EventType: "started", At: now.Add(10500 * time.Millisecond)}},
	})
	if err == nil {
		t.Fatal("TestRejectsEvidenceIdentityConflictAcrossRuns: reused evidence URI with a different digest was accepted")
	}
}
