package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"icecoreverdict/internal/domain"
)

func (s *Store) writeCheckpointUnlocked(caseID string, events []StoredEvent) error {
	plain := make([]domain.Event, len(events))
	for i := range events {
		plain[i] = events[i].Event
	}
	agg, err := domain.Rehydrate(plain)
	if err != nil {
		return err
	}
	checkpoint := Checkpoint{Version: 1, CaseID: caseID, Revision: agg.Case.Revision, RootDigest: lastDigest(events), Aggregate: *agg, UpdatedAt: time.Now().UTC()}
	b, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	dir, err := s.caseDir(caseID)
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "checkpoint.json"), b, 0600)
}

func (s *Store) ReadCheckpoint(caseID string) (Checkpoint, error) {
	dir, err := s.caseDir(caseID)
	if err != nil {
		return Checkpoint{}, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "checkpoint.json"))
	if os.IsNotExist(err) {
		return Checkpoint{}, ErrNotFound
	}
	if err != nil {
		return Checkpoint{}, err
	}
	var cp Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return Checkpoint{}, err
	}
	return cp, nil
}
