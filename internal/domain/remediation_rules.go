package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func (a *Aggregate) hasSufficientHypothesis() bool {
	for _, h := range a.Hypotheses {
		if h.Conclusion == "confirmed" || (h.Conclusion == "inconclusive" && len(h.EvidenceLinks) >= 3) {
			return true
		}
	}
	return false
}

func (a *Aggregate) hypothesisGate() error {
	if !a.hasSufficientHypothesis() {
		return conflict("缺少已确认或证据充分但不确定的来源候选")
	}
	for _, h := range a.Hypotheses {
		if len(h.ConflictEvidenceIDs) > 0 {
			return conflict("候选来源存在未解释冲突证据: " + strings.Join(h.ConflictEvidenceIDs, ","))
		}
		if h.ConfidenceScore >= .5 && h.Conclusion == "inconclusive" && len(h.EvidenceLinks) < 3 {
			return conflict("高分替代候选尚无明确结论: " + h.HypothesisID)
		}
	}
	return nil
}

func (a *Aggregate) DecidePlanRemediation(cmd PlanRemediation, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusInvestigating, StatusRemediation); err != nil {
		return nil, err
	}
	if err := a.hypothesisGate(); err != nil {
		return nil, err
	}
	action := cmd.Action
	if !nonblank(action.ActionID) || !nonblank(action.ActionType) {
		return nil, invalid("action", "措施编号和类型不能为空")
	}
	if _, exists := a.Actions[action.ActionID]; exists {
		return nil, invalid("action_id", "不得重复")
	}
	switch action.ActionType {
	case "sample_isolation", "instrument_cleaning", "sample_retest":
	default:
		return nil, invalid("action_type", "不支持的措施类型")
	}
	if len(action.TargetSampleIDs) == 0 {
		return nil, invalid("target_sample_ids", "至少指定一支样本")
	}
	targets := map[string]bool{}
	for _, id := range action.TargetSampleIDs {
		if _, ok := a.Samples[id]; !ok {
			return nil, invalid("target_sample_ids", "包含未知样本")
		}
		if targets[id] {
			return nil, invalid("target_sample_ids", "目标样本不得重复")
		}
		targets[id] = true
	}
	if action.Threshold == nil && len(strings.TrimSpace(action.AcceptanceThreshold)) < 3 {
		return nil, invalid("threshold", "必须记录结构化复测阈值")
	}
	if action.Threshold != nil {
		t := action.Threshold
		if !nonblank(t.MetricName) {
			return nil, invalid("threshold.metric_name", "不能为空")
		}
		switch t.Comparator {
		case "<", "<=", ">", ">=", "==":
		default:
			return nil, invalid("threshold.comparator", "仅支持 <、<=、>、>=、==")
		}
		if math.IsNaN(t.Value) || math.IsInf(t.Value, 0) {
			return nil, invalid("threshold.value", "必须为有限数值")
		}
		validUnit := false
		for _, units := range evidenceUnits {
			if units[t.Unit] {
				validUnit = true
			}
		}
		if !validUnit {
			return nil, invalid("threshold.unit", "未知或不支持的单位")
		}
		knownMetric := false
		for _, evidence := range a.Evidence {
			if evidence.MetricName == t.MetricName && evidence.Unit == t.Unit {
				knownMetric = true
				break
			}
		}
		if !knownMetric {
			return nil, invalid("threshold.metric_name", "指标与单位必须来自本案件已登记证据")
		}
	}
	if !nonblank(action.ExecutorID) {
		action.ExecutorID = actor
	}
	if action.ExecutorID != actor {
		return nil, invalid("executor_id", "必须与操作人一致")
	}
	if !nonblank(action.WitnessID) || action.WitnessID == action.ExecutorID {
		return nil, invalid("witness_id", "见证人必须存在且不同于执行人")
	}
	action.CaseID = a.Case.CaseID
	action.Status = "planned"
	e, err := NewEvent(a.Case.CaseID, EventRemediationPlanned, actor, time.Now().UTC(), RemediationPlannedData{Action: action})
	return []Event{e}, err
}

