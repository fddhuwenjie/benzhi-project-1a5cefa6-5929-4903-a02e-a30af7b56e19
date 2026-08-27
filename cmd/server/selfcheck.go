package main

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/application"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func postJSON(client *http.Client, url string, v any, out any) error {
	b, _ := json.Marshal(v)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s 返回 %d: %s", url, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, out)
}
func runSelfcheck(base string) error {
	client := &http.Client{Timeout: 4 * time.Second}
	var r application.CommandResult
	id := "SELF-CHECK"
	if err := postJSON(client, base+"/api/cases", application.CreateCaseCommand{Meta: application.Meta{RequestID: "req-create", ExpectedRevision: 0}, CaseID: id, BuildingName: "自检楼宇", CreatedBy: "recorder"}, &r); err != nil {
		return err
	}
	rule := domain.SequenceRule{ID: "R-FAN", Name: "探测至风机启动", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000, RequiredEvidence: "photo"}
	freeze := application.FreezeProtocolCommand{Meta: application.Meta{RequestID: "req-freeze", ExpectedRevision: r.Revision}, FrozenBy: "lead", Zones: []string{"A区"}, Devices: []domain.Device{{ID: "DET", Zone: "A区", Role: "detector"}, {ID: "FAN", Zone: "A区", Role: "fan"}}, Rules: []domain.SequenceRule{rule}, RequiredEvidenceKinds: []string{"photo"}, ParticipantIDs: []string{"recorder"}}
	precheck := application.PrecheckProtocolCommand{Meta: freeze.Meta, FrozenBy: freeze.FrozenBy, Zones: freeze.Zones, Devices: freeze.Devices, Rules: freeze.Rules, RequiredEvidenceKinds: freeze.RequiredEvidenceKinds, ParticipantIDs: freeze.ParticipantIDs}
	var checked application.ProtocolPrecheckResult
	if err := postJSON(client, base+"/api/cases/"+id+"/protocol/precheck", precheck, &checked); err != nil {
		return err
	}
	if !checked.Check.Valid {
		return fmt.Errorf("方案预检未通过: %v", checked.Check.Issues)
	}
	freeze.PrecheckDigest = checked.Check.Summary.Digest
	if err := postJSON(client, base+"/api/cases/"+id+"/freeze", freeze, &r); err != nil {
		return err
	}
	t := time.Now().UTC()
	ev := func(device, kind string, at time.Time) domain.Event {
		return domain.Event{DeviceID: device, EventType: kind, At: at, EvidenceRefs: []domain.EvidenceRef{{Kind: "photo", URI: "evidence://selfcheck", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}
	}
	first := application.SubmitRunCommand{Meta: application.Meta{RequestID: "req-run-1", ExpectedRevision: r.Revision}, RunID: "RUN-1", RunKind: "initial", RecordedBy: "recorder", Events: []domain.Event{ev("DET", "detected", t), ev("FAN", "started", t.Add(2*time.Second))}}
	if err := postJSON(client, base+"/api/cases/"+id+"/runs", first, &r); err != nil {
		return err
	}
	if r.Status != domain.StatusEvaluationFailed {
		return fmt.Errorf("预期初次评估失败，实际为 %s", r.Status)
	}
	correct := application.CorrectCommand{Meta: application.Meta{RequestID: "req-correct", ExpectedRevision: r.Revision}, Deviations: []application.DeviationInput{{DeviationID: "DEV-1", RuleID: "R-FAN", RootCause: "启动回路延迟", CorrectiveAction: "校正控制器参数", ReperformanceScope: "R-FAN"}}}
	if err := postJSON(client, base+"/api/cases/"+id+"/deviations", correct, &r); err != nil {
		return err
	}
	t = t.Add(10 * time.Second)
	reperform := application.SubmitRunCommand{Meta: application.Meta{RequestID: "req-run-2", ExpectedRevision: r.Revision}, RunID: "RUN-2", RunKind: "targeted", RecordedBy: "recorder", TargetRuleIDs: []string{"R-FAN"}, Events: []domain.Event{ev("DET", "detected", t), ev("FAN", "started", t.Add(500*time.Millisecond))}}
	if err := postJSON(client, base+"/api/cases/"+id+"/runs", reperform, &r); err != nil {
		return err
	}
	if r.Status != domain.StatusReviewPending {
		return fmt.Errorf("预期进入待复核，实际为 %s", r.Status)
	}
	review := application.ReviewCommand{Meta: application.Meta{RequestID: "req-review", ExpectedRevision: r.Revision}, ReviewerID: "reviewer", Decision: "approved"}
	if err := postJSON(client, base+"/api/cases/"+id+"/review", review, &r); err != nil {
		return err
	}
	resp, err := client.Get(base + "/api/cases/" + id + "/verify")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var verified application.Verification
	if err = json.NewDecoder(resp.Body).Decode(&verified); err != nil {
		return err
	}
	if resp.StatusCode != 200 || !verified.Valid {
		return fmt.Errorf("证书校验未通过: %s", verified.Message)
	}
	return nil
}
