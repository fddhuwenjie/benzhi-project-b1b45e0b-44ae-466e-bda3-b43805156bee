package domain

import "time"

type CreateCase struct {
	CaseID                string    `json:"case_id"`
	Title                 string    `json:"title"`
	TransferBatch         string    `json:"transfer_batch"`
	IncidentSummary       string    `json:"incident_summary"`
	LeadActorID           string    `json:"lead_actor_id"`
	CreatedBy             string    `json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
	AllowedResearchScopes []string  `json:"allowed_research_scopes,omitempty"`
}

type FreezeBaseline struct {
	Samples               []IceCoreSample `json:"samples"`
	FrozenAt              time.Time       `json:"frozen_at"`
	MinTemperatureCelsius float64         `json:"min_temperature_celsius"`
	MaxTemperatureCelsius float64         `json:"max_temperature_celsius"`
}

type AddEvidence struct {
	Evidence EvidenceRecord `json:"evidence"`
}

type EvaluateHypothesis struct {
	HypothesisID        string             `json:"hypothesis_id"`
	SourceCategory      string             `json:"source_category"`
	Statement           string             `json:"statement"`
	Relations           []EvidenceRelation `json:"relations"`
	Conclusion          string             `json:"conclusion"`
	EvaluatedAt         time.Time          `json:"evaluated_at"`
	ConflictExplanation string             `json:"conflict_explanation,omitempty"`
}

type PlanRemediation struct {
	Action RemediationAction `json:"action"`
}

type VerifyRemediation struct {
	ActionID          string    `json:"action_id"`
	RetestEvidenceIDs []string  `json:"retest_evidence_ids"`
	Passed            bool      `json:"passed"`
	VerifiedAt        time.Time `json:"verified_at"`
}

type SubmitReview struct {
	SubmittedAt time.Time `json:"submitted_at"`
}

type CompleteReview struct {
	ReviewID   string          `json:"review_id"`
	ReviewerID string          `json:"reviewer_id"`
	Outcome    string          `json:"outcome"`
	Findings   []ReviewFinding `json:"findings"`
	SignedAt   time.Time       `json:"signed_at"`
}

type CloseCorrectiveItem struct {
	ItemID             string    `json:"item_id"`
	Resolution         string    `json:"resolution"`
	RelatedEvidenceIDs []string  `json:"related_evidence_ids,omitempty"`
	RelatedActionIDs   []string  `json:"related_action_ids,omitempty"`
	OverdueExplanation string    `json:"overdue_explanation,omitempty"`
	ClosedAt           time.Time `json:"closed_at"`
}

type RecordDispositions struct {
	SignerID  string           `json:"signer_id"`
	Decisions []SampleDecision `json:"decisions"`
	SignedAt  time.Time        `json:"signed_at"`
	Precheck  bool             `json:"precheck,omitempty"`
}

type ArchiveCase struct {
	ArchivedAt time.Time `json:"archived_at"`
}
