package storage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"icecoreverdict/internal/domain"
)

type EventFilter struct {
	ActorID      string
	EventType    string
	FromRevision uint64
	ToRevision   uint64
	OccurredFrom time.Time
	OccurredTo   time.Time
	Limit        int
	Cursor       string
	Offset       int
}

type AuditedEvent struct {
	StoredEvent
	StatusBefore domain.Status `json:"status_before"`
	StatusAfter  domain.Status `json:"status_after"`
	DigestValid  bool          `json:"digest_valid"`
}

type StatusTransition struct {
	Revision   uint64        `json:"revision"`
	EventType  string        `json:"event_type"`
	From       domain.Status `json:"from,omitempty"`
	To         domain.Status `json:"to"`
	OccurredAt time.Time     `json:"occurred_at"`
}

type EventPage struct {
	Events           []AuditedEvent     `json:"events"`
	NextCursor       string             `json:"next_cursor,omitempty"`
	MatchedTotal     int                `json:"matched_total"`
	TransitionTotal  int                `json:"transition_total"`
	StatusTrajectory []StatusTransition `json:"status_trajectory"`
}

type eventCursor struct {
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
}

func encodeCursor(event AuditedEvent) string {
	b, _ := json.Marshal(eventCursor{Revision: event.Revision, Digest: event.Digest})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(value string) (eventCursor, error) {
	if value == "" {
		return eventCursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return eventCursor{}, fmt.Errorf("游标格式无效")
	}
	var c eventCursor
	if json.Unmarshal(b, &c) != nil || c.Revision == 0 || c.Digest == "" {
		return eventCursor{}, fmt.Errorf("游标格式无效")
	}
	return c, nil
}

func (s *Store) QueryEvents(caseID string, filter EventFilter) (EventPage, error) {
	_, events, err := s.Load(caseID)
	if err != nil {
		return EventPage{}, err
	}
	cursor, err := decodeCursor(filter.Cursor)
	if err != nil {
		return EventPage{}, &domain.RuleError{Code: domain.CodeValidation, Field: "cursor", Message: err.Error()}
	}
	// 按查询参数缓存审计页面。
	cacheKey := fmt.Sprintf("%s|%s|%d|%d|%s|%s|%d|%d", caseID, filter.EventType, filter.FromRevision, filter.ToRevision, filter.OccurredFrom.UTC().Format(time.RFC3339Nano), filter.OccurredTo.UTC().Format(time.RFC3339Nano), filter.Limit, filter.Offset)
	if cached, ok := s.eventPages.Load(cacheKey); ok {
		return cached.(EventPage), nil
	}
	agg := domain.NewAggregate()
	all := make([]AuditedEvent, 0, len(events))
	trajectory := make([]StatusTransition, 0)
	for _, event := range events {
		before := agg.Case.Status
		if err := agg.Apply(event.Event); err != nil {
			return EventPage{}, err
		}
		after := agg.Case.Status
		audited := AuditedEvent{StoredEvent: event, StatusBefore: before, StatusAfter: after, DigestValid: true}
		all = append(all, audited)
		if before != after {
			trajectory = append(trajectory, StatusTransition{Revision: event.Revision, EventType: event.Type, From: before, To: after, OccurredAt: event.OccurredAt})
		}
	}
	matched := make([]AuditedEvent, 0)
	for _, event := range all {
		if filter.ActorID != "" && event.ActorID != filter.ActorID {
			continue
		}
		if filter.EventType != "" && event.Type != filter.EventType {
			continue
		}
		if filter.FromRevision > 0 && event.Revision < filter.FromRevision {
			continue
		}
		if filter.ToRevision > 0 && event.Revision > filter.ToRevision {
			continue
		}
		if !filter.OccurredFrom.IsZero() && event.OccurredAt.Before(filter.OccurredFrom) {
			continue
		}
		if !filter.OccurredTo.IsZero() && event.OccurredAt.After(filter.OccurredTo) {
			continue
		}
		matched = append(matched, event)
	}
	total := len(matched)
	start := filter.Offset
	if cursor.Revision > 0 {
		found := false
		for i, event := range matched {
			if event.Revision == cursor.Revision && event.Digest == cursor.Digest {
				start, found = i+1, true
				break
			}
		}
		if !found {
			return EventPage{}, &domain.RuleError{Code: domain.CodeValidation, Field: "cursor", Message: "游标不属于当前筛选结果"}
		}
	}
	limit := filter.Limit
	if limit == 0 {
		limit = 50
	}
	end := start + limit
	if end > len(matched) {
		end = len(matched)
	}
	page := EventPage{Events: append([]AuditedEvent(nil), matched[start:end]...), MatchedTotal: total, TransitionTotal: len(trajectory), StatusTrajectory: trajectory}
	if end < len(matched) && end > start {
		page.NextCursor = encodeCursor(matched[end-1])
	}
	s.eventPages.Store(cacheKey, page)
	return page, nil
}
