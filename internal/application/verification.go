package application

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"sort"
	"time"
)

func (s *Service) Verify(id string) (Verification, error) {
	c, err := s.Get(id)
	if err != nil {
		return Verification{}, err
	}
	audit, err := s.store.DiagnoseAudit(id)
	if err != nil {
		return Verification{}, err
	}
	fileCertificate, err := s.store.DiagnoseCertificate(id)
	if err != nil {
		return Verification{}, err
	}
	v := Verification{AuditValid: audit.Valid, FirstInvalidFrame: audit.FirstInvalidFrame, AuditFailureCategory: audit.FailureCategory}
	add := func(code, label string, valid bool, message string) {
		v.Checks = append(v.Checks, VerificationCheck{Code: code, Label: label, Valid: valid, Message: message})
	}
	qualified := c.Status == domain.StatusQualified
	fileValid := fileCertificate.Valid
	add("certificate_file_digest", "独立证书文件摘要", fileValid, fileCertificate.Message)
	snapshotValid := c.Certificate != nil && c.Certificate.CertificateDigest != "" && c.Certificate.ComputeDigest() == c.Certificate.CertificateDigest
	add("snapshot_certificate", "案件内证书副本", snapshotValid, validMessage(snapshotValid, "案件内证书摘要有效", "案件内证书缺失或摘要无效"))
	copyValid := snapshotValid && fileValid && domain.Digest(c.Certificate) == domain.Digest(fileCertificate.Value)
	add("certificate_copy_match", "独立证书与案件副本一致", copyValid, validMessage(copyValid, "两份证书内容一致", "独立证书文件与案件副本不一致"))
	protocolValid := snapshotValid && c.Certificate.ProtocolDigest == c.ProtocolDigest()
	add("protocol_digest", "方案摘要", protocolValid, validMessage(protocolValid, "冻结方案摘要一致", "冻结方案摘要不一致"))
	resultValid := snapshotValid && c.Certificate.ResultDigest == c.ResultDigest()
	add("result_digest", "结果摘要", resultValid, validMessage(resultValid, "评估结果摘要一致", "评估结果摘要不一致"))
	auditHeadValid := audit.Valid && audit.ActualHead == c.AuditHead && snapshotValid && c.Certificate.AuditHeadDigest == c.AuditHead
	add("audit_head", "审计头", auditHeadValid, validMessage(auditHeadValid, "审计头与证书一致", "审计头、案件或证书不一致"))
	add("audit_frames", "审计帧连续性", audit.Valid, audit.Message)
	v.TerminalConsistent = qualified && c.Certificate != nil && c.Review != nil && c.Review.Decision == "approved"
	add("terminal_state", "案件终态一致性", v.TerminalConsistent, validMessage(v.TerminalConsistent, "案件终态与批准证书一致", "案件终态、复核决定或证书不一致"))
	v.CertificateValid = fileValid && snapshotValid && copyValid && protocolValid && resultValid
	v.Valid = v.CertificateValid && v.AuditValid && auditHeadValid && v.TerminalConsistent
	v.EvidenceSources = traceEvidence(c)
	for _, trace := range v.EvidenceSources {
		if trace.Missing || trace.Orphaned {
			v.Valid = false
		}
	}
	if v.Valid {
		v.Message = "证书、证据来源、审计链和终态逐项校验通过"
	} else {
		v.Message = "完整性校验失败"
	}
	return v, nil
}

func validMessage(valid bool, passed, failed string) string {
	if valid {
		return passed
	}
	return failed
}

func traceEvidence(c *domain.ExerciseCase) []EvidenceTrace {
	if c.Certificate == nil {
		return nil
	}
	rulesBySource := map[string]map[string]struct{}{}
	manifest := map[string]domain.EvidenceRef{}
	for _, ref := range c.Certificate.EvidenceManifest {
		ref = domain.NormalizeEvidence(ref)
		manifest[domain.EvidenceKey(ref)] = ref
	}
	for _, result := range c.Evaluations {
		for _, source := range result.EvidenceSources {
			for _, ref := range source.EvidenceRefs {
				key := sourceKey(domain.EvidenceKey(ref), source.RunID, source.DeviceID, source.At)
				if rulesBySource[key] == nil {
					rulesBySource[key] = map[string]struct{}{}
				}
				rulesBySource[key][result.RuleID] = struct{}{}
			}
		}
	}
	traces := map[string]*EvidenceTrace{}
	for key, ref := range manifest {
		traces[key] = &EvidenceTrace{Key: key, Evidence: ref}
	}
	for _, run := range c.Runs {
		for _, event := range run.Events {
			for _, ref := range event.EvidenceRefs {
				key := domain.EvidenceKey(ref)
				trace, ok := traces[key]
				if !ok {
					continue
				}
				rules := []string{}
				for ruleID := range rulesBySource[sourceKey(key, run.RunID, event.DeviceID, event.At)] {
					rules = append(rules, ruleID)
				}
				sort.Strings(rules)
				trace.Origins = append(trace.Origins, EvidenceOrigin{RunID: run.RunID, DeviceID: event.DeviceID, EventAt: event.At.UTC().Format(time.RFC3339Nano), RuleIDs: rules})
			}
		}
	}
	out := make([]EvidenceTrace, 0, len(traces))
	for _, trace := range traces {
		sort.Slice(trace.Origins, func(i, j int) bool {
			if trace.Origins[i].RunID != trace.Origins[j].RunID {
				return trace.Origins[i].RunID < trace.Origins[j].RunID
			}
			if trace.Origins[i].DeviceID != trace.Origins[j].DeviceID {
				return trace.Origins[i].DeviceID < trace.Origins[j].DeviceID
			}
			return trace.Origins[i].EventAt < trace.Origins[j].EventAt
		})
		supported := false
		for _, origin := range trace.Origins {
			supported = supported || len(origin.RuleIDs) > 0
		}
		if len(trace.Origins) == 0 {
			trace.Missing = true
		}
		trace.Orphaned = len(trace.Origins) > 0 && !supported
		out = append(out, *trace)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func sourceKey(evidenceKey, runID, deviceID string, at time.Time) string {
	return evidenceKey + "|" + runID + "|" + deviceID + "|" + at.UTC().Format(time.RFC3339Nano)
}
