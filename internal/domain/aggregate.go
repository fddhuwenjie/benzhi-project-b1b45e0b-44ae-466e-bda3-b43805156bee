package domain

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

type Aggregate struct {
	Case                ContaminationCase               `json:"case"`
	Samples             map[string]IceCoreSample        `json:"samples"`
	Evidence            map[string]EvidenceRecord       `json:"evidence"`
	Hypotheses          map[string]SourceHypothesis     `json:"hypotheses"`
	Actions             map[string]RemediationAction    `json:"actions"`
	Reviews             []ReviewDecision                `json:"reviews"`
	Investigators       map[string]bool                 `json:"investigators"`
	Witnesses           map[string]bool                 `json:"witnesses"`
	LastCustodySequence map[string]uint64               `json:"last_custody_sequence"`
	BaselineReceipt     BaselineReceipt                 `json:"baseline_receipt"`
	EvidenceMatrix      map[string]EvidenceCompleteness `json:"evidence_matrix"`
	CorrectiveItems     map[string]CorrectiveItem       `json:"corrective_items"`
	EvidenceRevisions   map[string]uint64               `json:"evidence_revisions"`
	ActionRevisions     map[string]uint64               `json:"action_revisions"`
}

func NewAggregate() *Aggregate {
	return &Aggregate{Samples: map[string]IceCoreSample{}, Evidence: map[string]EvidenceRecord{}, Hypotheses: map[string]SourceHypothesis{}, Actions: map[string]RemediationAction{}, Investigators: map[string]bool{}, Witnesses: map[string]bool{}, LastCustodySequence: map[string]uint64{}, EvidenceMatrix: map[string]EvidenceCompleteness{}, CorrectiveItems: map[string]CorrectiveItem{}, EvidenceRevisions: map[string]uint64{}, ActionRevisions: map[string]uint64{}}
}

func Rehydrate(events []Event) (*Aggregate, error) {
	a := NewAggregate()
	for i, event := range events {
		if event.Revision != uint64(i+1) {
			return nil, &RuleError{Code: CodeCorrupt, Message: "事件修订号不连续"}
		}
		if err := a.Apply(event); err != nil {
			return nil, fmt.Errorf("应用事件 %d: %w", event.Revision, err)
		}
	}
	return a, nil
}

func (a *Aggregate) ensureWritable() error {
	if a.Case.Status == StatusArchived {
		return &RuleError{Code: CodeArchived, Message: "案件已归档，拒绝业务写入"}
	}
	return nil
}

func (a *Aggregate) expect(statuses ...Status) error {
	for _, s := range statuses {
		if a.Case.Status == s {
			return nil
		}
	}
	values := make([]string, len(statuses))
	for i, s := range statuses {
		values[i] = string(s)
	}
	return conflict("当前状态 " + string(a.Case.Status) + " 不允许此操作，期望 " + strings.Join(values, " 或 "))
}

