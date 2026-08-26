package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

var evidenceCategories = []string{"blank_control", "ion_metric", "particle_metric", "container_inspection", "operation_log"}

func (f *ReviewFinding) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &f.Summary)
	}
	type plain ReviewFinding
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*plain)(f))
}

func (a *Aggregate) completenessFor(sampleID string, extra *EvidenceRecord) EvidenceCompleteness {
	r := EvidenceCompleteness{SampleID: sampleID, Counts: map[string]int{}, LatestCollectedAt: map[string]time.Time{}}
	all := make([]EvidenceRecord, 0)
	for _, e := range a.Evidence {
		if e.SampleID == sampleID {
			all = append(all, e)
		}
	}
	if extra != nil {
		all = append(all, *extra)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CustodySequence != all[j].CustodySequence {
			return all[i].CustodySequence < all[j].CustodySequence
		}
		return all[i].EvidenceID < all[j].EvidenceID
	})
	seenDigest := map[string]bool{}
	var lastTime time.Time
	var lastSeq uint64
	for _, e := range all {
		r.Counts[e.EvidenceType]++
		if e.CollectedAt.After(r.LatestCollectedAt[e.EvidenceType]) {
			r.LatestCollectedAt[e.EvidenceType] = e.CollectedAt
		}
		if seenDigest[e.ContentDigest] {
			r.BlockingIssues = append(r.BlockingIssues, "重复内容摘要:"+e.ContentDigest)
		}
		seenDigest[e.ContentDigest] = true
		if !lastTime.IsZero() && e.CollectedAt.Before(lastTime) {
			r.BlockingIssues = append(r.BlockingIssues, "采集时间倒序:"+e.EvidenceID)
		}
		if lastSeq != 0 && e.CustodySequence != lastSeq+1 {
			r.BlockingIssues = append(r.BlockingIssues, "保管链序号空洞:"+e.EvidenceID)
		}
		lastTime, lastSeq = e.CollectedAt, e.CustodySequence
		if units, ok := evidenceUnits[e.EvidenceType]; !ok || !units[e.Unit] {
			r.BlockingIssues = append(r.BlockingIssues, "单位与指标不匹配:"+e.EvidenceID)
		}
	}
	for _, category := range evidenceCategories {
		if r.Counts[category] == 0 {
			r.MissingCategories = append(r.MissingCategories, category)
		}
	}
	r.CompletionRate = math.Round(float64(len(evidenceCategories)-len(r.MissingCategories))/float64(len(evidenceCategories))*10000) / 100
	return r
}

func (a *Aggregate) EvidenceCompletenessMatrix() ([]EvidenceCompleteness, float64) {
	result := make([]EvidenceCompleteness, 0, len(a.Samples))
	var total float64
	for _, id := range sortedKeys(a.Samples) {
		r := a.completenessFor(id, nil)
		result = append(result, r)
		total += r.CompletionRate
	}
	if len(result) == 0 {
		return result, 0
	}
	return result, math.Round(total/float64(len(result))*100) / 100
}

func (a *Aggregate) recomputeHypothesisComparison() {
	ids := sortedKeys(a.Hypotheses)
	conflicts := map[string]map[string]bool{}
	for _, id := range ids {
		conflicts[id] = map[string]bool{}
	}
	for i, leftID := range ids {
		left := a.Hypotheses[leftID]
		for _, rightID := range ids[i+1:] {
			right := a.Hypotheses[rightID]
			if left.SourceCategory != right.SourceCategory || nonblank(left.ConflictExplanation) || nonblank(right.ConflictExplanation) {
				continue
			}
			leftLabels := map[string]string{}
			for j, eid := range left.EvidenceLinks {
				leftLabels[eid] = left.RelationLabels[j]
			}
			for j, eid := range right.EvidenceLinks {
				if (leftLabels[eid] == "supports" && right.RelationLabels[j] == "refutes") || (leftLabels[eid] == "refutes" && right.RelationLabels[j] == "supports") {
					conflicts[leftID][eid], conflicts[rightID][eid] = true, true
				}
			}
		}
	}
	for _, id := range ids {
		h := a.Hypotheses[id]
		h.ConflictEvidenceIDs = sortedBoolKeys(conflicts[id])
		a.Hypotheses[id] = h
	}
	sort.Slice(ids, func(i, j int) bool {
		a1, a2 := a.Hypotheses[ids[i]], a.Hypotheses[ids[j]]
		if a1.ConfidenceScore != a2.ConfidenceScore {
			return a1.ConfidenceScore > a2.ConfidenceScore
		}
		if a1.SupportCount != a2.SupportCount {
			return a1.SupportCount > a2.SupportCount
		}
		return a1.HypothesisID < a2.HypothesisID
	})
	for i, id := range ids {
		h := a.Hypotheses[id]
		h.Rank = i + 1
		if len(ids) > 1 && i == 0 {
			h.LeadMargin = math.Round((h.ConfidenceScore-a.Hypotheses[ids[1]].ConfidenceScore)*1000) / 1000
		} else if i > 0 {
			h.LeadMargin = math.Round((a.Hypotheses[ids[0]].ConfidenceScore-h.ConfidenceScore)*1000) / 1000
		} else {
			h.LeadMargin = 0
		}
		a.Hypotheses[id] = h
	}
}

func sortedBoolKeys(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func (a *Aggregate) RemediationCoverageMatrix() []RemediationCoverage {
	result := make([]RemediationCoverage, 0, len(a.Samples))
	for _, sampleID := range sortedKeys(a.Samples) {
		r := RemediationCoverage{SampleID: sampleID}
		var latest time.Time
		for _, action := range a.Actions {
			if !contains(action.TargetSampleIDs, sampleID) {
				continue
			}
			switch action.ActionType {
			case "sample_isolation":
				r.Isolation = r.Isolation || action.Status == "verified"
			case "instrument_cleaning":
				r.InstrumentCleaning = r.InstrumentCleaning || action.Status == "verified"
			case "sample_retest":
				r.Retest = r.Retest || action.Status == "verified" || action.Status == "failed"
				if action.VerifiedAt.After(latest) {
					latest, r.LatestRetestPassed = action.VerifiedAt, action.Status == "verified"
				}
			}
		}
		if !r.Isolation {
			r.MissingActionTypes = append(r.MissingActionTypes, "sample_isolation")
		}
		if !r.InstrumentCleaning {
			r.MissingActionTypes = append(r.MissingActionTypes, "instrument_cleaning")
		}
		if !r.Retest {
			r.MissingActionTypes = append(r.MissingActionTypes, "sample_retest")
		}
		result = append(result, r)
	}
	return result
}

func contains(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}

func compare(value float64, threshold RetestThreshold) (bool, float64, string) {
	var passed bool
	switch threshold.Comparator {
	case "<":
		passed = value < threshold.Value
	case "<=":
		passed = value <= threshold.Value
	case ">":
		passed = value > threshold.Value
	case ">=":
		passed = value >= threshold.Value
	case "==":
		passed = value == threshold.Value
	}
	difference := value - threshold.Value
	return passed, difference, fmt.Sprintf("实测 %.6g %s，规则 %s %.6g %s", value, threshold.Unit, threshold.Comparator, threshold.Value, threshold.Unit)
}

func normalizeScopes(values []string) ([]string, error) {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, invalid("allowed_research_scope", "范围不能为空")
		}
		if seen[value] {
			return nil, invalid("allowed_research_scope", "范围不得重复")
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
