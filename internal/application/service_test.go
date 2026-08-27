package application

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	"errors"
	"testing"
	"time"
)

func TestIdempotencyAndStaleRevision(t *testing.T) {
	repo, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo)
	cmd := CreateCaseCommand{Meta: Meta{RequestID: "same", ExpectedRevision: 0}, CaseID: "C", BuildingName: "楼宇", CreatedBy: "u"}
	first, err := svc.Create(cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.Create(cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first != replay {
		t.Fatalf("幂等响应不同: %#v %#v", first, replay)
	}
	cmd.BuildingName = "另一楼宇"
	if _, err = svc.Create(cmd); err == nil {
		t.Fatal("相同 request_id 的不同载荷未拒绝")
	}
	receipt, err := svc.Receipt("C", "same")
	if err != nil || !receipt.Processed || receipt.HTTPStatus != 201 || receipt.Result == nil {
		t.Fatalf("幂等回执不完整: %#v %v", receipt, err)
	}
	freeze := FreezeProtocolCommand{Meta: Meta{RequestID: "freeze", ExpectedRevision: 0}, FrozenBy: "lead", Zones: []string{"A"}, Devices: []domain.Device{{ID: "D", Zone: "A", Role: "detector"}, {ID: "F", Zone: "A", Role: "fan"}}, Rules: []domain.SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000}}}
	if _, err = svc.Freeze("C", freeze); err == nil {
		t.Fatal("陈旧修订未拒绝")
	}
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.CaseID != "C" || conflict.CurrentRevision != first.Revision || conflict.CurrentStatus != domain.StatusDraft {
		t.Fatalf("陈旧冲突缺少恢复字段: %#v", conflict)
	}
}

func TestFullQualificationFlow(t *testing.T) {
	repo, _ := store.New(t.TempDir())
	svc := New(repo)
	r, err := svc.Create(CreateCaseCommand{Meta: Meta{RequestID: "1"}, CaseID: "C", BuildingName: "楼宇", CreatedBy: "rec"})
	if err != nil {
		t.Fatal(err)
	}
	freeze := FreezeProtocolCommand{Meta: Meta{RequestID: "2", ExpectedRevision: r.Revision}, FrozenBy: "lead", Zones: []string{"A"}, Devices: []domain.Device{{ID: "D", Zone: "A", Role: "detector"}, {ID: "F", Zone: "A", Role: "fan"}}, Rules: []domain.SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000}}, ParticipantIDs: []string{"rec"}}
	check, err := svc.PrecheckProtocol("C", PrecheckProtocolCommand{Meta: freeze.Meta, FrozenBy: freeze.FrozenBy, Zones: freeze.Zones, Devices: freeze.Devices, Rules: freeze.Rules, ParticipantIDs: freeze.ParticipantIDs})
	if err != nil {
		t.Fatal(err)
	}
	freeze.PrecheckDigest = check.Check.Summary.Digest
	r, err = svc.Freeze("C", freeze)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	event := func(id, kind string, at time.Time) domain.Event {
		return domain.Event{DeviceID: id, EventType: kind, At: at}
	}
	r, err = svc.SubmitRun("C", SubmitRunCommand{Meta: Meta{RequestID: "3", ExpectedRevision: r.Revision}, RunID: "one", RunKind: "initial", RecordedBy: "rec", Events: []domain.Event{event("D", "detected", now), event("F", "started", now.Add(2*time.Second))}})
	if err != nil {
		t.Fatal(err)
	}
	r, err = svc.Correct("C", CorrectCommand{Meta: Meta{RequestID: "4", ExpectedRevision: r.Revision}, Deviations: []DeviationInput{{DeviationID: "dev", RuleID: "R", RootCause: "延迟", CorrectiveAction: "校正", ReperformanceScope: "R"}}})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	r, err = svc.SubmitRun("C", SubmitRunCommand{Meta: Meta{RequestID: "5", ExpectedRevision: r.Revision}, RunID: "two", RunKind: "targeted", RecordedBy: "rec", TargetRuleIDs: []string{"R"}, Events: []domain.Event{event("D", "detected", now), event("F", "started", now.Add(1500*time.Millisecond))}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.StatusEvaluationFailed {
		t.Fatalf("首次复演失败后状态错误: %s", r.Status)
	}
	r, err = svc.Correct("C", CorrectCommand{Meta: Meta{RequestID: "6", ExpectedRevision: r.Revision}, Deviations: []DeviationInput{{DeviationID: "dev", RuleID: "R", RootCause: "二次分析", CorrectiveAction: "更换控制模块", ScopeRuleIDs: []string{"R"}}}})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	r, err = svc.SubmitRun("C", SubmitRunCommand{Meta: Meta{RequestID: "7", ExpectedRevision: r.Revision}, RunID: "three", RunKind: "targeted", RecordedBy: "rec", TargetRuleIDs: []string{"R"}, Events: []domain.Event{event("D", "detected", now), event("F", "started", now.Add(500*time.Millisecond))}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := svc.Get("C")
	if err != nil || len(c.Deviations) != 1 || len(c.Deviations[0].Attempts) != 2 || c.Deviations[0].Attempts[0].ReperformanceFailure == "" {
		t.Fatalf("连续整改历史未追加: %#v %v", c.Deviations, err)
	}
	r, err = svc.Review("C", ReviewCommand{Meta: Meta{RequestID: "8", ExpectedRevision: r.Revision}, ReviewerID: "reviewer", Decision: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != domain.StatusQualified {
		t.Fatalf("终态错误: %s", r.Status)
	}
	v, err := svc.Verify("C")
	if err != nil || !v.Valid {
		t.Fatalf("校验失败: %#v %v", v, err)
	}
}
