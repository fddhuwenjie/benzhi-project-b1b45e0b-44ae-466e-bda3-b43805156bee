package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"icecoreverdict/internal/domain"
)

var ErrNotFound = errors.New("案件不存在")

type Store struct {
	root       string
	locks      sync.Map
	quarantine sync.Map
}

func Open(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if err := ensureDir(filepath.Join(root, "cases")); err != nil {
		return nil, err
	}
	if err := ensureDir(filepath.Join(root, "archives")); err != nil {
		return nil, err
	}
	s := &Store{root: root}
	if err := s.scan(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) lock(caseID string) *sync.Mutex {
	value, _ := s.locks.LoadOrStore(caseID, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func (s *Store) scan() error {
	entries, err := os.ReadDir(filepath.Join(s.root, "cases"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		_, _, err := s.loadVerified(entry.Name())
		if err != nil && !errors.Is(err, ErrNotFound) {
			s.quarantine.Store(entry.Name(), err.Error())
		}
	}
	return nil
}

func (s *Store) IsQuarantined(caseID string) (string, bool) {
	value, ok := s.quarantine.Load(caseID)
	if !ok {
		return "", false
	}
	return value.(string), true
}

func (s *Store) Load(caseID string) (*domain.Aggregate, []StoredEvent, error) {
	if reason, ok := s.IsQuarantined(caseID); ok {
		return nil, nil, &domain.RuleError{Code: domain.CodeCorrupt, Message: "案件已隔离: " + reason}
	}
	mu := s.lock(caseID)
	mu.Lock()
	defer mu.Unlock()
	return s.loadVerified(caseID)
}
