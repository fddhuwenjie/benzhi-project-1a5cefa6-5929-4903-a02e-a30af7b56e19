package application

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/qualification"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Service struct {
	store             *store.Store
	evaluator         qualification.Evaluator
	coordinator       Coordinator
	precheckCacheKey  string
	precheckCacheBody []byte
}

func New(s *store.Store) *Service { return &Service{store: s} }
func (s *Service) Get(id string) (*domain.ExerciseCase, error) {
	snap, err := s.store.Load(id)
	if errors.Is(err, os.ErrNotExist) {
		return nil, &NotFoundError{ID: id}
	}
	if err != nil {
		return nil, err
	}
	return snap.Case, nil
}
func validateMeta(m Meta) error {
	if m.RequestID == "" {
		return errors.New("request_id不能为空")
	}
	if m.ExpectedRevision < 0 {
		return errors.New("expected_revision无效")
	}
	return nil
}
func result(c *domain.ExerciseCase, msg string) CommandResult {
	return CommandResult{CaseID: c.CaseID, Revision: c.Revision, Status: c.Status, Message: msg}
}
func (s *Service) execute(id string, meta Meta, command any, successStatus int, mutate func(*store.Snapshot) (CommandResult, error)) (CommandResult, error) {
	unlock := s.coordinator.lock(id)
	defer unlock()
	if err := validateMeta(meta); err != nil {
		return CommandResult{}, err
	}
	fp := store.Fingerprint(command)
	_, err := s.store.WithCase(id, func(snap *store.Snapshot) ([]byte, bool, error) {
		if old, ok := snap.Requests[meta.RequestID]; ok {
			if old.Fingerprint != fp {
				summary := old.Summary
				if summary == "" {
					summary = store.ResponseSummary(old.Body)
				}
				conflict := &ConflictError{Message: "request_id已用于不同请求", CaseID: id, ReceiptSummary: summary}
				if snap.Case != nil {
					conflict.CurrentRevision = snap.Case.Revision
					conflict.CurrentStatus = snap.Case.Status
				}
				return nil, false, conflict
			}
			return old.Body, false, nil
		}
		if snap.Case != nil && snap.Case.Revision != meta.ExpectedRevision {
			return nil, false, &ConflictError{Message: fmt.Sprintf("页面版本已过期，当前修订为 %d", snap.Case.Revision), CaseID: id, CurrentRevision: snap.Case.Revision, CurrentStatus: snap.Case.Status}
		}
		r, err := mutate(snap)
		if err != nil {
			return nil, false, err
		}
		b, err := json.Marshal(r)
		if err != nil {
			return nil, false, err
		}
		snap.Requests[meta.RequestID] = store.StoredResponse{Fingerprint: fp, Status: successStatus, Body: b, Summary: store.ResponseSummary(b)}
		event := domain.NewDomainEvent(fmt.Sprintf("%T", command), id, meta.RequestID, fp, meta.ExpectedRevision, r.Revision, b)
		event.BusinessSummary = r.Summary
		if review, ok := command.(ReviewCommand); ok {
			event.Decision = review.Decision
		}
		auditPayload, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return nil, false, marshalErr
		}
		return auditPayload, true, nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	snap, loadErr := s.store.Load(id)
	if loadErr != nil {
		return CommandResult{}, loadErr
	}
	stored, ok := snap.Requests[meta.RequestID]
	if !ok {
		return CommandResult{}, errors.New("幂等响应未持久化")
	}
	var out CommandResult
	if err = json.Unmarshal(stored.Body, &out); err != nil {
		return out, err
	}
	return out, nil
}

func protocolFromCommand(zones []string, devices []domain.Device, rules []domain.SequenceRule, evidenceKinds, participants []string) domain.ProtocolBaseline {
	return domain.ProtocolBaseline{Zones: zones, Devices: devices, SequenceRules: rules, RequiredEvidenceKinds: evidenceKinds, ParticipantIDs: participants, DeadlineRulesMS: map[string]int64{}}
}

func (s *Service) PrecheckProtocol(id string, cmd PrecheckProtocolCommand) (ProtocolPrecheckResult, error) {
	unlock := s.coordinator.lock(id)
	defer unlock()
	if err := validateMeta(cmd.Meta); err != nil {
		return ProtocolPrecheckResult{}, err
	}
	c, err := s.Get(id)
	if err != nil {
		return ProtocolPrecheckResult{}, err
	}
	if c.Status != domain.StatusDraft {
		return ProtocolPrecheckResult{}, errors.New("仅草稿案件可预检方案")
	}
	if c.Revision != cmd.ExpectedRevision {
		return ProtocolPrecheckResult{}, &ConflictError{Message: fmt.Sprintf("页面版本已过期，当前修订为 %d", c.Revision), CaseID: id, CurrentRevision: c.Revision, CurrentStatus: c.Status}
	}
	cacheKey := store.Fingerprint(struct {
		CaseID  string
		Command PrecheckProtocolCommand
	}{CaseID: id, Command: cmd})
	if s.precheckCacheKey == cacheKey {
		var cached ProtocolPrecheckResult
		if json.Unmarshal(s.precheckCacheBody, &cached) == nil {
			return cached, nil
		}
	}
	p := protocolFromCommand(cmd.Zones, cmd.Devices, cmd.Rules, cmd.RequiredEvidenceKinds, cmd.ParticipantIDs)
	check := domain.PrecheckProtocol(p, cmd.FrozenBy)
	out := ProtocolPrecheckResult{CaseID: id, Revision: c.Revision, Status: c.Status, Check: check}
	if body, marshalErr := json.Marshal(out); marshalErr == nil {
		s.precheckCacheBody = body
		s.precheckCacheKey = cacheKey
	}
	return out, nil
}
func (s *Service) Create(cmd CreateCaseCommand) (CommandResult, error) {
	return s.execute(cmd.CaseID, cmd.Meta, cmd, 201, func(snap *store.Snapshot) (CommandResult, error) {
		if snap.Case != nil {
			return CommandResult{}, &ConflictError{Message: "案件已存在"}
		}
		if cmd.CaseID == "" || cmd.BuildingName == "" || cmd.CreatedBy == "" {
			return CommandResult{}, errors.New("案件编号、建筑和创建人不能为空")
		}
		snap.Case = domain.NewCase(cmd.CaseID, cmd.BuildingName, cmd.CreatedBy, time.Now().UTC())
		return result(snap.Case, "案件已创建"), nil
	})
}
func (s *Service) Freeze(id string, cmd FreezeProtocolCommand) (CommandResult, error) {
	return s.execute(id, cmd.Meta, cmd, 200, func(snap *store.Snapshot) (CommandResult, error) {
		if snap.Case == nil {
			return CommandResult{}, &NotFoundError{ID: id}
		}
		p := protocolFromCommand(cmd.Zones, cmd.Devices, cmd.Rules, cmd.RequiredEvidenceKinds, cmd.ParticipantIDs)
		check := domain.PrecheckProtocol(p, cmd.FrozenBy)
		if cmd.PrecheckDigest == "" {
			return CommandResult{}, errors.New("冻结前必须先完成方案预检")
		}
		if !check.Valid {
			return CommandResult{}, &domain.ValidationError{Issues: check.Issues}
		}
		if cmd.PrecheckDigest != check.Summary.Digest {
			return CommandResult{}, &ConflictError{Message: "方案内容在预检后发生变化，请重新预检", CaseID: id, CurrentRevision: snap.Case.Revision, CurrentStatus: snap.Case.Status}
		}
		p = check.Normalized
		if err := snap.Case.FreezeProtocol(p, cmd.FrozenBy); err != nil {
			return CommandResult{}, err
		}
		if err := snap.Case.StartCollection(); err != nil {
			return CommandResult{}, err
		}
		out := result(snap.Case, "方案已冻结，开始采集")
		out.Summary = check.Summary.Digest
		return out, nil
	})
}
func (s *Service) SubmitRun(id string, cmd SubmitRunCommand) (CommandResult, error) {
	return s.execute(id, cmd.Meta, cmd, 200, func(snap *store.Snapshot) (CommandResult, error) {
		c := snap.Case
		if c == nil {
			return CommandResult{}, &NotFoundError{ID: id}
		}
		run := domain.NormalizeRun(domain.ExerciseRun{RunID: cmd.RunID, RunKind: cmd.RunKind, RecordedBy: cmd.RecordedBy, TargetRuleIDs: cmd.TargetRuleIDs, Events: cmd.Events})
		if len(run.Events) > 0 {
			run.StartedAt = run.Events[0].At
			run.CompletedAt = run.Events[len(run.Events)-1].At
		}
		if err := c.AddRun(run); err != nil {
			return CommandResult{}, err
		}
		results := s.evaluator.Evaluate(c.Protocol, c.Runs)
		c.Runs[len(c.Runs)-1].EvaluationDigest = qualification.StableSummary(results)
		if run.RunKind == "targeted" {
			if err := c.RecordReperformance(run.RunID, run.TargetRuleIDs, results); err != nil {
				return CommandResult{}, err
			}
		}
		c.SetEvaluation(results)
		return result(c, "轮次已提交并完成确定性评估"), nil
	})
}
func (s *Service) Correct(id string, cmd CorrectCommand) (CommandResult, error) {
	return s.execute(id, cmd.Meta, cmd, 200, func(snap *store.Snapshot) (CommandResult, error) {
		c := snap.Case
		if c == nil {
			return CommandResult{}, &NotFoundError{ID: id}
		}
		drafts := make([]domain.DeviationDraft, 0, len(cmd.Deviations))
		for _, in := range cmd.Deviations {
			scope := append([]string(nil), in.ScopeRuleIDs...)
			if len(scope) == 0 {
				for _, id := range strings.Split(in.ReperformanceScope, ",") {
					if id = strings.TrimSpace(id); id != "" {
						scope = append(scope, id)
					}
				}
			}
			drafts = append(drafts, domain.DeviationDraft{DeviationID: in.DeviationID, RuleID: in.RuleID, RootCause: in.RootCause, CorrectiveAction: in.CorrectiveAction, ScopeRuleIDs: scope})
		}
		if err := c.ApplyCorrections(drafts, time.Now().UTC()); err != nil {
			return CommandResult{}, err
		}
		return result(c, "偏差已登记，等待定向复演"), nil
	})
}
func (s *Service) Review(id string, cmd ReviewCommand) (CommandResult, error) {
	out, err := s.execute(id, cmd.Meta, cmd, 200, func(snap *store.Snapshot) (CommandResult, error) {
		c := snap.Case
		if c == nil {
			return CommandResult{}, &NotFoundError{ID: id}
		}
		readiness := qualification.BuildReviewReadiness(c, strings.TrimSpace(cmd.ReviewerID))
		if len(readiness.DutyConflicts) > 0 {
			return CommandResult{}, fmt.Errorf("复核职责冲突: %s", readiness.DutyConflicts[0].Detail)
		}
		if cmd.Decision == "approved" && !readiness.Ready {
			return CommandResult{}, errors.New("送审就绪清单未全部通过")
		}
		if cmd.Decision == "rejected" && strings.TrimSpace(cmd.Reason) == "" {
			return CommandResult{}, errors.New("拒绝必须填写理由")
		}
		if err := c.SubmitReview(); err != nil {
			return CommandResult{}, err
		}
		if err := c.ReviewDecision(cmd.ReviewerID, cmd.Decision, cmd.Reason); err != nil {
			return CommandResult{}, err
		}
		if cmd.Decision == "approved" {
			cert := &domain.QualificationCertificate{CertificateID: "cert-" + c.CaseID, CaseID: c.CaseID, ProtocolDigest: c.ProtocolDigest(), ResultDigest: c.ResultDigest(), EvidenceManifest: evidenceManifest(c), AuditHeadDigest: c.AuditHead, ApprovedBy: cmd.ReviewerID, IssuedAt: time.Now().UTC()}
			cert.CertificateDigest = cert.ComputeDigest()
			c.Certificate = cert
		}
		out := result(c, "复核结论已封存")
		out.Summary = readiness.ChecklistDigest
		return out, nil
	})
	if err == nil && out.Status == domain.StatusQualified {
		c, getErr := s.Get(id)
		if getErr != nil {
			return out, getErr
		}
		if saveErr := s.store.SaveCertificate(c.Certificate); saveErr != nil {
			return out, saveErr
		}
	}
	return out, err
}

func (s *Service) ReviewReadiness(id, reviewer string) (qualification.ReviewReadiness, error) {
	c, err := s.Get(id)
	if err != nil {
		return qualification.ReviewReadiness{}, err
	}
	return qualification.BuildReviewReadiness(c, strings.TrimSpace(reviewer)), nil
}

func (s *Service) Receipt(caseID, requestID string) (Receipt, error) {
	if strings.TrimSpace(requestID) == "" {
		return Receipt{}, errors.New("request_id不能为空")
	}
	response, ok, err := s.store.LookupRequest(caseID, requestID)
	if errors.Is(err, os.ErrNotExist) {
		return Receipt{}, &NotFoundError{ID: caseID}
	}
	if err != nil {
		return Receipt{}, err
	}
	out := Receipt{CaseID: caseID, RequestID: requestID, Processed: ok}
	if !ok {
		return out, nil
	}
	out.HTTPStatus = response.Status
	out.ResponseSummary = response.Summary
	var commandResult CommandResult
	if err := json.Unmarshal(response.Body, &commandResult); err == nil {
		out.Result = &commandResult
	}
	return out, nil
}
func (s *Service) AuditTimeline(id string) (AuditTimeline, error) {
	c, err := s.Get(id)
	if err != nil {
		return AuditTimeline{}, err
	}
	entries, err := s.store.Timeline(id)
	if err != nil {
		return AuditTimeline{}, err
	}
	return AuditTimeline{CaseID: id, HeadDigest: c.AuditHead, Entries: entries}, nil
}
