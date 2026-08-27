package store

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestSnapshotAuditAndTamperDetection(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.WithCase("C", func(snap *Snapshot) ([]byte, bool, error) {
		snap.Case = domain.NewCase("C", "楼宇", "u", time.Now())
		b, _ := json.Marshal(map[string]string{"event": "created"})
		return b, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.Load("C")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyAudit("C", snap.Case.AuditHead); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(s.auditPath("C"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("broken")
	_ = f.Close()
	if err = s.VerifyAudit("C", snap.Case.AuditHead); err == nil {
		t.Fatal("截断审计帧未被检测")
	}
}
