package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"icecoreverdict/internal/domain"
)

type transactionFile struct {
	Version  int                          `json:"version"`
	Events   []StoredEvent                `json:"events"`
	Requests map[string]IdempotencyRecord `json:"requests"`
}

func (s *Store) recoverTransactionUnlocked(caseID string) error {
	dir, err := s.caseDir(caseID)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "transaction.json")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var txn transactionFile
	if err := json.Unmarshal(b, &txn); err != nil {
		return fmt.Errorf("事务文件损坏: %w", err)
	}
	if txn.Version != 1 {
		return fmt.Errorf("未知事务版本")
	}
	eventData, err := encodeEventStream(txn.Events)
	if err != nil {
		return err
	}
	requestData, err := json.MarshalIndent(txn.Requests, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "events.jsonl"), eventData, 0600); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "requests.json"), requestData, 0600); err != nil {
		return err
	}
	if err := s.writeCheckpointUnlocked(caseID, txn.Events); err != nil {
		return err
	}
	return os.Remove(path)
}

func (s *Store) Commit(caseID, requestID string, expectedRevision uint64, newEvents []domain.Event, statusCode int, response json.RawMessage) (CommitResult, error) {
	if requestID == "" {
		return CommitResult{}, fmt.Errorf("request_id 不能为空")
	}
	mu := s.lock(caseID)
	mu.Lock()
	defer mu.Unlock()
	dir, err := s.caseDir(caseID)
	if err != nil {
		return CommitResult{}, err
	}
	if err := ensureDir(dir); err != nil {
		return CommitResult{}, err
	}
	existing, err := s.readEventsUnlocked(caseID)
	if err != nil && err != ErrNotFound {
		return CommitResult{}, err
	}
	requests, err := s.readRequestsUnlocked(caseID)
	if err != nil {
		return CommitResult{}, err
	}
	if record, ok := requests[requestID]; ok {
		return CommitResult{Revision: record.Revision, RootDigest: lastDigest(existing)}, nil
	}
	if uint64(len(existing)) != expectedRevision {
		return CommitResult{}, &domain.RuleError{Code: domain.CodeStateConflict, Message: "案件修订已变化，请重试"}
	}
	previous := lastDigest(existing)
	for i := range newEvents {
		newEvents[i].CaseID = caseID
		newEvents[i].Revision = uint64(len(existing) + 1)
		stored := StoredEvent{Event: newEvents[i], PreviousDigest: previous}
		canonical, err := json.Marshal(stored)
		if err != nil {
			return CommitResult{}, err
		}
		stored.Digest = digestBytes(canonical)
		previous = stored.Digest
		existing = append(existing, stored)
	}
	record := IdempotencyRecord{RequestID: requestID, CaseID: caseID, Revision: uint64(len(existing)), StatusCode: statusCode, Response: append(json.RawMessage(nil), response...), RecordedAt: time.Now().UTC()}
	requests[requestID] = record
	eventData, err := encodeEventStream(existing)
	if err != nil {
		return CommitResult{}, err
	}
	requestData, err := json.MarshalIndent(requests, "", "  ")
	if err != nil {
		return CommitResult{}, err
	}
	txnData, err := json.Marshal(transactionFile{Version: 1, Events: existing, Requests: requests})
	if err != nil {
		return CommitResult{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "transaction.json"), txnData, 0600); err != nil {
		return CommitResult{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "events.jsonl"), eventData, 0600); err != nil {
		return CommitResult{}, err
	}
	if err := atomicWrite(filepath.Join(dir, "requests.json"), requestData, 0600); err != nil {
		return CommitResult{}, err
	}
	if err := s.writeCheckpointUnlocked(caseID, existing); err != nil {
		return CommitResult{}, err
	}
	if err := os.Remove(filepath.Join(dir, "transaction.json")); err != nil && !os.IsNotExist(err) {
		return CommitResult{}, err
	}
	return CommitResult{Revision: uint64(len(existing)), RootDigest: previous}, nil
}

func lastDigest(events []StoredEvent) string {
	if len(events) == 0 {
		return ""
	}
	return events[len(events)-1].Digest
}
