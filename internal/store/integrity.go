package store

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type AuditDiagnostic struct {
	Valid             bool   `json:"valid"`
	FrameCount        int    `json:"frame_count"`
	ActualHead        string `json:"actual_head"`
	FirstInvalidFrame int64  `json:"first_invalid_frame,omitempty"`
	FailureCategory   string `json:"failure_category,omitempty"`
	Message           string `json:"message"`
}

type CertificateDiagnostic struct {
	Present bool                             `json:"present"`
	Valid   bool                             `json:"valid"`
	Message string                           `json:"message"`
	Value   *domain.QualificationCertificate `json:"-"`
}

func (s *Store) DiagnoseCertificate(id string) (CertificateDiagnostic, error) {
	if err := safeID(id); err != nil {
		return CertificateDiagnostic{}, err
	}
	body, err := os.ReadFile(CertificatePath(s.root, "cert-"+id))
	if errors.Is(err, os.ErrNotExist) {
		return CertificateDiagnostic{Message: "独立证书文件不存在"}, nil
	}
	if err != nil {
		return CertificateDiagnostic{}, err
	}
	diagnostic := CertificateDiagnostic{Present: true}
	var certificate domain.QualificationCertificate
	if err := json.Unmarshal(body, &certificate); err != nil {
		diagnostic.Message = "独立证书文件格式损坏"
		return diagnostic, nil
	}
	diagnostic.Value = &certificate
	diagnostic.Valid = certificate.CertificateDigest != "" && certificate.ComputeDigest() == certificate.CertificateDigest
	if diagnostic.Valid {
		diagnostic.Message = "独立证书文件摘要有效"
	} else {
		diagnostic.Message = "独立证书文件摘要不符"
	}
	return diagnostic, nil
}

func (s *Store) DiagnoseAudit(id string) (AuditDiagnostic, error) {
	if err := safeID(id); err != nil {
		return AuditDiagnostic{}, err
	}
	file, err := os.Open(s.auditPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return AuditDiagnostic{Valid: true, Message: "审计链为空且结构有效"}, nil
	}
	if err != nil {
		return AuditDiagnostic{}, err
	}
	defer file.Close()
	diagnostic := AuditDiagnostic{Valid: true, Message: "审计帧连续且摘要有效"}
	reader := bufio.NewReader(file)
	previous := ""
	expectedSequence := int64(1)
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr == io.EOF && len(line) == 0 {
			break
		}
		if readErr == io.EOF {
			return invalidAudit(diagnostic, expectedSequence, "truncated_frame", "末帧截断"), nil
		}
		if readErr != nil {
			return AuditDiagnostic{}, readErr
		}
		var frame AuditFrame
		if err := json.Unmarshal(line, &frame); err != nil {
			return invalidAudit(diagnostic, expectedSequence, "frame_format_invalid", "审计帧格式损坏"), nil
		}
		if frame.Sequence != expectedSequence {
			return invalidAudit(diagnostic, expectedSequence, "sequence_gap", "审计帧序号跳跃"), nil
		}
		if frame.Length != len(frame.Payload) {
			return invalidAudit(diagnostic, frame.Sequence, "length_mismatch", "审计帧长度不符"), nil
		}
		if frame.PayloadDigest != sum(frame.Payload) {
			return invalidAudit(diagnostic, frame.Sequence, "payload_digest_mismatch", "审计帧载荷摘要不符"), nil
		}
		if frame.PreviousDigest != previous {
			return invalidAudit(diagnostic, frame.Sequence, "previous_digest_mismatch", "审计帧前序摘要不符"), nil
		}
		if frame.FrameDigest != frameDigest(frame) {
			return invalidAudit(diagnostic, frame.Sequence, "frame_digest_mismatch", "审计帧摘要不符"), nil
		}
		diagnostic.FrameCount++
		diagnostic.ActualHead = frame.FrameDigest
		previous = frame.FrameDigest
		expectedSequence++
	}
	return diagnostic, nil
}

func invalidAudit(diagnostic AuditDiagnostic, sequence int64, category, message string) AuditDiagnostic {
	diagnostic.Valid = false
	diagnostic.FirstInvalidFrame = sequence
	diagnostic.FailureCategory = category
	diagnostic.Message = message
	return diagnostic
}

type IntegrityResult struct {
	CaseID             string `json:"case_id"`
	AuditFrames        int    `json:"audit_frames"`
	AuditHead          string `json:"audit_head"`
	CertificatePresent bool   `json:"certificate_present"`
}

func (s *Store) VerifyCase(id string) (IntegrityResult, error) {
	snap, err := s.Load(id)
	if err != nil {
		return IntegrityResult{}, err
	}
	frames, err := s.readAudit(id)
	if err != nil {
		return IntegrityResult{}, err
	}
	head := ""
	if len(frames) > 0 {
		head = frames[len(frames)-1].FrameDigest
	}
	if snap.Case == nil || head != snap.Case.AuditHead {
		return IntegrityResult{}, errors.New("案件快照与审计头不一致")
	}
	result := IntegrityResult{CaseID: id, AuditFrames: len(frames), AuditHead: head}
	if snap.Case.Status == domain.StatusQualified {
		cert, readErr := s.LoadCertificate("cert-" + id)
		if readErr != nil {
			return result, fmt.Errorf("资格证书缺失或损坏: %w", readErr)
		}
		if snap.Case.Certificate == nil || cert.CertificateDigest != snap.Case.Certificate.CertificateDigest {
			return result, errors.New("独立证书与案件快照不一致")
		}
		result.CertificatePresent = true
	}
	return result, nil
}
func (s *Store) VerifyAll() error {
	entries, err := os.ReadDir(filepath.Join(s.root, "cases"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if _, err := s.VerifyCase(id); err != nil {
			return fmt.Errorf("案件 %s 完整性校验失败: %w", id, err)
		}
	}
	return nil
}
func (s *Store) LoadCertificate(id string) (*domain.QualificationCertificate, error) {
	if err := safeID(id); err != nil {
		return nil, err
	}
	body, err := os.ReadFile(CertificatePath(s.root, id))
	if err != nil {
		return nil, err
	}
	var cert domain.QualificationCertificate
	if err = json.Unmarshal(body, &cert); err != nil {
		return nil, err
	}
	if cert.CertificateDigest == "" || cert.ComputeDigest() != cert.CertificateDigest {
		return nil, errors.New("证书摘要无效")
	}
	return &cert, nil
}
