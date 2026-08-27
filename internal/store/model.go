package store

import "benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"

type StoredResponse struct {
	Fingerprint string `json:"fingerprint"`
	Status      int    `json:"status"`
	Body        []byte `json:"body"`
	Summary     string `json:"summary,omitempty"`
}
type Snapshot struct {
	Case     *domain.ExerciseCase      `json:"case"`
	Requests map[string]StoredResponse `json:"requests"`
}
type AuditFrame struct {
	Sequence       int64  `json:"sequence"`
	Length         int    `json:"length"`
	PreviousDigest string `json:"previous_digest"`
	PayloadDigest  string `json:"payload_digest"`
	Payload        []byte `json:"payload"`
	FrameDigest    string `json:"frame_digest"`
}
