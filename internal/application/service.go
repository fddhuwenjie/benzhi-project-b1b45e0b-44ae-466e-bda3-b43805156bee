package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

type Service struct {
	store   *storage.Store
	archive *archive.Service
	boxes   *mailboxes
}

func New(services Services) *Service {
	return &Service{store: services.Store, archive: services.Archive, boxes: newMailboxes(32)}
}

func (s *Service) Create(ctx context.Context, requestID string, cmd domain.CreateCase) (CommandResult, error) {
	return s.boxes.submit(ctx, cmd.CaseID, func() (CommandResult, error) {
		if old, ok, err := s.store.IdempotentResult(cmd.CaseID, requestID); err != nil {
			return CommandResult{}, err
		} else if ok {
			return decodeResult(old.Response)
		}
		if _, _, err := s.store.Load(cmd.CaseID); err == nil {
			return CommandResult{}, &domain.RuleError{Code: domain.CodeStateConflict, Field: "case_id", Message: "案件已存在"}
		} else if !errors.Is(err, storage.ErrNotFound) {
			return CommandResult{}, err
		}
		events, err := domain.DecideCreate(cmd)
		if err != nil {
			return CommandResult{}, err
		}
		return s.commit(cmd.CaseID, requestID, "create", events, 0)
	})
}

func (s *Service) Execute(ctx context.Context, caseID string, cmd Command) (CommandResult, error) {
	return s.boxes.submit(ctx, caseID, func() (CommandResult, error) {
		if old, ok, err := s.store.IdempotentResult(caseID, cmd.RequestID); err != nil {
			return CommandResult{}, err
		} else if ok {
			result, err := decodeResult(old.Response)
			if err == nil && cmd.Action == "archive" && result.ArchiveDigest == "" {
				doc, saveErr := s.archive.Save(caseID)
				if saveErr != nil {
					return CommandResult{}, saveErr
				}
				result.ArchiveDigest = doc.ContentDigest
				if updateErr := s.store.UpdateIdempotentResponse(caseID, cmd.RequestID, result); updateErr != nil {
					return CommandResult{}, updateErr
				}
			}
			return result, err
		}
		agg, _, err := s.store.Load(caseID)
		if err != nil {
			return CommandResult{}, err
		}
		var events []domain.Event
		switch cmd.Action {
		case "freeze_baseline":
			var c domain.FreezeBaseline
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideFreeze(c, cmd.ActorID)
			}
		case "add_evidence":
			var c domain.AddEvidence
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideAddEvidence(c, cmd.ActorID)
			}
		case "evaluate_hypothesis":
			var c domain.EvaluateHypothesis
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideHypothesis(c, cmd.ActorID)
			}
		case "plan_remediation":
			var c domain.PlanRemediation
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecidePlanRemediation(c, cmd.ActorID)
			}
		case "verify_remediation":
			var c domain.VerifyRemediation
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideVerifyRemediation(c, cmd.ActorID)
			}
		case "submit_review":
			var c domain.SubmitReview
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideSubmitReview(c, cmd.ActorID)
			}
		case "complete_review":
			var c domain.CompleteReview
			err = decode(cmd.Payload, &c)
			if err == nil {
				if c.ReviewerID != cmd.ActorID {
					err = &domain.RuleError{Code: domain.CodeValidation, Field: "reviewer_id", Message: "必须与当前操作人一致"}
				} else {
					events, err = agg.DecideReview(c)
				}
			}
		case "close_corrective_item":
			var c domain.CloseCorrectiveItem
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideCloseCorrective(c, cmd.ActorID)
			}
		case "record_dispositions":
			var c domain.RecordDispositions
			err = decode(cmd.Payload, &c)
			if err == nil {
				if c.SignerID != cmd.ActorID {
					err = &domain.RuleError{Code: domain.CodeValidation, Field: "signer_id", Message: "必须与当前操作人一致"}
					break
				}
				if c.Precheck {
					var checks []domain.DispositionPrecheck
					checks, _, err = agg.PrecheckDispositions(c)
					if err == nil {
						return CommandResult{CaseID: caseID, RequestID: cmd.RequestID, Action: cmd.Action, Status: agg.Case.Status, Revision: agg.Case.Revision, DispositionPrecheck: checks, CompletedAt: time.Now().UTC()}, nil
					}
				} else {
					events, err = agg.DecideDispositions(c)
				}
			}
		case "archive":
			var c domain.ArchiveCase
			err = decode(cmd.Payload, &c)
			if err == nil {
				events, err = agg.DecideArchive(c, cmd.ActorID)
			}
		default:
			return CommandResult{}, &domain.RuleError{Code: domain.CodeValidation, Field: "action", Message: "未知案件命令"}
		}
		if err != nil {
			return CommandResult{}, err
		}
		result, err := s.commit(caseID, cmd.RequestID, cmd.Action, events, agg.Case.Revision)
		if err != nil {
			return CommandResult{}, err
		}
		if cmd.Action == "archive" {
			doc, err := s.archive.Save(caseID)
			if err != nil {
				return CommandResult{}, fmt.Errorf("保存档案: %w", err)
			}
			result.ArchiveDigest = doc.ContentDigest
			if err := s.store.UpdateIdempotentResponse(caseID, cmd.RequestID, result); err != nil {
				return CommandResult{}, err
			}
		}
		return result, nil
	})
}

