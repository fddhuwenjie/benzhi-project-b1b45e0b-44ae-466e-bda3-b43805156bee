package archive

import (
	"encoding/json"
	"time"

	"icecoreverdict/internal/domain"
)

const FormatVersion = "icecore-verdict-archive/v2"

type EventIndex struct {
	Revision   uint64    `json:"revision"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	ActorID    string    `json:"actor_id"`
	Digest     string    `json:"digest"`
}

type Document struct {
	FormatVersion   string                     `json:"format_version"`
	Case            domain.ContaminationCase   `json:"case"`
	Samples         []domain.IceCoreSample     `json:"samples"`
	Evidence        []domain.EvidenceRecord    `json:"evidence"`
	Hypotheses      []domain.SourceHypothesis  `json:"hypotheses"`
	Actions         []domain.RemediationAction `json:"actions"`
	Reviews         []domain.ReviewDecision    `json:"reviews"`
	Events          []EventIndex               `json:"events"`
	EventRootDigest string                     `json:"event_root_digest"`
	GeneratedAt     time.Time                  `json:"generated_at"`
	ContentDigest   string                     `json:"content_digest"`
	SectionDigests  []SectionDigest            `json:"section_digests"`
	CorrectiveItems []domain.CorrectiveItem    `json:"corrective_items,omitempty"`
}

type SectionDigest struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}
type SectionVerification struct {
	Name           string `json:"name"`
	Valid          bool   `json:"valid"`
	ExpectedDigest string `json:"expected_digest"`
	ActualDigest   string `json:"actual_digest"`
}

type Verification struct {
	Valid                bool                  `json:"valid"`
	CaseID               string                `json:"case_id"`
	ContentDigest        string                `json:"content_digest"`
	EventRootDigest      string                `json:"event_root_digest"`
	Message              string                `json:"message"`
	FormatValid          bool                  `json:"format_valid"`
	TerminalStateValid   bool                  `json:"terminal_state_valid"`
	ContentDigestValid   bool                  `json:"content_digest_valid"`
	EventRootDigestValid bool                  `json:"event_root_digest_valid"`
	Sections             []SectionVerification `json:"sections"`
}

func canonical(doc Document) ([]byte, error) { doc.ContentDigest = ""; return json.Marshal(doc) }