func (a *Aggregate) Apply(event Event) error {
	a.ensureMaps()
	if event.Revision != a.Case.Revision+1 {
		return &RuleError{Code: CodeCorrupt, Message: "聚合修订号不连续"}
	}
	switch event.Type {
	case EventCaseCreated:
		var d CaseCreatedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Case = ContaminationCase{CaseID: d.Command.CaseID, Title: d.Command.Title, Status: StatusDraft, TransferBatch: d.Command.TransferBatch, IncidentSummary: d.Command.IncidentSummary, LeadActorID: d.Command.LeadActorID, CreatedBy: d.Command.CreatedBy, CreatedAt: d.Command.CreatedAt.UTC(), AllowedResearchScopes: append([]string(nil), d.Command.AllowedResearchScopes...)}
	case EventBaselineFrozen:
		var d BaselineFrozenData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		for _, sample := range d.Samples {
			a.Samples[sample.SampleID] = sample
		}
		a.Case.BaselineFrozenAt = d.FrozenAt.UTC()
		a.BaselineReceipt = d.Receipt
		a.Case.Status = StatusBounded
	case EventEvidenceAdded:
		var d EvidenceAddedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Evidence[d.Evidence.EvidenceID] = d.Evidence
		if a.EvidenceRevisions == nil {
			a.EvidenceRevisions = map[string]uint64{}
		}
		a.EvidenceRevisions[d.Evidence.EvidenceID] = event.Revision
		a.EvidenceMatrix[d.Evidence.SampleID] = d.Completeness
		a.LastCustodySequence[d.Evidence.SampleID] = d.Evidence.CustodySequence
		a.Investigators[d.Evidence.CollectorID] = true
		if a.Case.Status != StatusRemediation {
			a.Case.Status = StatusInvestigating
		}
	case EventHypothesisEvaluated:
		var d HypothesisEvaluatedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Hypotheses[d.Hypothesis.HypothesisID] = d.Hypothesis
		a.recomputeHypothesisComparison()
	case EventRemediationPlanned:
		var d RemediationPlannedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Actions[d.Action.ActionID] = d.Action
		if a.ActionRevisions == nil {
			a.ActionRevisions = map[string]uint64{}
		}
		a.ActionRevisions[d.Action.ActionID] = event.Revision
		a.Investigators[d.Action.ExecutorID] = true
		a.Witnesses[d.Action.WitnessID] = true
		a.Case.Status = StatusRemediation
	case EventRemediationVerified:
		var d RemediationVerifiedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		action := a.Actions[d.ActionID]
		action.RetestEvidenceIDs = append([]string(nil), d.EvidenceIDs...)
		if d.Passed {
			action.Status = "verified"
		} else {
			action.Status = "failed"
		}
		action.VerifiedAt = d.VerifiedAt.UTC()
		value, difference := d.MeasuredValue, d.Difference
		action.MeasuredValue, action.Difference, action.DecisionReason = &value, &difference, d.Reason
		a.Actions[d.ActionID] = action
	case EventReviewSubmitted:
		a.Case.Status = StatusPendingReview
	case EventReviewRejected:
		var d ReviewStateData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Reviews = append(a.Reviews, d.Review)
		for _, item := range d.CorrectiveItems {
			a.CorrectiveItems[item.ItemID] = item
		}
		a.Case.Status = StatusInvestigating
	case EventReviewApproved:
		var d ReviewStateData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Reviews = append(a.Reviews, d.Review)
	case EventCorrectiveItemClosed:
		var d CorrectiveItemClosedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.CorrectiveItems[d.Item.ItemID] = d.Item
		a.Case.Status = StatusInvestigating
	case EventDispositionsRecorded:
		var d DispositionsData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		for _, decision := range d.Decisions {
			s := a.Samples[decision.SampleID]
			s.Disposition = decision.Disposition
			s.AllowedResearchScope = append([]string(nil), decision.AllowedResearchScope...)
			s.DispositionReason = decision.Reason
			s.DispositionBasisReferences = append([]string(nil), decision.BasisReferences...)
			a.Samples[s.SampleID] = s
		}
		a.Case.Status = StatusDecided
	case EventCaseArchived:
		var d ArchivedData
		if err := json.Unmarshal(event.Data, &d); err != nil {
			return err
		}
		a.Case.Status = StatusArchived
		a.Case.ArchivedAt = d.ArchivedAt.UTC()
	default:
		return &RuleError{Code: CodeCorrupt, Message: "未知事件类型: " + event.Type}
	}
	a.Case.Revision = event.Revision
	return nil
}