func (s *Service) commit(caseID, requestID, action string, events []domain.Event, revision uint64) (CommandResult, error) {
	types := make([]string, len(events))
	for i := range events {
		types[i] = events[i].Type
	}
	result := CommandResult{CaseID: caseID, RequestID: requestID, Action: action, Revision: revision + uint64(len(events)), EventTypes: types, CompletedAt: time.Now().UTC()}
	if len(events) > 0 && events[len(events)-1].Type == domain.EventBaselineFrozen {
		var d domain.BaselineFrozenData
		if json.Unmarshal(events[len(events)-1].Data, &d) == nil {
			result.BaselineReceipt = &d.Receipt
		}
	}
	if len(events) > 0 {
		result.Status = statusAfter(events[len(events)-1].Type)
	}
	b, _ := json.Marshal(result)
	if _, err := s.store.Commit(caseID, requestID, revision, events, 200, b); err != nil {
		return CommandResult{}, err
	}
	agg, _, err := s.store.Load(caseID)
	if err != nil {
		return CommandResult{}, err
	}
	result.Status = agg.Case.Status
	result.Revision = agg.Case.Revision
	if corrected, _ := json.Marshal(result); !bytes.Equal(corrected, b) {
		if err := s.store.UpdateIdempotentResponse(caseID, requestID, result); err != nil {
			return CommandResult{}, err
		}
	}
	return result, nil
}

func statusAfter(eventType string) domain.Status {
	switch eventType {
	case domain.EventCaseCreated:
		return domain.StatusDraft
	case domain.EventBaselineFrozen:
		return domain.StatusBounded
	case domain.EventEvidenceAdded, domain.EventHypothesisEvaluated, domain.EventReviewRejected, domain.EventCorrectiveItemClosed:
		return domain.StatusInvestigating
	case domain.EventRemediationPlanned, domain.EventRemediationVerified:
		return domain.StatusRemediation
	case domain.EventReviewSubmitted, domain.EventReviewApproved:
		return domain.StatusPendingReview
	case domain.EventDispositionsRecorded:
		return domain.StatusDecided
	case domain.EventCaseArchived:
		return domain.StatusArchived
	default:
		return ""
	}
}

func decode(data []byte, target any) error {
	if len(data) == 0 {
		data = []byte("{}")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return &domain.RuleError{Code: domain.CodeValidation, Field: "payload", Message: "JSON 结构无效"}
	}
	return nil
}
func decodeResult(data []byte) (CommandResult, error) {
	var r CommandResult
	err := json.Unmarshal(data, &r)
	return r, err
}

func (s *Service) Get(caseID string) (CaseView, error) {
	agg, events, err := s.store.Load(caseID)
	if err != nil {
		return CaseView{}, err
	}
	v := CaseView{Case: agg.Case, Reviews: append([]domain.ReviewDecision(nil), agg.Reviews...), BaselineReceipt: agg.BaselineReceipt}
	for _, id := range sorted(agg.Samples) {
		v.Samples = append(v.Samples, agg.Samples[id])
	}
	for _, id := range sorted(agg.Evidence) {
		v.Evidence = append(v.Evidence, agg.Evidence[id])
	}
	for _, hypothesis := range rankedHypotheses(agg.Hypotheses) {
		v.Hypotheses = append(v.Hypotheses, hypothesis)
	}
	for _, id := range sorted(agg.Actions) {
		v.Actions = append(v.Actions, agg.Actions[id])
	}
	v.EvidenceCompletenessMatrix, v.OverallEvidenceCompletionRate = agg.EvidenceCompletenessMatrix()
	v.RemediationCoverageMatrix = agg.RemediationCoverageMatrix()
	for _, id := range sorted(agg.CorrectiveItems) {
		v.CorrectiveItems = append(v.CorrectiveItems, agg.CorrectiveItems[id])
	}
	if len(events) > 0 {
		v.EventRootDigest = events[len(events)-1].Digest
	}
	return v, nil
}
func sorted[T any](m map[string]T) []string {
	k := make([]string, 0, len(m))
	for v := range m {
		k = append(k, v)
	}
	sort.Strings(k)
	return k
}
func rankedHypotheses(values map[string]domain.SourceHypothesis) []domain.SourceHypothesis {
	result := make([]domain.SourceHypothesis, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Rank != result[j].Rank {
			return result[i].Rank < result[j].Rank
		}
		return result[i].HypothesisID < result[j].HypothesisID
	})
	return result
}
func (s *Service) Events(caseID string, offset, limit int) ([]storage.StoredEvent, int, error) {
	return s.store.Events(caseID, offset, limit)
}
func (s *Service) AuditEvents(caseID string, filter storage.EventFilter) (storage.EventPage, error) {
	return s.store.QueryEvents(caseID, filter)
}
func (s *Service) Archive(caseID string) (archive.Document, []byte, error) {
	return s.archive.Read(caseID)
}
func (s *Service) VerifyArchive(caseID string) (archive.Verification, error) {
	return s.archive.Verify(caseID)
}
