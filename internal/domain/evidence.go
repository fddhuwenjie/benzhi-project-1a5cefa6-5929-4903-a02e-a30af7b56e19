package domain

import (
	"encoding/hex"
	"errors"
	"strings"
)

func ValidateEvidence(ref EvidenceRef, requiredKinds map[string]struct{}) error {
	ref = NormalizeEvidence(ref)
	if strings.TrimSpace(ref.Kind) == "" || strings.TrimSpace(ref.URI) == "" {
		return errors.New("证据引用必须包含种类和 URI")
	}
	if len(requiredKinds) > 0 {
		if _, ok := requiredKinds[ref.Kind]; !ok {
			return errors.New("证据种类不在冻结方案中")
		}
	}
	raw, err := hex.DecodeString(ref.SHA256)
	if err != nil || len(raw) != 32 {
		return errors.New("证据引用必须包含有效 SHA-256 摘要")
	}
	return nil
}

func NormalizeEvidence(ref EvidenceRef) EvidenceRef {
	ref.Kind = strings.ToLower(strings.TrimSpace(ref.Kind))
	ref.URI = strings.TrimSpace(ref.URI)
	ref.SHA256 = strings.ToLower(strings.TrimSpace(ref.SHA256))
	return ref
}

func EvidenceKey(ref EvidenceRef) string {
	ref = NormalizeEvidence(ref)
	return ref.SHA256 + "|" + ref.Kind + "|" + ref.URI
}
