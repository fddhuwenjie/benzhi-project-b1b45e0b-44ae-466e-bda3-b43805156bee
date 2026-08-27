package auditpagecachealias

import (
	"encoding/json"
	"testing"
	"time"

	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

func TestAuditFilterCacheIsolatedByActor(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	created, err := domain.DecideCreate(domain.CreateCase{
		CaseID: "case-audit-page", Title: "分页缓存", TransferBatch: "batch-1",
		IncidentSummary: "用于验证事件审计分页缓存隔离的完整描述", LeadActorID: "lead", CreatedBy: "creator", CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	created[0].Revision = 1
	if _, err := store.Commit("case-audit-page", "request-create-1", 0, created, 201, json.RawMessage(`{"revision":1}`)); err != nil {
		t.Fatal(err)
	}
	agg, err := domain.Rehydrate(created)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := agg.DecideFreeze(domain.FreezeBaseline{
		Samples:  []domain.IceCoreSample{{SampleID: "sample-1", CoreSegment: "10m", ContainerSeal: "seal-1", TransferBatch: "batch-1", CustodyHolder: "holder", TransferTemperatureCelsius: -20}},
		FrozenAt: now.Add(time.Minute), MinTemperatureCelsius: -30, MaxTemperatureCelsius: -10,
	}, "freezer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit("case-audit-page", "request-freeze-1", 1, frozen, 200, json.RawMessage(`{"revision":2}`)); err != nil {
		t.Fatal(err)
	}

	first, err := store.QueryEvents("case-audit-page", storage.EventFilter{ActorID: "creator", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.QueryEvents("case-audit-page", storage.EventFilter{ActorID: "freezer", Limit: 10, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Events) != 1 || first.Events[0].ActorID != "creator" {
		t.Fatalf("首个操作者筛选错误: %+v", first)
	}
	if len(second.Events) != 1 || second.Events[0].ActorID != "freezer" {
		t.Fatalf("审计筛选缓存串扰: %+v", second)
	}
}
