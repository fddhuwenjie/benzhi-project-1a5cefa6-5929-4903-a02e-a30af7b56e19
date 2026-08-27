package domain

import (
	"encoding/json"
	"time"
)

type DomainEvent struct {
	EventType          string    `json:"event_type"`
	CaseID             string    `json:"case_id"`
	RequestID          string    `json:"request_id"`
	RequestFingerprint string    `json:"request_fingerprint"`
	RevisionBefore     int64     `json:"revision_before"`
	RevisionAfter      int64     `json:"revision_after"`
	ResponseDigest     string    `json:"response_digest"`
	BusinessSummary    string    `json:"business_summary,omitempty"`
	Decision           string    `json:"decision,omitempty"`
	OccurredAt         time.Time `json:"occurred_at"`
}

func NewDomainEvent(eventType, caseID, requestID, fingerprint string, before, after int64, response []byte) DomainEvent {
	return DomainEvent{EventType: eventType, CaseID: caseID, RequestID: requestID, RequestFingerprint: fingerprint, RevisionBefore: before, RevisionAfter: after, ResponseDigest: Digest(json.RawMessage(response)), OccurredAt: time.Now().UTC()}
}
