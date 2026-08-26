package domain

import (
	"math"
	"strings"
	"time"
)

var evidenceUnits = map[string]map[string]bool{
	"blank_control":        {"count": true, "ng/L": true},
	"ion_metric":           {"ng/L": true, "µg/L": true, "mg/L": true},
	"particle_metric":      {"count/mL": true, "µm": true},
	"container_inspection": {"score": true},
	"operation_log":        {"count": true},
}

func (a *Aggregate) DecideAddEvidence(cmd AddEvidence, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusBounded, StatusInvestigating, StatusRemediation); err != nil {
		return nil, err
	}
	evidence := cmd.Evidence
	if _, ok := a.Samples[evidence.SampleID]; !ok {
		return nil, invalid("sample_id", "样本不属于本案件")
	}
	if _, exists := a.Evidence[evidence.EvidenceID]; exists || !nonblank(evidence.EvidenceID) {
		return nil, invalid("evidence_id", "不能为空且不得重复")
	}
	units, ok := evidenceUnits[evidence.EvidenceType]
	if !ok {
		return nil, invalid("evidence_type", "不支持的证据类型")
	}
	if !units[evidence.Unit] {
		return nil, invalid("unit", "证据类型与单位不匹配")
	}
	if !nonblank(evidence.MetricName) || math.IsNaN(evidence.Value) || math.IsInf(evidence.Value, 0) {
		return nil, invalid("metric_name", "指标和值必须有效")
	}
	if !validTime(evidence.CollectedAt) || evidence.CollectedAt.Before(a.Case.BaselineFrozenAt.Add(-24*time.Hour)) {
		return nil, invalid("collected_at", "采集时间无效或早于允许窗口")
	}
	if !nonblank(evidence.CollectorID) {
		evidence.CollectorID = actor
	}
	if evidence.CollectorID != actor {
		return nil, invalid("collector_id", "必须与当前操作人一致")
	}
	if len(evidence.ContentDigest) < 16 {
		return nil, invalid("content_digest", "摘要长度不足")
	}
	if !strings.HasPrefix(evidence.ContentDigest, "sha256:") || !validDigestToken(strings.TrimPrefix(evidence.ContentDigest, "sha256:")) {
		return nil, invalid("content_digest", "必须使用 sha256:<摘要> 格式")
	}
	for _, existing := range a.Evidence {
		if existing.ContentDigest == evidence.ContentDigest {
			return nil, invalid("content_digest", "案件内证据内容摘要不得重复")
		}
		if existing.SampleID == evidence.SampleID && evidence.CollectedAt.Before(existing.CollectedAt) {
			return nil, invalid("collected_at", "同一样本的采集时间不得倒序")
		}
	}
	expected := a.LastCustodySequence[evidence.SampleID] + 1
	if evidence.CustodySequence != expected {
		return nil, invalid("custody_sequence", "保管链序号必须连续")
	}
	evidence.CaseID = a.Case.CaseID
	matrix := a.completenessFor(evidence.SampleID, &evidence)
	e, err := NewEvent(a.Case.CaseID, EventEvidenceAdded, actor, evidence.CollectedAt, EvidenceAddedData{Evidence: evidence, Completeness: matrix})
	return []Event{e}, err
}

