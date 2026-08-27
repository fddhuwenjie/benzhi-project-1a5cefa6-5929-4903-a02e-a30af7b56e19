package application

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"fmt"
)

type ConflictError struct {
	Message         string
	CaseID          string
	CurrentRevision int64
	CurrentStatus   domain.Status
	ReceiptSummary  string
}

func (e *ConflictError) Error() string { return e.Message }

type NotFoundError struct{ ID string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("案件 %s 不存在", e.ID) }
