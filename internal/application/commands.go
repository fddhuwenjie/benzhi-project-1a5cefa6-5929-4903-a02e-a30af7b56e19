package application

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
)

type Meta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}
type CreateCaseCommand struct {
	Meta
	CaseID       string `json:"case_id"`
	BuildingName string `json:"building_name"`
	CreatedBy    string `json:"created_by"`
}
type FreezeProtocolCommand struct {
	Meta
	PrecheckDigest        string                `json:"precheck_digest"`
	FrozenBy              string                `json:"frozen_by"`
	Zones                 []string              `json:"zones"`
	Devices               []domain.Device       `json:"devices"`
	Rules                 []domain.SequenceRule `json:"rules"`
	RequiredEvidenceKinds []string              `json:"required_evidence_kinds"`
	ParticipantIDs        []string              `json:"participant_ids"`
}
type PrecheckProtocolCommand struct {
	Meta
	FrozenBy              string                `json:"frozen_by"`
	Zones                 []string              `json:"zones"`
	Devices               []domain.Device       `json:"devices"`
	Rules                 []domain.SequenceRule `json:"rules"`
	RequiredEvidenceKinds []string              `json:"required_evidence_kinds"`
	ParticipantIDs        []string              `json:"participant_ids"`
}
type ProtocolPrecheckResult struct {
	CaseID   string                  `json:"case_id"`
	Revision int64                   `json:"revision"`
	Status   domain.Status           `json:"status"`
	Check    domain.ProtocolPrecheck `json:"check"`
}
type SubmitRunCommand struct {
	Meta
	RunID         string         `json:"run_id"`
	RunKind       string         `json:"run_kind"`
	RecordedBy    string         `json:"recorded_by"`
	TargetRuleIDs []string       `json:"target_rule_ids"`
	Events        []domain.Event `json:"events"`
}
type DeviationInput struct {
	DeviationID        string   `json:"deviation_id"`
	RuleID             string   `json:"rule_id"`
	RootCause          string   `json:"root_cause"`
	CorrectiveAction   string   `json:"corrective_action"`
	ReperformanceScope string   `json:"reperformance_scope"`
	ScopeRuleIDs       []string `json:"scope_rule_ids"`
}
type CorrectCommand struct {
	Meta
	Deviations []DeviationInput `json:"deviations"`
}
type ReviewCommand struct {
	Meta
	ReviewerID string `json:"reviewer_id"`
	Decision   string `json:"decision"`
	Reason     string `json:"reason"`
}
type CommandResult struct {
	CaseID   string        `json:"case_id"`
	Revision int64         `json:"revision"`
	Status   domain.Status `json:"status"`
	Message  string        `json:"message"`
	Summary  string        `json:"summary,omitempty"`
}
type Receipt struct {
	CaseID          string         `json:"case_id"`
	RequestID       string         `json:"request_id"`
	Processed       bool           `json:"processed"`
	HTTPStatus      int            `json:"http_status,omitempty"`
	ResponseSummary string         `json:"response_summary,omitempty"`
	Result          *CommandResult `json:"result,omitempty"`
}
type Verification struct {
	Valid                bool                `json:"valid"`
	AuditValid           bool                `json:"audit_valid"`
	CertificateValid     bool                `json:"certificate_valid"`
	TerminalConsistent   bool                `json:"terminal_consistent"`
	Message              string              `json:"message"`
	Checks               []VerificationCheck `json:"checks"`
	FirstInvalidFrame    int64               `json:"first_invalid_frame,omitempty"`
	AuditFailureCategory string              `json:"audit_failure_category,omitempty"`
	EvidenceSources      []EvidenceTrace     `json:"evidence_sources"`
}
type VerificationCheck struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}
type EvidenceOrigin struct {
	RunID    string   `json:"run_id"`
	DeviceID string   `json:"device_id"`
	EventAt  string   `json:"event_at"`
	RuleIDs  []string `json:"rule_ids"`
}
type EvidenceTrace struct {
	Key      string             `json:"key"`
	Evidence domain.EvidenceRef `json:"evidence"`
	Origins  []EvidenceOrigin   `json:"origins"`
	Orphaned bool               `json:"orphaned"`
	Missing  bool               `json:"missing"`
}
type AuditTimeline struct {
	CaseID     string                `json:"case_id"`
	HeadDigest string                `json:"head_digest"`
	Entries    []store.TimelineEntry `json:"entries"`
}
