package web

import "benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"

type CaseView struct {
	*domain.ExerciseCase
	CanWrite         bool   `json:"can_write"`
	VerificationHint string `json:"verification_hint"`
}

func ProjectCase(c *domain.ExerciseCase) CaseView {
	hint := "案件尚未进入终态"
	if c.Status == domain.StatusQualified {
		hint = "可校验证书与审计链"
	}
	if c.Status == domain.StatusRejected {
		hint = "拒绝理由已封存"
	}
	return CaseView{ExerciseCase: c, CanWrite: domain.CanWrite(c.Status), VerificationHint: hint}
}
