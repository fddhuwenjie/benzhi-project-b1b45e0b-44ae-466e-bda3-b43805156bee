package storage

import (
	"encoding/json"
	"time"

	"icecoreverdict/internal/domain"
)

type StoredEvent struct {
	domain.Event
	PreviousDigest string `json:"previous_digest"`
	Digest         string `json:"digest"`
}

type IdempotencyRecord struct {
	RequestID  string          `json:"request_id"`
	CaseID     string          `json:"case_id"`
	Revision   uint64          `json:"revision"`
	StatusCode int             `json:"status_code"`
	Response   json.RawMessage `json:"response"`
	RecordedAt time.Time       `json:"recorded_at"`
}

type Checkpoint struct {
	Version    int              `json:"version"`
	CaseID     string           `json:"case_id"`
	Revision   uint64           `json:"revision"`
	RootDigest string           `json:"root_digest"`
	Aggregate  domain.Aggregate `json:"aggregate"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

type CommitResult struct {
	Revision   uint64 `json:"revision"`
	RootDigest string `json:"root_digest"`
}

type CaseSummary struct {
	CaseID   string        `json:"case_id"`
	Status   domain.Status `json:"status"`
	Revision uint64        `json:"revision"`
	Title    string        `json:"title"`
}
