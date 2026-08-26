package domain

import "time"

type Status string

const (
	StatusDraft         Status = "draft"
	StatusBounded       Status = "bounded"
	StatusInvestigating Status = "investigating"
	StatusRemediation   Status = "remediation_validation"
	StatusPendingReview Status = "pending_review"
	StatusDecided       Status = "decided"
	StatusArchived      Status = "archived"
)

type Disposition string

const (
	DispositionFull     Disposition = "all_research"
	DispositionLimited  Disposition = "limited_research"
	DispositionRejected Disposition = "no_research"
)

type ContaminationCase struct {
	CaseID                string    `json:"case_id"`
	Title                 string    `json:"title"`
	Status                Status    `json:"status"`
	TransferBatch         string    `json:"transfer_batch"`
	IncidentSummary       string    `json:"incident_summary"`
	BaselineFrozenAt      time.Time `json:"baseline_frozen_at,omitempty"`
	LeadActorID           string    `json:"lead_actor_id"`
	CreatedBy             string    `json:"created_by"`
	Revision              uint64    `json:"revision"`
	CreatedAt             time.Time `json:"created_at"`
	ArchivedAt            time.Time `json:"archived_at,omitempty"`
	AllowedResearchScopes []string  `json:"allowed_research_scopes,omitempty"`
}

type IceCoreSample struct {
	SampleID                   string      `json:"sample_id"`
	CaseID                     string      `json:"case_id"`
	CoreSegment                string      `json:"core_segment"`
	ContainerSeal              string      `json:"container_seal"`
	TransferTemperatureCelsius float64     `json:"transfer_temperature_celsius"`
	CustodyHolder              string      `json:"custody_holder"`
	TransferBatch              string      `json:"transfer_batch,omitempty"`
	TemperatureException       string      `json:"temperature_exception,omitempty"`
	Disposition                Disposition `json:"disposition,omitempty"`
	AllowedResearchScope       []string    `json:"allowed_research_scope,omitempty"`
	DispositionReason          string      `json:"disposition_reason,omitempty"`
	DispositionBasisReferences []string    `json:"disposition_basis_references,omitempty"`
}

type EvidenceRecord struct {
	EvidenceID      string    `json:"evidence_id"`
	CaseID          string    `json:"case_id"`
	SampleID        string    `json:"sample_id"`
	EvidenceType    string    `json:"evidence_type"`
	MetricName      string    `json:"metric_name"`
	Value           float64   `json:"value"`
	Unit            string    `json:"unit"`
	CollectedAt     time.Time `json:"collected_at"`
	CollectorID     string    `json:"collector_id"`
	ContentDigest   string    `json:"content_digest"`
	CustodySequence uint64    `json:"custody_sequence"`
}

type EvidenceRelation struct {
	EvidenceID string `json:"evidence_id"`
	Relation   string `json:"relation"`
}

type SourceHypothesis struct {
	HypothesisID        string    `json:"hypothesis_id"`
	CaseID              string    `json:"case_id"`
	SourceCategory      string    `json:"source_category"`
	Statement           string    `json:"statement"`
	EvidenceLinks       []string  `json:"evidence_links"`
	RelationLabels      []string  `json:"relation_labels"`
	ConfidenceScore     float64   `json:"confidence_score"`
	Conclusion          string    `json:"conclusion"`
	EvaluatedAt         time.Time `json:"evaluated_at"`
	SupportCount        int       `json:"support_count"`
	RefuteCount         int       `json:"refute_count"`
	IrrelevantCount     int       `json:"irrelevant_count"`
	Rank                int       `json:"rank"`
	LeadMargin          float64   `json:"lead_margin"`
	ConflictEvidenceIDs []string  `json:"conflict_evidence_ids,omitempty"`
	ConflictExplanation string    `json:"conflict_explanation,omitempty"`
}

type RetestThreshold struct {
	MetricName string  `json:"metric_name"`
	Comparator string  `json:"comparator"`
	Value      float64 `json:"value"`
	Unit       string  `json:"unit"`
}

type RemediationAction struct {
	ActionID            string           `json:"action_id"`
	CaseID              string           `json:"case_id"`
	ActionType          string           `json:"action_type"`
	TargetSampleIDs     []string         `json:"target_sample_ids"`
	AcceptanceThreshold string           `json:"acceptance_threshold"`
	Threshold           *RetestThreshold `json:"threshold,omitempty"`
	ExecutorID          string           `json:"executor_id"`
	WitnessID           string           `json:"witness_id"`
	Status              string           `json:"status"`
	RetestEvidenceIDs   []string         `json:"retest_evidence_ids,omitempty"`
	VerifiedAt          time.Time        `json:"verified_at,omitempty"`
	MeasuredValue       *float64         `json:"measured_value,omitempty"`
	Difference          *float64         `json:"difference,omitempty"`
	DecisionReason      string           `json:"decision_reason,omitempty"`
}

