package review_certificate_atomicity_test

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/application"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewFailureDoesNotCommitQualifiedState(t *testing.T) {
	root := t.TempDir()
	repo, err := store.New(root)
	if err != nil {
		t.Fatal(err)
	}
	svc := application.New(repo)
	created, err := svc.Create(application.CreateCaseCommand{
		Meta: application.Meta{RequestID: "create"}, CaseID: "CASE", BuildingName: "测试楼宇", CreatedBy: "recorder",
	})
	if err != nil {
		t.Fatal(err)
	}
	freeze := application.FreezeProtocolCommand{
		Meta: application.Meta{RequestID: "freeze", ExpectedRevision: created.Revision}, FrozenBy: "lead", Zones: []string{"A区"},
		Devices: []domain.Device{{ID: "DET", Zone: "A区", Role: "detector"}, {ID: "FAN", Zone: "A区", Role: "fan"}},
		Rules:   []domain.SequenceRule{{ID: "R", FromRole: "detector", ToRole: "fan", MaxResponseMS: 1000}}, ParticipantIDs: []string{"recorder"},
	}
	checked, err := svc.PrecheckProtocol("CASE", application.PrecheckProtocolCommand{
		Meta: freeze.Meta, FrozenBy: freeze.FrozenBy, Zones: freeze.Zones, Devices: freeze.Devices, Rules: freeze.Rules, ParticipantIDs: freeze.ParticipantIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	freeze.PrecheckDigest = checked.Check.Summary.Digest
	state, err := svc.Freeze("CASE", freeze)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	state, err = svc.SubmitRun("CASE", application.SubmitRunCommand{
		Meta: application.Meta{RequestID: "run", ExpectedRevision: state.Revision}, RunID: "RUN", RunKind: "initial", RecordedBy: "recorder",
		Events: []domain.Event{{DeviceID: "DET", EventType: "detected", At: now}, {DeviceID: "FAN", EventType: "started", At: now.Add(500 * time.Millisecond)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	certificatePath := store.CertificatePath(root, "cert-CASE")
	if err := os.MkdirAll(filepath.Dir(certificatePath), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certificatePath, []byte("{}"), 0440); err != nil {
		t.Fatal(err)
	}
	_, err = svc.Review("CASE", application.ReviewCommand{
		Meta: application.Meta{RequestID: "review", ExpectedRevision: state.Revision}, ReviewerID: "reviewer", Decision: "approved",
	})
	if err == nil {
		t.Fatal("expected the conflicting certificate file to reject approval")
	}
	got, getErr := svc.Get("CASE")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if got.Status != domain.StatusReviewPending || got.Certificate != nil {
		t.Fatalf("TestReviewFailureDoesNotCommitQualifiedState: review returned an error but persisted status=%s certificate=%v", got.Status, got.Certificate != nil)
	}
	receipt, receiptErr := svc.Receipt("CASE", "review")
	if receiptErr != nil {
		t.Fatal(receiptErr)
	}
	if receipt.Processed {
		t.Fatal("TestReviewFailureDoesNotCommitQualifiedState: failed review was recorded as processed")
	}
}
