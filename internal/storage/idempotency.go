package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func (s *Store) readRequestsUnlocked(caseID string) (map[string]IdempotencyRecord, error) {
	dir, err := s.caseDir(caseID)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "requests.json"))
	if os.IsNotExist(err) {
		return map[string]IdempotencyRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records map[string]IdempotencyRecord
	if err := json.Unmarshal(b, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = map[string]IdempotencyRecord{}
	}
	return records, nil
}

func (s *Store) IdempotentResult(caseID, requestID string) (IdempotencyRecord, bool, error) {
	mu := s.lock(caseID)
	mu.Lock()
	defer mu.Unlock()
	records, err := s.readRequestsUnlocked(caseID)
	if err != nil {
		return IdempotencyRecord{}, false, err
	}
	record, ok := records[requestID]
	return record, ok, nil
}

func (s *Store) UpdateIdempotentResponse(caseID, requestID string, value any) error {
	mu := s.lock(caseID)
	mu.Lock()
	defer mu.Unlock()
	records, err := s.readRequestsUnlocked(caseID)
	if err != nil {
		return err
	}
	record, ok := records[requestID]
	if !ok {
		return ErrNotFound
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	record.Response = b
	records[requestID] = record
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	dir, err := s.caseDir(caseID)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "requests.json"), data, 0600)
}