type SampleDecision struct {
	SampleID             string      `json:"sample_id"`
	Disposition          Disposition `json:"disposition"`
	AllowedResearchScope []string    `json:"allowed_research_scope,omitempty"`
	Reason               string      `json:"reason"`
	BasisReferences      []string    `json:"basis_references,omitempty"`
}

type ReviewFinding struct {
	FindingID  string    `json:"finding_id,omitempty"`
	Summary    string    `json:"summary"`
	Severity   string    `json:"severity,omitempty"`
	AssigneeID string    `json:"assignee_id,omitempty"`
	DueAt      time.Time `json:"due_at,omitempty"`
}

type CorrectiveItem struct {
	ItemID             string    `json:"item_id"`
	ReviewID           string    `json:"review_id"`
	Summary            string    `json:"summary"`
	Severity           string    `json:"severity"`
	AssigneeID         string    `json:"assignee_id"`
	DueAt              time.Time `json:"due_at"`
	Status             string    `json:"status"`
	Resolution         string    `json:"resolution,omitempty"`
	RelatedEvidenceIDs []string  `json:"related_evidence_ids,omitempty"`
	RelatedActionIDs   []string  `json:"related_action_ids,omitempty"`
	OverdueExplanation string    `json:"overdue_explanation,omitempty"`
	ClosedBy           string    `json:"closed_by,omitempty"`
	ClosedAt           time.Time `json:"closed_at,omitempty"`
	RejectedAt         time.Time `json:"rejected_at"`
	RejectedRevision   uint64    `json:"rejected_revision"`
}

type ReviewDecision struct {
	ReviewID         string              `json:"review_id"`
	CaseID           string              `json:"case_id"`
	ReviewerID       string              `json:"reviewer_id"`
	ConflictCheck    string              `json:"conflict_check"`
	Outcome          string              `json:"outcome"`
	Findings         []ReviewFinding     `json:"findings"`
	SampleDecisions  map[string]string   `json:"sample_decisions,omitempty"`
	ScopeConstraints map[string][]string `json:"scope_constraints,omitempty"`
	SignedAt         time.Time           `json:"signed_at"`
}

type BaselineSampleCheck struct {
	SampleID    string  `json:"sample_id"`
	Level       string  `json:"level"`
	Temperature float64 `json:"temperature_celsius"`
	Explanation string  `json:"explanation,omitempty"`
}

type BaselineReceipt struct {
	SampleChecks          []BaselineSampleCheck `json:"sample_checks"`
	NormalCount           int                   `json:"normal_count"`
	CriticalCount         int                   `json:"critical_count"`
	OutOfRangeCount       int                   `json:"out_of_range_count"`
	FrozenAt              time.Time             `json:"frozen_at"`
	FrozenRevision        uint64                `json:"frozen_revision"`
	MinTemperatureCelsius float64               `json:"min_temperature_celsius"`
	MaxTemperatureCelsius float64               `json:"max_temperature_celsius"`
}

type EvidenceCompleteness struct {
	SampleID          string               `json:"sample_id"`
	Counts            map[string]int       `json:"counts"`
	LatestCollectedAt map[string]time.Time `json:"latest_collected_at"`
	MissingCategories []string             `json:"missing_categories"`
	BlockingIssues    []string             `json:"blocking_issues"`
	Warnings          []string             `json:"warnings"`
	CompletionRate    float64              `json:"completion_rate"`
}

type RemediationCoverage struct {
	SampleID           string   `json:"sample_id"`
	Isolation          bool     `json:"isolation"`
	InstrumentCleaning bool     `json:"instrument_cleaning"`
	Retest             bool     `json:"retest"`
	LatestRetestPassed bool     `json:"latest_retest_passed"`
	MissingActionTypes []string `json:"missing_action_types,omitempty"`
}

type DispositionPrecheck struct {
	SampleID        string   `json:"sample_id"`
	Signable        bool     `json:"signable"`
	BasisReferences []string `json:"basis_references"`
	UnresolvedRisks []string `json:"unresolved_risks"`
}