func (a *Aggregate) DecideVerifyRemediation(cmd VerifyRemediation, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusRemediation); err != nil {
		return nil, err
	}
	action, ok := a.Actions[cmd.ActionID]
	if !ok {
		return nil, invalid("action_id", "措施不存在")
	}
	if action.Status == "verified" {
		return nil, conflict("措施已经验证")
	}
	if actor != action.WitnessID {
		return nil, invalid("actor_id", "仅登记的见证人可以确认验证")
	}
	if len(cmd.RetestEvidenceIDs) == 0 {
		return nil, invalid("retest_evidence_ids", "必须关联复测证据")
	}
	computedPassed := cmd.Passed
	var measured, difference float64
	reason := "按兼容文本阈值由见证人确认"
	covered := map[string]bool{}
	for _, id := range cmd.RetestEvidenceIDs {
		evidence, ok := a.Evidence[id]
		if !ok {
			return nil, invalid("retest_evidence_ids", "复测证据不存在")
		}
		if evidence.CollectedAt.Before(a.Case.BaselineFrozenAt) {
			return nil, invalid("retest_evidence_ids", "复测证据时间无效")
		}
		if !contains(action.TargetSampleIDs, evidence.SampleID) {
			return nil, invalid("retest_evidence_ids", "复测证据样本不在措施目标中")
		}
		covered[evidence.SampleID] = true
		if action.Threshold != nil {
			if evidence.MetricName != action.Threshold.MetricName {
				return nil, invalid("retest_evidence_ids", "复测指标与阈值指标不匹配")
			}
			if evidence.Unit != action.Threshold.Unit {
				return nil, invalid("retest_evidence_ids", "复测证据单位与阈值单位不兼容")
			}
			passed, diff, detail := compare(evidence.Value, *action.Threshold)
			if reason == "按兼容文本阈值由见证人确认" || !passed {
				measured, difference, reason = evidence.Value, diff, detail
			}
			if id == cmd.RetestEvidenceIDs[0] {
				computedPassed = passed
			} else {
				computedPassed = computedPassed && passed
			}
		}
	}
	for _, sampleID := range action.TargetSampleIDs {
		if !covered[sampleID] {
			return nil, invalid("retest_evidence_ids", "缺少目标样本复测证据: "+sampleID)
		}
	}
	if action.Threshold != nil && cmd.Passed != computedPassed {
		return nil, invalid("passed", fmt.Sprintf("客户端状态与自动判定相反；%s，差值 %.6g", reason, difference))
	}
	if cmd.VerifiedAt.IsZero() {
		cmd.VerifiedAt = time.Now().UTC()
	}
	e, err := NewEvent(a.Case.CaseID, EventRemediationVerified, actor, cmd.VerifiedAt, RemediationVerifiedData{ActionID: cmd.ActionID, EvidenceIDs: cmd.RetestEvidenceIDs, Passed: computedPassed, VerifiedAt: cmd.VerifiedAt, MeasuredValue: measured, Difference: difference, Reason: reason})
	return []Event{e}, err
}

func (a *Aggregate) DecideSubmitReview(cmd SubmitReview, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusRemediation, StatusInvestigating); err != nil {
		return nil, err
	}
	if len(a.Actions) == 0 {
		return nil, conflict("尚未制定处置措施")
	}
	if cmd.SubmittedAt.IsZero() {
		cmd.SubmittedAt = time.Now().UTC()
	}
	for _, item := range a.CorrectiveItems {
		if item.Status != "closed" {
			return nil, conflict("整改项尚未关闭: " + item.ItemID)
		}
		if cmd.SubmittedAt.After(item.DueAt) && !nonblank(item.OverdueExplanation) {
			return nil, conflict("逾期整改缺少说明: " + item.ItemID)
		}
	}
	for _, coverage := range a.RemediationCoverageMatrix() {
		if len(coverage.MissingActionTypes) > 0 {
			return nil, conflict("样本 " + coverage.SampleID + " 缺少措施: " + strings.Join(coverage.MissingActionTypes, ","))
		}
		if !coverage.LatestRetestPassed {
			return nil, conflict("样本 " + coverage.SampleID + " 最新复测未通过")
		}
	}
	e, err := NewEvent(a.Case.CaseID, EventReviewSubmitted, actor, cmd.SubmittedAt, struct {
		SubmittedAt time.Time `json:"submitted_at"`
	}{cmd.SubmittedAt})
	return []Event{e}, err
}
