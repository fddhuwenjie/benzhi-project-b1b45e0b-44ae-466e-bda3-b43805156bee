package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (a *Aggregate) reviewerConflict(reviewer string) bool {
	return reviewer == a.Case.CreatedBy || reviewer == a.Case.LeadActorID || a.Investigators[reviewer] || a.Witnesses[reviewer]
}

func (a *Aggregate) DecideReview(cmd CompleteReview) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusPendingReview); err != nil {
		return nil, err
	}
	if !nonblank(cmd.ReviewID) || !nonblank(cmd.ReviewerID) {
		return nil, invalid("review", "复核编号和复核员不能为空")
	}
	if a.reviewerConflict(cmd.ReviewerID) {
		return nil, &RuleError{Code: CodeDutyConflict, Field: "reviewer_id", Message: "复核员与建档、调查或措施见证职责冲突"}
	}
	if len(a.Reviews) >= 2 {
		return nil, conflict("只允许一次驳回整改和一次重新复核")
	}
	if cmd.SignedAt.IsZero() {
		cmd.SignedAt = time.Now().UTC()
	}
	review := ReviewDecision{ReviewID: cmd.ReviewID, CaseID: a.Case.CaseID, ReviewerID: cmd.ReviewerID, ConflictCheck: "passed", Outcome: cmd.Outcome, Findings: append([]ReviewFinding(nil), cmd.Findings...), SignedAt: cmd.SignedAt.UTC()}
	var typ string
	var items []CorrectiveItem
	switch cmd.Outcome {
	case "approved":
		typ = EventReviewApproved
	case "rejected":
		if len(a.Reviews) > 0 {
			return nil, conflict("整改后复核不得再次驳回")
		}
		if len(cmd.Findings) == 0 {
			return nil, invalid("findings", "驳回必须给出整改发现")
		}
		seenSummary, seenID := map[string]bool{}, map[string]bool{}
		for i := range review.Findings {
			finding := &review.Findings[i]
			finding.Summary = strings.TrimSpace(finding.Summary)
			if finding.Summary == "" || seenSummary[finding.Summary] {
				return nil, invalid("findings", "发现不能为空或重复")
			}
			seenSummary[finding.Summary] = true
			if finding.FindingID == "" {
				finding.FindingID = fmt.Sprintf("%s-F%03d", cmd.ReviewID, i+1)
			}
			if seenID[finding.FindingID] {
				return nil, invalid("findings.finding_id", "整改项编号不得重复")
			}
			seenID[finding.FindingID] = true
			if finding.Severity == "" {
				finding.Severity = "medium"
			}
			switch finding.Severity {
			case "low", "medium", "high", "critical":
			default:
				return nil, invalid("findings.severity", "严重级别无效")
			}
			if finding.AssigneeID == "" {
				finding.AssigneeID = a.Case.LeadActorID
			}
			if finding.DueAt.IsZero() {
				finding.DueAt = cmd.SignedAt.Add(7 * 24 * time.Hour)
			}
			if !finding.DueAt.After(cmd.SignedAt) {
				return nil, invalid("findings.due_at", "期限必须晚于驳回时间")
			}
			items = append(items, CorrectiveItem{ItemID: finding.FindingID, ReviewID: cmd.ReviewID, Summary: finding.Summary, Severity: finding.Severity, AssigneeID: finding.AssigneeID, DueAt: finding.DueAt.UTC(), Status: "open", RejectedAt: cmd.SignedAt.UTC(), RejectedRevision: a.Case.Revision + 1})
		}
		typ = EventReviewRejected
	default:
		return nil, invalid("outcome", "仅支持 approved 或 rejected")
	}
	e, err := NewEvent(a.Case.CaseID, typ, cmd.ReviewerID, cmd.SignedAt, ReviewStateData{Review: review, CorrectiveItems: items})
	return []Event{e}, err
}

