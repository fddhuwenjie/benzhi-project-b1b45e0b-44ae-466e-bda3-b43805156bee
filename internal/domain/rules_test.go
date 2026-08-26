package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func testInvestigated(t *testing.T) *Aggregate {
	t.Helper()
	now := time.Now().UTC()
	created, _ := DecideCreate(CreateCase{CaseID: "case-1", Title: "污染", TransferBatch: "batch", IncidentSummary: "污染事件描述足够长且可供测试", LeadActorID: "lead", CreatedBy: "creator", CreatedAt: now})
	created[0].Revision = 1
	a, err := Rehydrate(created)
	if err != nil {
		t.Fatal(err)
	}
	freeze, err := a.DecideFreeze(FreezeBaseline{Samples: []IceCoreSample{{SampleID: "S1", CoreSegment: "10m", ContainerSeal: "seal", TransferTemperatureCelsius: -20, CustodyHolder: "holder"}}, FrozenAt: now.Add(time.Minute), MinTemperatureCelsius: -30, MaxTemperatureCelsius: -10}, "creator")
	if err != nil {
		t.Fatal(err)
	}
	freeze[0].Revision = 2
	if err := a.Apply(freeze[0]); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		e, err := a.DecideAddEvidence(AddEvidence{Evidence: EvidenceRecord{EvidenceID: map[int]string{1: "E1", 2: "E2"}[i], SampleID: "S1", EvidenceType: "ion_metric", MetricName: "chloride", Value: float64(i), Unit: "ng/L", CollectedAt: now.Add(time.Duration(i+1) * time.Minute), CollectorID: "investigator", ContentDigest: map[int]string{1: "sha256:0123456789abcdef", 2: "sha256:fedcba9876543210"}[i], CustodySequence: uint64(i)}}, "investigator")
		if err != nil {
			t.Fatal(err)
		}
		e[0].Revision = a.Case.Revision + 1
		if err := a.Apply(e[0]); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

func TestEvidenceCustodyAndHypothesisRules(t *testing.T) {
	a := testInvestigated(t)
	bad, err := a.DecideAddEvidence(AddEvidence{Evidence: EvidenceRecord{EvidenceID: "E3", SampleID: "S1", EvidenceType: "ion_metric", MetricName: "chloride", Value: 3, Unit: "ng/L", CollectedAt: time.Now().UTC(), CollectorID: "investigator", ContentDigest: "sha256:0123456789abcdef", CustodySequence: 4}}, "investigator")
	if err == nil || bad != nil {
		t.Fatal("应拒绝断裂的保管链")
	}
	_, err = a.DecideHypothesis(EvaluateHypothesis{HypothesisID: "H1", SourceCategory: "tool", Statement: "器具残留导致污染", Relations: []EvidenceRelation{{EvidenceID: "E1", Relation: "supports"}, {EvidenceID: "E2", Relation: "refutes"}}, Conclusion: "confirmed"}, "investigator")
	if err == nil {
		t.Fatal("应拒绝矛盾证据下的确认结论")
	}
}

func TestReviewerDutyConflict(t *testing.T) {
	a := testInvestigated(t)
	a.Case.Status = StatusPendingReview
	a.Investigators["investigator"] = true
	_, err := a.DecideReview(CompleteReview{ReviewID: "R1", ReviewerID: "investigator", Outcome: "approved"})
	code, _, _ := ErrorDetails(err)
	if code != CodeDutyConflict {
		t.Fatalf("期望职责冲突，得到 %v", err)
	}
	events, err := a.DecideReview(CompleteReview{ReviewID: "R1", ReviewerID: "independent", Outcome: "approved"})
	if err != nil || events[0].Type != EventReviewApproved {
		t.Fatalf("独立复核应通过: %v", err)
	}
}

func TestDispositionCompleteness(t *testing.T) {
	a := testInvestigated(t)
	a.Case.Status = StatusPendingReview
	a.Reviews = []ReviewDecision{{ReviewerID: "reviewer", Outcome: "approved"}}
	_, err := a.DecideDispositions(RecordDispositions{SignerID: "reviewer", Decisions: nil})
	if err == nil {
		t.Fatal("应拒绝缺失逐样本裁定")
	}
	_, err = a.DecideDispositions(RecordDispositions{SignerID: "reviewer", Decisions: []SampleDecision{{SampleID: "S1", Disposition: DispositionLimited, Reason: "受限"}}})
	if err == nil {
		t.Fatal("限用途裁定必须给出范围")
	}
}

func TestBaselineReceiptAndDuplicateSeal(t *testing.T) {
	now := time.Now().UTC()
	created, _ := DecideCreate(CreateCase{CaseID: "case-baseline", Title: "基线", TransferBatch: "B-1", IncidentSummary: "用于验证冻结基线完整业务规则", LeadActorID: "lead", CreatedBy: "creator", CreatedAt: now})
	created[0].Revision = 1
	a, err := Rehydrate(created)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := FreezeBaseline{MinTemperatureCelsius: -30, MaxTemperatureCelsius: -15, Samples: []IceCoreSample{{SampleID: "S1", CoreSegment: "1m", ContainerSeal: "seal", CustodyHolder: "holder", TransferTemperatureCelsius: -20}, {SampleID: "S2", CoreSegment: "2m", ContainerSeal: "seal", CustodyHolder: "holder", TransferTemperatureCelsius: -20}}}
	if events, err := a.DecideFreeze(duplicate, "creator"); err == nil || events != nil {
		t.Fatal("应原子拒绝重复封签")
	}
	valid := duplicate
	valid.Samples[1].ContainerSeal = "seal-2"
	valid.Samples[1].TransferTemperatureCelsius = -10
	valid.Samples[1].TemperatureException = "转运途中制冷单元报警"
	events, err := a.DecideFreeze(valid, "creator")
	if err != nil {
		t.Fatal(err)
	}
	var data BaselineFrozenData
	if err := json.Unmarshal(events[0].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Receipt.OutOfRangeCount != 1 || data.Receipt.NormalCount != 1 || data.Receipt.FrozenRevision != 2 {
		t.Fatalf("冻结回执错误: %+v", data.Receipt)
	}
}
