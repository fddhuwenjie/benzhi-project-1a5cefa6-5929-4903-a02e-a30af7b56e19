package domain

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft            Status = "draft"
	StatusProtocolFrozen   Status = "protocol_frozen"
	StatusCollecting       Status = "collecting"
	StatusEvaluationFailed Status = "evaluation_failed"
	StatusCorrecting       Status = "correcting"
	StatusReperforming     Status = "reperforming"
	StatusReviewPending    Status = "review_pending"
	StatusQualified        Status = "qualified"
	StatusRejected         Status = "rejected"
)

type Device struct{ ID, Zone, Role string }
type SequenceRule struct {
	ID, Name, FromRole, ToRole string
	MaxResponseMS              int64
	RequiredEvidence           string
}
type ProtocolBaseline struct {
	ProtocolID, CaseID    string
	BaselineDigest        string
	Zones                 []string
	Devices               []Device
	SequenceRules         []SequenceRule
	DeadlineRulesMS       map[string]int64
	RequiredEvidenceKinds []string
	ParticipantIDs        []string
	FrozenAt              time.Time
}
type ExerciseCase struct {
	CaseID, BuildingName string
	Status               Status
	ProtocolRevision     int
	CreatedBy, FrozenBy  string
	Revision             int64
	CreatedAt, UpdatedAt time.Time
	Protocol             *ProtocolBaseline
	Runs                 []ExerciseRun
	Deviations           []Deviation
	Evaluations          []RuleResult
	AuditHead            string
	Certificate          *QualificationCertificate
	Review               *Review
}
type Event struct {
	DeviceID, EventType string
	At                  time.Time
	EvidenceRefs        []EvidenceRef
}
type EvidenceRef struct{ Kind, URI, SHA256 string }
type ExerciseRun struct {
	RunID, CaseID, RunKind string
	TargetRuleIDs          []string
	RecordedBy             string
	StartedAt, CompletedAt time.Time
	Events                 []Event
	EvidenceRefs           []EvidenceRef
	EvaluationDigest       string
}
type Deviation struct {
	DeviationID, CaseID, RuleID, FailureDetail, RootCause, CorrectiveAction, ReperformanceScope, Status, ClosedByRunID string
	ScopeRuleIDs                                                                                                       []string            `json:"ScopeRuleIDs,omitempty"`
	Attempts                                                                                                           []CorrectionAttempt `json:"Attempts,omitempty"`
}
type CorrectionAttempt struct {
	Attempt              int       `json:"attempt"`
	RootCause            string    `json:"root_cause"`
	CorrectiveAction     string    `json:"corrective_action"`
	ScopeRuleIDs         []string  `json:"scope_rule_ids"`
	RecordedAt           time.Time `json:"recorded_at"`
	ReperformanceRunID   string    `json:"reperformance_run_id,omitempty"`
	ReperformanceFailure string    `json:"reperformance_failure,omitempty"`
}
type RuleResult struct {
	RuleID, Name    string
	Passed          bool
	Detail          string
	EvidenceWindow  []string
	ResponseMS      int64
	FailureCategory string            `json:"FailureCategory,omitempty"`
	MarginMS        int64             `json:"MarginMS"`
	LimitUsageRatio float64           `json:"LimitUsageRatio"`
	CriticalPass    bool              `json:"CriticalPass"`
	CandidateWindow []string          `json:"CandidateWindow,omitempty"`
	EvidenceSources []RuleEventSource `json:"EvidenceSources,omitempty"`
}
type RuleEventSource struct {
	RunID        string        `json:"RunID"`
	DeviceID     string        `json:"DeviceID"`
	At           time.Time     `json:"At"`
	EvidenceRefs []EvidenceRef `json:"EvidenceRefs,omitempty"`
}
type Review struct {
	ReviewerID, Decision, Reason string
	At                           time.Time
}
type QualificationCertificate struct {
	CertificateID, CaseID, ProtocolDigest, ResultDigest string
	EvidenceManifest                                    []EvidenceRef
	AuditHeadDigest, ApprovedBy                         string
	IssuedAt                                            time.Time
	CertificateDigest                                   string
}

