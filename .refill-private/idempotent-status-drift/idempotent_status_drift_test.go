package idempotentstatusdrift

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"icecoreverdict/internal/application"
	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

func TestIdempotentRetryMustPreserveFirstResponseStatus(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	created, err := domain.DecideCreate(domain.CreateCase{
		CaseID: "case-status", Title: "状态缓存", TransferBatch: "batch-1",
		IncidentSummary: "用于验证幂等重试响应与首次响应完全一致", LeadActorID: "lead", CreatedBy: "creator", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	freeze, err := domain.NewEvent("case-status", domain.EventBaselineFrozen, "creator", now.Add(time.Minute), domain.BaselineFrozenData{
		Samples:  []domain.IceCoreSample{{SampleID: "sample-1", CaseID: "case-status", CoreSegment: "10m", ContainerSeal: "seal-1", TransferTemperatureCelsius: -20, CustodyHolder: "holder", TransferBatch: "batch-1"}},
		FrozenAt: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := domain.NewEvent("case-status", domain.EventRemediationPlanned, "executor", now.Add(2*time.Minute), domain.RemediationPlannedData{Action: domain.RemediationAction{
		ActionID: "action-1", CaseID: "case-status", ActionType: "sample_isolation", TargetSampleIDs: []string{"sample-1"}, ExecutorID: "executor", WitnessID: "witness", Status: "planned",
	}})
	if err != nil {
		t.Fatal(err)
	}
	seed := append(created, freeze, planned)
	if _, err := store.Commit("case-status", "request-seed-001", 0, seed, 200, json.RawMessage(`{"revision":3}`)); err != nil {
		t.Fatal(err)
	}
	app := application.New(application.Services{Store: store, Archive: archive.New(store)})
	payload, err := json.Marshal(domain.AddEvidence{Evidence: domain.EvidenceRecord{
		EvidenceID: "evidence-1", SampleID: "sample-1", EvidenceType: "ion_metric", MetricName: "chloride", Value: 1,
		Unit: "ng/L", CollectedAt: now.Add(3 * time.Minute), CollectorID: "investigator", ContentDigest: "sha256:0123456789abcdef", CustodySequence: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := application.Command{RequestID: "request-evidence-001", Action: "add_evidence", ActorID: "investigator", Payload: payload}
	first, err := app.Execute(context.Background(), "case-status", cmd)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := app.Execute(context.Background(), "case-status", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != domain.StatusRemediation {
		t.Fatalf("测试前提失败，首次状态=%s", first.Status)
	}
	if retry.Status != first.Status {
		t.Fatalf("TestIdempotentRetryMustPreserveFirstResponseStatus: 首次状态=%s，重试状态=%s", first.Status, retry.Status)
	}
}