func validDigestToken(value string) bool {
	if len(value) < 16 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func (a *Aggregate) DecideHypothesis(cmd EvaluateHypothesis, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusInvestigating); err != nil {
		return nil, err
	}
	if !nonblank(cmd.HypothesisID) || !nonblank(cmd.SourceCategory) || len(strings.TrimSpace(cmd.Statement)) < 8 {
		return nil, invalid("hypothesis", "编号、来源类别和完整陈述均为必填")
	}
	if _, exists := a.Hypotheses[cmd.HypothesisID]; exists {
		return nil, invalid("hypothesis_id", "不得重复")
	}
	if len(cmd.Relations) < 2 {
		return nil, invalid("relations", "至少需要两项证据关系")
	}
	support, refute := 0, 0
	links := make([]string, 0, len(cmd.Relations))
	labels := make([]string, 0, len(cmd.Relations))
	seen := map[string]bool{}
	for _, relation := range cmd.Relations {
		if seen[relation.EvidenceID] {
			return nil, invalid("relations", "同一证据不能重复关联")
		}
		seen[relation.EvidenceID] = true
		if _, ok := a.Evidence[relation.EvidenceID]; !ok {
			return nil, invalid("relations.evidence_id", "引用了不存在的证据")
		}
		switch relation.Relation {
		case "supports":
			support++
		case "refutes":
			refute++
		case "irrelevant":
		default:
			return nil, invalid("relations.relation", "仅支持 supports、refutes、irrelevant")
		}
		links = append(links, relation.EvidenceID)
		labels = append(labels, relation.Relation)
	}
	linkedSamples := map[string]bool{}
	for _, id := range links {
		linkedSamples[a.Evidence[id].SampleID] = true
	}
	missing := []string{}
	for sampleID := range linkedSamples {
		matrix := a.completenessFor(sampleID, nil)
		if len(matrix.BlockingIssues) > 0 || len(matrix.MissingCategories) > 0 {
			missing = append(missing, sampleID+":"+strings.Join(matrix.MissingCategories, ","))
		}
	}
	if len(missing) > 0 {
		return nil, invalid("relations", "关联样本证据不完整: "+strings.Join(missing, ";"))
	}
	if support > 0 && refute > 0 && cmd.Conclusion == "confirmed" {
		return nil, invalid("conclusion", "存在相互矛盾证据时不能确认为污染来源")
	}
	score := float64(support-refute) / float64(len(cmd.Relations))
	score = math.Round(score*1000) / 1000
	switch cmd.Conclusion {
	case "confirmed":
		if support < 2 || score < .5 {
			return nil, invalid("conclusion", "支持证据不足")
		}
	case "excluded":
		if refute < 1 {
			return nil, invalid("conclusion", "缺少反驳证据")
		}
	case "inconclusive":
	default:
		return nil, invalid("conclusion", "结论值无效")
	}
	if cmd.EvaluatedAt.IsZero() {
		cmd.EvaluatedAt = time.Now().UTC()
	}
	h := SourceHypothesis{HypothesisID: cmd.HypothesisID, CaseID: a.Case.CaseID, SourceCategory: cmd.SourceCategory, Statement: cmd.Statement, EvidenceLinks: links, RelationLabels: labels, ConfidenceScore: score, Conclusion: cmd.Conclusion, EvaluatedAt: cmd.EvaluatedAt.UTC(), SupportCount: support, RefuteCount: refute, IrrelevantCount: len(cmd.Relations) - support - refute, ConflictExplanation: strings.TrimSpace(cmd.ConflictExplanation)}
	for _, other := range a.Hypotheses {
		if other.SourceCategory != h.SourceCategory || nonblank(other.ConflictExplanation) || nonblank(h.ConflictExplanation) {
			continue
		}
		otherLabels := map[string]string{}
		for i, id := range other.EvidenceLinks {
			otherLabels[id] = other.RelationLabels[i]
		}
		for i, id := range h.EvidenceLinks {
			if (otherLabels[id] == "supports" && h.RelationLabels[i] == "refutes") || (otherLabels[id] == "refutes" && h.RelationLabels[i] == "supports") {
				if h.Conclusion == "confirmed" || other.Conclusion == "confirmed" {
					return nil, invalid("conflict_explanation", "冲突证据 "+id+" 未解释，不能确认来源")
				}
			}
		}
	}
	e, err := NewEvent(a.Case.CaseID, EventHypothesisEvaluated, actor, cmd.EvaluatedAt, HypothesisEvaluatedData{Hypothesis: h})
	return []Event{e}, err
}