func (a *Aggregate) DecideCloseCorrective(cmd CloseCorrectiveItem, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusInvestigating, StatusRemediation); err != nil {
		return nil, err
	}
	item, ok := a.CorrectiveItems[cmd.ItemID]
	if !ok {
		return nil, invalid("item_id", "整改项不存在")
	}
	if item.Status == "closed" {
		return nil, conflict("整改项已经关闭")
	}
	if actor != item.AssigneeID {
		return nil, invalid("actor_id", "仅整改责任人可以关闭")
	}
	if !nonblank(cmd.Resolution) {
		return nil, invalid("resolution", "整改说明不能为空")
	}
	if len(cmd.RelatedEvidenceIDs)+len(cmd.RelatedActionIDs) == 0 {
		return nil, invalid("related_objects", "至少关联一项驳回后证据或措施")
	}
	for _, id := range cmd.RelatedEvidenceIDs {
		if _, ok := a.Evidence[id]; !ok || a.EvidenceRevisions[id] <= item.RejectedRevision {
			return nil, invalid("related_evidence_ids", "关联证据必须属于本案件且产生于驳回之后")
		}
	}
	for _, id := range cmd.RelatedActionIDs {
		if _, ok := a.Actions[id]; !ok || a.ActionRevisions[id] <= item.RejectedRevision {
			return nil, invalid("related_action_ids", "关联措施必须属于本案件且产生于驳回之后")
		}
	}
	if cmd.ClosedAt.IsZero() {
		cmd.ClosedAt = time.Now().UTC()
	}
	if !cmd.ClosedAt.After(item.RejectedAt) {
		return nil, invalid("closed_at", "关闭时间必须晚于驳回时间")
	}
	if cmd.ClosedAt.After(item.DueAt) && !nonblank(cmd.OverdueExplanation) {
		return nil, invalid("overdue_explanation", "逾期整改必须说明原因")
	}
	item.Status, item.Resolution, item.ClosedBy, item.ClosedAt = "closed", strings.TrimSpace(cmd.Resolution), actor, cmd.ClosedAt.UTC()
	item.RelatedEvidenceIDs, item.RelatedActionIDs = uniqueSorted(cmd.RelatedEvidenceIDs), uniqueSorted(cmd.RelatedActionIDs)
	item.OverdueExplanation = strings.TrimSpace(cmd.OverdueExplanation)
	e, err := NewEvent(a.Case.CaseID, EventCorrectiveItemClosed, actor, cmd.ClosedAt, CorrectiveItemClosedData{Item: item})
	return []Event{e}, err
}

func (a *Aggregate) DecideDispositions(cmd RecordDispositions) ([]Event, error) {
	_, normalized, err := a.PrecheckDispositions(cmd)
	if err != nil {
		return nil, err
	}
	cmd.Decisions = normalized
	if cmd.SignedAt.IsZero() {
		cmd.SignedAt = time.Now().UTC()
	}
	e, err := NewEvent(a.Case.CaseID, EventDispositionsRecorded, cmd.SignerID, cmd.SignedAt, DispositionsData{SignerID: cmd.SignerID, Decisions: cmd.Decisions, SignedAt: cmd.SignedAt})
	return []Event{e}, err
}

