package storage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"icecoreverdict/internal/domain"
)

func (s *Store) readEventsUnlocked(caseID string) ([]StoredEvent, error) {
	dir, err := s.caseDir(caseID)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(filepath.Join(dir, "events.jsonl"))
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	events := []StoredEvent{}
	previous := ""
	for {
		line, readErr := reader.ReadBytes('\n')
		line = trimLine(line)
		if len(line) > 0 {
			var stored StoredEvent
			if err := json.Unmarshal(line, &stored); err != nil {
				return nil, fmt.Errorf("事件 JSON 损坏: %w", err)
			}
			expectedRevision := uint64(len(events) + 1)
			if stored.Revision != expectedRevision {
				return nil, fmt.Errorf("修订号不连续")
			}
			if stored.PreviousDigest != previous {
				return nil, fmt.Errorf("前序摘要不连续")
			}
			digest := stored.Digest
			stored.Digest = ""
			canonical, err := json.Marshal(stored)
			if err != nil {
				return nil, err
			}
			if digestBytes(canonical) != digest {
				return nil, fmt.Errorf("事件摘要校验失败")
			}
			stored.Digest = digest
			previous = digest
			events = append(events, stored)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return events, nil
}

func (s *Store) loadVerified(caseID string) (*domain.Aggregate, []StoredEvent, error) {
	if err := s.recoverTransactionUnlocked(caseID); err != nil {
		return nil, nil, err
	}
	stored, err := s.readEventsUnlocked(caseID)
	if err != nil {
		return nil, nil, err
	}
	events := make([]domain.Event, len(stored))
	for i := range stored {
		events[i] = stored[i].Event
	}
	checkpoint, checkpointErr := s.ReadCheckpoint(caseID)
	var agg *domain.Aggregate
	if checkpointErr == nil {
		if checkpoint.CaseID != caseID || checkpoint.Revision != checkpoint.Aggregate.Case.Revision || checkpoint.Revision > uint64(len(stored)) {
			return nil, nil, fmt.Errorf("检查点元数据无效")
		}
		expectedRoot := ""
		if checkpoint.Revision > 0 {
			expectedRoot = stored[checkpoint.Revision-1].Digest
		}
		if checkpoint.RootDigest != expectedRoot {
			return nil, nil, fmt.Errorf("检查点根摘要无效")
		}
		// Verify the cached aggregate content matches the state derived by
		// replaying the event history up to the checkpoint revision. The
		// checks above only cover identity/revision/root pointers, so without
		// this content verification a tampered checkpoint whose case_id,
		// revision and root_digest are left intact would be trusted and the
		// injected aggregate state would be served. Reject such checkpoints
		// instead of falling back, so the corrupt content is never exposed.
		rehydrated, rehydrateErr := domain.Rehydrate(events[:checkpoint.Revision])
		if rehydrateErr != nil {
			return nil, nil, fmt.Errorf("检查点内容校验失败: %w", rehydrateErr)
		}
		if !rehydrated.Equal(&checkpoint.Aggregate) {
			return nil, nil, fmt.Errorf("检查点内容与事件历史不一致")
		}
		copyAggregate := checkpoint.Aggregate
		agg = &copyAggregate
		for _, event := range events[checkpoint.Revision:] {
			if err := agg.Apply(event); err != nil {
				return nil, nil, err
			}
		}
	} else if errors.Is(checkpointErr, ErrNotFound) {
		agg, err = domain.Rehydrate(events)
		if err != nil {
			return nil, nil, err
		}
	} else {
		return nil, nil, checkpointErr
	}
	return agg, stored, nil
}

func encodeEventStream(events []StoredEvent) ([]byte, error) {
	var buffer bytes.Buffer
	for _, event := range events {
		line, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		buffer.Write(line)
		buffer.WriteByte('\n')
	}
	return buffer.Bytes(), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return syncDir(dir)
}
