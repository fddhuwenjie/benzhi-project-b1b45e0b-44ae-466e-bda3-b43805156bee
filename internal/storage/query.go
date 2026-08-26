package storage

import (
	"os"
	"path/filepath"
	"sort"
)

func (s *Store) Events(caseID string, offset, limit int) ([]StoredEvent, int, error) {
	_, events, err := s.Load(caseID)
	if err != nil {
		return nil, 0, err
	}
	total := len(events)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return append([]StoredEvent(nil), events[offset:end]...), total, nil
}

func (s *Store) RootDigest(caseID string) (string, error) {
	_, events, err := s.Load(caseID)
	if err != nil {
		return "", err
	}
	return lastDigest(events), nil
}

func (s *Store) List() ([]CaseSummary, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "cases"))
	if err != nil {
		return nil, err
	}
	result := []CaseSummary{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		agg, _, err := s.Load(entry.Name())
		if err != nil {
			continue
		}
		result = append(result, CaseSummary{CaseID: agg.Case.CaseID, Status: agg.Case.Status, Revision: agg.Case.Revision, Title: agg.Case.Title})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CaseID < result[j].CaseID })
	return result, nil
}
