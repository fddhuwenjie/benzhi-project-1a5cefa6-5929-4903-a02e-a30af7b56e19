package store

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"encoding/json"
	"fmt"
)

type TimelineEntry struct {
	Sequence       int64              `json:"sequence"`
	FrameDigest    string             `json:"frame_digest"`
	PreviousDigest string             `json:"previous_digest"`
	Event          domain.DomainEvent `json:"event"`
}

func (s *Store) Timeline(id string) ([]TimelineEntry, error) {
	if err := safeID(id); err != nil {
		return nil, err
	}
	frames, err := s.readAudit(id)
	if err != nil {
		return nil, err
	}
	entries := make([]TimelineEntry, 0, len(frames))
	for _, frame := range frames {
		var event domain.DomainEvent
		if err := json.Unmarshal(frame.Payload, &event); err != nil {
			return nil, fmt.Errorf("审计事件 %d 无法解析: %w", frame.Sequence, err)
		}
		entries = append(entries, TimelineEntry{Sequence: frame.Sequence, FrameDigest: frame.FrameDigest, PreviousDigest: frame.PreviousDigest, Event: event})
	}
	return entries, nil
}