func NewCase(id, building, creator string, now time.Time) *ExerciseCase {
	return &ExerciseCase{CaseID: id, BuildingName: building, Status: StatusDraft, CreatedBy: creator, Revision: 1, CreatedAt: now, UpdatedAt: now}
}
func (c *ExerciseCase) touch() { c.Revision++; c.UpdatedAt = time.Now().UTC() }
func (c *ExerciseCase) FreezeProtocol(p ProtocolBaseline, by string) error {
	if c.Status != StatusDraft {
		return errors.New("仅草稿案件可冻结方案")
	}
	check := PrecheckProtocol(p, by)
	if !check.Valid {
		return &ValidationError{Issues: check.Issues}
	}
	p = check.Normalized
	p.BaselineDigest = check.Summary.Digest
	by = strings.TrimSpace(by)
	if err := ValidateProtocol(p, by); err != nil {
		return err
	}
	for _, id := range p.ParticipantIDs {
		if id == by {
			return errors.New("方案冻结者不得作为独立复核员")
		}
	}
	p.CaseID = c.CaseID
	p.ProtocolID = fmt.Sprintf("protocol-%s-%d", c.CaseID, c.ProtocolRevision+1)
	p.FrozenAt = time.Now().UTC()
	c.Protocol = &p
	c.FrozenBy = by
	c.ProtocolRevision++
	c.Status = StatusProtocolFrozen
	c.touch()
	return nil
}
func (c *ExerciseCase) StartCollection() error {
	if c.Status != StatusProtocolFrozen {
		return errors.New("当前状态不能开始采集")
	}
	c.Status = StatusCollecting
	c.touch()
	return nil
}
func (c *ExerciseCase) AddRun(run ExerciseRun) error {
	if c.Status != StatusCollecting && c.Status != StatusReperforming {
		return errors.New("当前状态不能提交演练轮次")
	}
	if c.Protocol == nil {
		return errors.New("方案未冻结")
	}
	if c.Status == StatusCollecting && run.RunKind != "initial" {
		return errors.New("采集阶段必须提交初次演练")
	}
	if c.Status == StatusReperforming && run.RunKind != "targeted" {
		return errors.New("整改后必须提交定向复演")
	}
	if c.Status == StatusReperforming {
		if err := c.ValidateTargetCoverage(run.TargetRuleIDs); err != nil {
			return err
		}
	}
	run = NormalizeRun(run)
	for _, existing := range c.Runs {
		if existing.RunID == run.RunID {
			return errors.New("run_id 已存在")
		}
		if run.RunKind == "initial" && existing.RunKind == "initial" {
			return errors.New("初次演练只能提交一次")
		}
	}
	if err := ValidateRun(*c.Protocol, run); err != nil {
		return err
	}
	if err := c.validateCrossRunEvidence(run); err != nil {
		return err
	}
	run.CaseID = c.CaseID
	c.Runs = append(c.Runs, run)
	c.touch()
	return nil
}
func (c *ExerciseCase) SetEvaluation(results []RuleResult) {
	c.Evaluations = results
	all := len(results) > 0
	for _, r := range results {
		if !r.Passed {
			all = false
		}
	}
	for _, d := range c.Deviations {
		if d.Status != "closed" {
			all = false
		}
	}
	if all {
		c.Status = StatusReviewPending
	} else {
		c.Status = StatusEvaluationFailed
	}
	c.touch()
}
func (c *ExerciseCase) BeginCorrection() error {
	if c.Status != StatusEvaluationFailed {
		return errors.New("只有评估失败案件可整改")
	}
	c.Status = StatusCorrecting
	c.touch()
	return nil
}
func (c *ExerciseCase) OpenReperformance() error {
	if c.Status != StatusCorrecting {
		return errors.New("当前状态不能复演")
	}
	c.Status = StatusReperforming
	c.touch()
	return nil
}
func (c *ExerciseCase) SubmitReview() error {
	if c.Status != StatusReviewPending {
		return errors.New("尚未满足送审条件")
	}
	for _, d := range c.Deviations {
		if d.Status != "closed" {
			return errors.New("存在未关闭偏差")
		}
	}
	c.touch()
	return nil
}
func (c *ExerciseCase) ReviewDecision(reviewer, decision, reason string) error {
	if c.Status != StatusReviewPending {
		return errors.New("当前状态不可复核")
	}
	reviewer = strings.TrimSpace(reviewer)
	reason = strings.TrimSpace(reason)
	if reviewer == "" || reviewer == c.CreatedBy || reviewer == c.FrozenBy {
		return errors.New("复核员必须与记录和冻结职责分离")
	}
	if decision != "approved" && decision != "rejected" {
		return errors.New("复核结论无效")
	}
	if decision == "rejected" && reason == "" {
		return errors.New("拒绝必须填写理由")
	}
	c.Review = &Review{ReviewerID: reviewer, Decision: decision, Reason: reason, At: time.Now().UTC()}
	if decision == "approved" {
		c.Status = StatusQualified
	} else {
		c.Status = StatusRejected
	}
	c.touch()
	return nil
}

func (c *ExerciseCase) IsTerminal() bool {
	return c.Status == StatusQualified || c.Status == StatusRejected
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return fmt.Sprintf("%x", h[:])
}
func (c *ExerciseCase) ProtocolDigest() string { return Digest(c.Protocol) }
func (c *ExerciseCase) ResultDigest() string {
	rs := append([]RuleResult(nil), c.Evaluations...)
	sort.Slice(rs, func(i, j int) bool { return rs[i].RuleID < rs[j].RuleID })
	return Digest(rs)
}