func (a *Aggregate) PrecheckDispositions(cmd RecordDispositions) ([]DispositionPrecheck, []SampleDecision, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, nil, err
	}
	if err := a.expect(StatusPendingReview); err != nil {
		return nil, nil, err
	}
	if len(a.Reviews) == 0 || a.Reviews[len(a.Reviews)-1].Outcome != "approved" {
		return nil, nil, conflict("独立复核尚未通过")
	}
	if cmd.SignerID != a.Reviews[len(a.Reviews)-1].ReviewerID {
		return nil, nil, invalid("signer_id", "用途裁定须由通过复核的复核员签署")
	}
	if len(cmd.Decisions) != len(a.Samples) {
		return nil, nil, invalid("decisions", "必须覆盖案件内每支样本")
	}
	seen := map[string]bool{}
	result := make([]DispositionPrecheck, 0, len(cmd.Decisions))
	normalized := append([]SampleDecision(nil), cmd.Decisions...)
	allowed := map[string]bool{}
	for _, scope := range a.Case.AllowedResearchScopes {
		allowed[scope] = true
	}
	for i := range normalized {
		d := &normalized[i]
		if _, ok := a.Samples[d.SampleID]; !ok || seen[d.SampleID] {
			return nil, nil, invalid("decisions.sample_id", "样本未知或重复")
		}
		seen[d.SampleID] = true
		if !nonblank(d.Reason) {
			return nil, nil, invalid("decisions.reason", "每项裁定必须说明理由")
		}
		var err error
		d.AllowedResearchScope, err = normalizeScopes(d.AllowedResearchScope)
		if err != nil {
			return nil, nil, err
		}
		pre := DispositionPrecheck{SampleID: d.SampleID, Signable: true, BasisReferences: uniqueSorted(d.BasisReferences)}
		pre.BasisReferences = uniqueSorted(append(pre.BasisReferences, evidenceIDsForSample(a.Evidence, d.SampleID)...))
		for _, hypothesis := range a.Hypotheses {
			for _, evidenceID := range hypothesis.EvidenceLinks {
				if evidence, ok := a.Evidence[evidenceID]; ok && evidence.SampleID == d.SampleID {
					pre.BasisReferences = append(pre.BasisReferences, hypothesis.HypothesisID)
					break
				}
			}
		}
		for _, action := range a.Actions {
			if contains(action.TargetSampleIDs, d.SampleID) {
				pre.BasisReferences = append(pre.BasisReferences, action.ActionID)
			}
		}
		for _, review := range a.Reviews {
			pre.BasisReferences = append(pre.BasisReferences, review.ReviewID)
		}
		pre.BasisReferences = uniqueSorted(pre.BasisReferences)
		for _, action := range a.Actions {
			if contains(action.TargetSampleIDs, d.SampleID) && action.Status == "failed" {
				pre.UnresolvedRisks = append(pre.UnresolvedRisks, "失败处置:"+action.ActionID)
			}
		}
		for _, item := range a.CorrectiveItems {
			if (item.Severity == "high" || item.Severity == "critical") && item.Status != "closed" {
				pre.UnresolvedRisks = append(pre.UnresolvedRisks, "未决高风险发现:"+item.ItemID)
			}
		}
		for _, hypothesis := range a.Hypotheses {
			if len(hypothesis.ConflictEvidenceIDs) > 0 {
				pre.UnresolvedRisks = append(pre.UnresolvedRisks, "来源冲突:"+hypothesis.HypothesisID)
			}
		}
		switch d.Disposition {
		case DispositionFull:
			if len(d.AllowedResearchScope) != 0 {
				return nil, nil, invalid("allowed_research_scope", "全部研究不应设置限定范围")
			}
			if len(pre.UnresolvedRisks) > 0 {
				return nil, nil, invalid("disposition", "全部研究与样本风险冲突: "+strings.Join(pre.UnresolvedRisks, ","))
			}
		case DispositionLimited:
			if len(d.AllowedResearchScope) == 0 {
				return nil, nil, invalid("allowed_research_scope", "限用途裁定必须指定研究范围")
			}
			for _, scope := range d.AllowedResearchScope {
				if len(allowed) > 0 && !allowed[scope] {
					return nil, nil, invalid("allowed_research_scope", "范围不在案件允许研究词表中: "+scope)
				}
			}
		case DispositionRejected:
			if len(d.AllowedResearchScope) != 0 {
				return nil, nil, invalid("allowed_research_scope", "不可用样本不能设置研究范围")
			}
			if len(d.BasisReferences) == 0 {
				return nil, nil, invalid("basis_references", "不可研究理由必须引用证据或复核发现")
			}
			for _, ref := range d.BasisReferences {
				if _, ok := a.Evidence[ref]; !ok {
					if _, ok := a.CorrectiveItems[ref]; !ok {
						return nil, nil, invalid("basis_references", "引用对象不属于本案件: "+ref)
					}
				}
			}
		default:
			return nil, nil, invalid("disposition", "裁定值无效")
		}
		d.BasisReferences = append([]string(nil), pre.BasisReferences...)
		result = append(result, pre)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SampleID < result[j].SampleID })
	return result, normalized, nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}
func evidenceIDsForSample(values map[string]EvidenceRecord, sampleID string) []string {
	result := []string{}
	for id, e := range values {
		if e.SampleID == sampleID {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

func (a *Aggregate) DecideArchive(cmd ArchiveCase, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusDecided); err != nil {
		return nil, err
	}
	if cmd.ArchivedAt.IsZero() {
		cmd.ArchivedAt = time.Now().UTC()
	}
	e, err := NewEvent(a.Case.CaseID, EventCaseArchived, actor, cmd.ArchivedAt, ArchivedData{ArchivedAt: cmd.ArchivedAt})
	return []Event{e}, err
}
