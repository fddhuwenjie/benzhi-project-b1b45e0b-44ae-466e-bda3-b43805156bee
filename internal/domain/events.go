package domain

import (
	"encoding/json"
	"time"
)

const (
	EventCaseCreated          = "case_created"
	EventBaselineFrozen       = "baseline_frozen"
	EventEvidenceAdded        = "evidence_added"
	EventHypothesisEvaluated  = "hypothesis_evaluated"
	EventRemediationPlanned   = "remediation_planned"
	EventRemediationVerified  = "remediation_verified"
	EventReviewSubmitted      = "review_submitted"
	EventReviewRejected       = "review_rejected"
	EventReviewApproved       = "review_approved"
	EventCorrectiveItemClosed = "corrective_item_closed"
	EventDispositionsRecorded = "dispositions_recorded"
	EventCaseArchived         = "case_archived"
)

type Event struct {
	CaseID     string          `json:"case_id"`
	Revision   uint64          `json:"revision"`
	Type       string          `json:"type"`
	OccurredAt time.Time       `json:"occurred_at"`
	ActorID    string          `json:"actor_id"`
	Data       json.RawMessage `json:"data"`
}

func NewEvent(caseID, eventType, actorID string, at time.Time, data any) (Event, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return Event{}, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return Event{CaseID: caseID, Type: eventType, ActorID: actorID, OccurredAt: at.UTC(), Data: b}, nil
}

func IsKnownEventType(value string) bool {
	switch value {
	case EventCaseCreated, EventBaselineFrozen, EventEvidenceAdded, EventHypothesisEvaluated, EventRemediationPlanned, EventRemediationVerified, EventReviewSubmitted, EventReviewRejected, EventReviewApproved, EventCorrectiveItemClosed, EventDispositionsRecorded, EventCaseArchived:
		return true
	default:
		return false
	}
}

type CaseCreatedData struct {
	Command CreateCase `json:"case"`
}
type BaselineFrozenData struct {
	Samples  []IceCoreSample `json:"samples"`
	FrozenAt time.Time       `json:"frozen_at"`
	Receipt  BaselineReceipt `json:"receipt"`
}
type EvidenceAddedData struct {
	Evidence     EvidenceRecord       `json:"evidence"`
	Completeness EvidenceCompleteness `json:"completeness"`
}
type HypothesisEvaluatedData struct {
	Hypothesis SourceHypothesis `json:"hypothesis"`
}
type RemediationPlannedData struct {
	Action RemediationAction `json:"action"`
}
type RemediationVerifiedData struct {
	ActionID      string    `json:"action_id"`
	EvidenceIDs   []string  `json:"evidence_ids"`
	Passed        bool      `json:"passed"`
	VerifiedAt    time.Time `json:"verified_at"`
	MeasuredValue float64   `json:"measured_value"`
	Difference    float64   `json:"difference"`
	Reason        string    `json:"reason"`
}
type ReviewStateData struct {
	Review          ReviewDecision   `json:"review"`
	CorrectiveItems []CorrectiveItem `json:"corrective_items,omitempty"`
}
type CorrectiveItemClosedData struct {
	Item CorrectiveItem `json:"item"`
}
type DispositionsData struct {
	SignerID  string           `json:"signer_id"`
	Decisions []SampleDecision `json:"decisions"`
	SignedAt  time.Time        `json:"signed_at"`
}
type ArchivedData struct {
	ArchivedAt    time.Time `json:"archived_at"`
	ArchiveDigest string    `json:"archive_digest,omitempty"`
}
