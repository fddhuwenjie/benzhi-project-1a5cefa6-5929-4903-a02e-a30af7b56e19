package snapshot_audit_atomicity_test

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotWriteFailureDoesNotAdvanceAudit(t *testing.T) {
	root := t.TempDir()
	repo, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.WithCase("CASE", func(snap *store.Snapshot) ([]byte, bool, error) {
		snap.Case = domain.NewCase("CASE", "原始楼宇", "creator", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		payload, marshalErr := json.Marshal(map[string]string{"event": "created"})
		return payload, true, marshalErr
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshotPath := filepath.Join(root, "cases", "CASE.json")
	original, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.WithCase("CASE", func(snap *store.Snapshot) ([]byte, bool, error) {
		snap.Case.BuildingName = "不应提交的楼宇"
		if removeErr := os.Remove(snapshotPath); removeErr != nil {
			return nil, false, removeErr
		}
		if mkdirErr := os.Mkdir(snapshotPath, 0750); mkdirErr != nil {
			return nil, false, mkdirErr
		}
		payload, marshalErr := json.Marshal(map[string]string{"event": "renamed"})
		return payload, true, marshalErr
	})
	if err == nil {
		t.Fatal("expected snapshot replacement to fail")
	}
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, original, 0640); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.VerifyCase("CASE"); err != nil {
		t.Fatalf("TestSnapshotWriteFailureDoesNotAdvanceAudit: failed snapshot transaction changed durable audit state: %v", err)
	}
}
