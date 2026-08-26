package application

import (
	"encoding/json"
	"time"

	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

type Command struct {
	RequestID string          `json:"request_id"`
	Action    string          `json:"action"`
	ActorID   string          `json:"actor_id"`
	Payload   json.RawMessage `json:"payload"`
}

type CommandResult struct {
	CaseID              string                       `json:"case_id"`
	RequestID           string                       `json:"request_id"`
	Action              string                       `json:"action"`
	Status              domain.Status                `json:"status"`
	Revision            uint64                       `json:"revision"`
	EventTypes          []string                     `json:"event_types"`
	ArchiveDigest       string                       `json:"archive_digest,omitempty"`
	CompletedAt         time.Time                    `json:"completed_at"`
	BaselineReceipt     *domain.BaselineReceipt      `json:"baseline_receipt,omitempty"`
	DispositionPrecheck []domain.DispositionPrecheck `json:"disposition_precheck,omitempty"`
}

type CaseView struct {
	Case                          domain.ContaminationCase      `json:"case"`
	Samples                       []domain.IceCoreSample        `json:"samples"`
	Evidence                      []domain.EvidenceRecord       `json:"evidence"`
	Hypotheses                    []domain.SourceHypothesis     `json:"hypotheses"`
	Actions                       []domain.RemediationAction    `json:"actions"`
	Reviews                       []domain.ReviewDecision       `json:"reviews"`
	EventRootDigest               string                        `json:"event_root_digest"`
	BaselineReceipt               domain.BaselineReceipt        `json:"baseline_receipt"`
	EvidenceCompletenessMatrix    []domain.EvidenceCompleteness `json:"evidence_completeness_matrix"`
	OverallEvidenceCompletionRate float64                       `json:"overall_evidence_completion_rate"`
	RemediationCoverageMatrix     []domain.RemediationCoverage  `json:"remediation_coverage_matrix"`
	CorrectiveItems               []domain.CorrectiveItem       `json:"corrective_items"`
}

type Services struct {
	Store   *storage.Store
	Archive *archive.Service
}
