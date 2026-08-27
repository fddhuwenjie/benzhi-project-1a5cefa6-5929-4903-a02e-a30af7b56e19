package domain

import "time"

type certificateEnvelope struct {
	CertificateID, CaseID, ProtocolDigest, ResultDigest, AuditHeadDigest, ApprovedBy string
	EvidenceManifest                                                                 []EvidenceRef
	IssuedAt                                                                         time.Time
}

func (c *QualificationCertificate) ComputeDigest() string {
	if c == nil {
		return ""
	}
	return Digest(certificateEnvelope{c.CertificateID, c.CaseID, c.ProtocolDigest, c.ResultDigest, c.AuditHeadDigest, c.ApprovedBy, c.EvidenceManifest, c.IssuedAt})
}
func (c *QualificationCertificate) SealAuditHead(head string) {
	if c == nil {
		return
	}
	c.AuditHeadDigest = head
	c.CertificateDigest = c.ComputeDigest()
}