func (a *Aggregate) ensureMaps() {
	if a.Samples == nil {
		a.Samples = map[string]IceCoreSample{}
	}
	if a.Evidence == nil {
		a.Evidence = map[string]EvidenceRecord{}
	}
	if a.Hypotheses == nil {
		a.Hypotheses = map[string]SourceHypothesis{}
	}
	if a.Actions == nil {
		a.Actions = map[string]RemediationAction{}
	}
	if a.Investigators == nil {
		a.Investigators = map[string]bool{}
	}
	if a.Witnesses == nil {
		a.Witnesses = map[string]bool{}
	}
	if a.LastCustodySequence == nil {
		a.LastCustodySequence = map[string]uint64{}
	}
	if a.EvidenceMatrix == nil {
		a.EvidenceMatrix = map[string]EvidenceCompleteness{}
	}
	if a.CorrectiveItems == nil {
		a.CorrectiveItems = map[string]CorrectiveItem{}
	}
	if a.EvidenceRevisions == nil {
		a.EvidenceRevisions = map[string]uint64{}
	}
	if a.ActionRevisions == nil {
		a.ActionRevisions = map[string]uint64{}
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func nonblank(value string) bool     { return strings.TrimSpace(value) != "" }
func validTime(value time.Time) bool { return !value.IsZero() && value.Year() >= 2000 }

// Equal reports whether two aggregates have identical content.
// It treats nil and empty maps/slices as equivalent, matching the
// invariants maintained by ensureMaps and the JSON round-trip used when
// persisting checkpoints (omitted omitempty fields decode to nil, while the
// in-memory state uses non-nil empty slices/maps). This is used to verify
// that a cached checkpoint aggregate still matches the state derived from the
// event history, so that tampered or stale cached content cannot be trusted.
func (a *Aggregate) Equal(other *Aggregate) bool {
	if other == nil {
		return false
	}
	return deepEqualNormalized(reflect.ValueOf(*a), reflect.ValueOf(*other))
}

// deepEqualNormalized is a deep comparison that treats nil and empty
// slices/maps as equivalent. This mirrors the JSON round-trip invariants
// (omitted omitempty fields decode to nil) so a freshly rehydrated aggregate,
// which holds non-nil empty slices/maps, compares equal to one restored from
// checkpoint JSON.
func deepEqualNormalized(x, y reflect.Value) bool {
	if !x.IsValid() || !y.IsValid() {
		return x.IsValid() == y.IsValid()
	}
	switch x.Kind() {
	case reflect.Slice, reflect.Array:
		if x.Len() == 0 && y.Len() == 0 {
			return true
		}
		if x.Len() != y.Len() {
			return false
		}
		for i := 0; i < x.Len(); i++ {
			if !deepEqualNormalized(x.Index(i), y.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Map:
		if x.Len() == 0 && y.Len() == 0 {
			return true
		}
		if x.Len() != y.Len() {
			return false
		}
		for _, key := range x.MapKeys() {
			yv := y.MapIndex(key)
			if !yv.IsValid() || !deepEqualNormalized(x.MapIndex(key), yv) {
				return false
			}
		}
		return true
	case reflect.Struct:
		if x.Type() != y.Type() {
			return false
		}
		for i := 0; i < x.NumField(); i++ {
			if !deepEqualNormalized(x.Field(i), y.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Interface:
		if x.IsNil() && y.IsNil() {
			return true
		}
		if x.IsNil() || y.IsNil() {
			return false
		}
		return deepEqualNormalized(x.Elem(), y.Elem())
	case reflect.Pointer:
		if x.IsNil() && y.IsNil() {
			return true
		}
		if x.IsNil() || y.IsNil() {
			return false
		}
		return deepEqualNormalized(x.Elem(), y.Elem())
	default:
		// Compare primitive comparable values without forcing .Interface(),
		// which would panic on unexported fields. Use the typed comparison
		// helpers where possible and fall back to DeepEqual only for values
		// that can be surfaced.
		switch x.Kind() {
		case reflect.Bool:
			return x.Bool() == y.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return x.Int() == y.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return x.Uint() == y.Uint()
		case reflect.Float32, reflect.Float64:
			return x.Float() == y.Float()
		case reflect.String:
			return x.String() == y.String()
		default:
			if x.CanInterface() && y.CanInterface() {
				return reflect.DeepEqual(x.Interface(), y.Interface())
			}
			return false
		}
	}
}
