package store

import (
	"benzhi-project-1a5cefa6-5929-4903-a02e-a30af7b56e19/internal/domain"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Store struct {
	root  string
	locks lockTable
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("数据目录不能为空")
	}
	if err := os.MkdirAll(filepath.Join(root, "cases"), 0750); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}
func (s *Store) caseLock(id string) *sync.Mutex {
	return s.locks.forCase(id)
}
func safeID(id string) error {
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return errors.New("标识无效")
	}
	return nil
}
func (s *Store) snapshotPath(id string) string { return filepath.Join(s.root, "cases", id+".json") }
func (s *Store) auditPath(id string) string    { return filepath.Join(s.root, "cases", id+".audit") }
func (s *Store) Load(id string) (*Snapshot, error) {
	if err := safeID(id); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.snapshotPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err = json.Unmarshal(b, &snap); err != nil {
		return nil, fmt.Errorf("快照损坏: %w", err)
	}
	if snap.Requests == nil {
		snap.Requests = map[string]StoredResponse{}
	}
	return &snap, nil
}
func (s *Store) WithCase(id string, fn func(*Snapshot) ([]byte, bool, error)) ([]byte, error) {
	if err := safeID(id); err != nil {
		return nil, err
	}
	l := s.caseLock(id)
	l.Lock()
	defer l.Unlock()
	snap, err := s.Load(id)
	if errors.Is(err, os.ErrNotExist) {
		snap = &Snapshot{Requests: map[string]StoredResponse{}}
	} else if err != nil {
		return nil, err
	}
	payload, changed, err := fn(snap)
	if err != nil {
		return nil, err
	}
	if !changed {
		return payload, nil
	}
	if snap.Case == nil {
		return nil, errors.New("案件快照为空")
	}
	head, prevSize, err := s.appendAudit(id, payload)
	if err != nil {
		s.rollbackAudit(id, prevSize)
		return nil, err
	}
	snap.Case.AuditHead = head
	if snap.Case.Certificate != nil {
		snap.Case.Certificate.SealAuditHead(head)
	}
	if err = s.writeSnapshot(id, snap); err != nil {
		s.rollbackAudit(id, prevSize)
		return nil, err
	}
	return payload, nil
}
func (s *Store) writeSnapshot(id string, snap *Snapshot) error {
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.snapshotPath(id))
	f, err := os.CreateTemp(dir, id+"-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, s.snapshotPath(id)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err == nil {
		err = d.Sync()
		d.Close()
	}
	return err
}
func (s *Store) appendAudit(id string, payload []byte) (head string, prevSize int64, err error) {
	frames, err := s.readAudit(id)
	if err != nil {
		return "", 0, err
	}
	prev := ""
	seq := int64(1)
	if len(frames) > 0 {
		prev = frames[len(frames)-1].FrameDigest
		seq = frames[len(frames)-1].Sequence + 1
	}
	info, statErr := os.Stat(s.auditPath(id))
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return "", 0, statErr
	}
	if info != nil {
		prevSize = info.Size()
	}
	ph := sum(payload)
	f := AuditFrame{Sequence: seq, Length: len(payload), PreviousDigest: prev, PayloadDigest: ph, Payload: payload}
	f.FrameDigest = frameDigest(f)
	b, _ := json.Marshal(f)
	file, err := os.OpenFile(s.auditPath(id), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return "", prevSize, err
	}
	if _, err = file.Write(append(b, '\n')); err != nil {
		file.Close()
		return "", prevSize, err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return "", prevSize, err
	}
	if err = file.Close(); err != nil {
		return "", prevSize, err
	}
	return f.FrameDigest, prevSize, nil
}

func (s *Store) rollbackAudit(id string, prevSize int64) {
	if prevSize <= 0 {
		_ = os.Remove(s.auditPath(id))
		return
	}
	path := s.auditPath(id)
	if err := os.Truncate(path, prevSize); err != nil {
		return
	}
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		d.Close()
	}
}
func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func frameDigest(f AuditFrame) string {
	return sum([]byte(fmt.Sprintf("%d|%d|%s|%s", f.Sequence, f.Length, f.PreviousDigest, f.PayloadDigest)))
}
func (s *Store) readAudit(id string) ([]AuditFrame, error) {
	file, err := os.Open(s.auditPath(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	rd := bufio.NewReader(file)
	out := []AuditFrame{}
	for {
		line, er := rd.ReadBytes('\n')
		if er == io.EOF && len(line) == 0 {
			break
		}
		if er == io.EOF {
			return nil, errors.New("审计链存在截断帧")
		}
		if er != nil {
			return nil, er
		}
		var f AuditFrame
		if json.Unmarshal(line, &f) != nil {
			return nil, errors.New("审计帧格式损坏")
		}
		prev := ""
		seq := int64(1)
		if len(out) > 0 {
			prev = out[len(out)-1].FrameDigest
			seq = out[len(out)-1].Sequence + 1
		}
		if f.Sequence != seq || f.PreviousDigest != prev || f.Length != len(f.Payload) || f.PayloadDigest != sum(f.Payload) || f.FrameDigest != frameDigest(f) {
			return nil, errors.New("审计链校验失败")
		}
		out = append(out, f)
	}
	return out, nil
}
func (s *Store) VerifyAudit(id, expectedHead string) error {
	if err := safeID(id); err != nil {
		return err
	}
	frames, err := s.readAudit(id)
	if err != nil {
		return err
	}
	head := ""
	if len(frames) > 0 {
		head = frames[len(frames)-1].FrameDigest
	}
	if head != expectedHead {
		return errors.New("审计头摘要不一致")
	}
	return nil
}
func Fingerprint(v any) string           { b, _ := json.Marshal(v); return sum(b) }
func ResponseSummary(body []byte) string { return sum(body) }

func (s *Store) LookupRequest(caseID, requestID string) (StoredResponse, bool, error) {
	snap, err := s.Load(caseID)
	if err != nil {
		return StoredResponse{}, false, err
	}
	response, ok := snap.Requests[requestID]
	if ok && response.Summary == "" {
		response.Summary = ResponseSummary(response.Body)
	}
	return response, ok, nil
}
func CertificatePath(root, id string) string { return filepath.Join(root, "certificates", id+".json") }
func (s *Store) SaveCertificate(c *domain.QualificationCertificate) error {
	dir := filepath.Join(s.root, "certificates")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	path := CertificatePath(s.root, c.CertificateID)
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if old, readErr := os.ReadFile(path); readErr == nil {
		if string(old) == string(b) {
			return nil
		}
		return errors.New("证书已存在且内容不一致")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0440)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
