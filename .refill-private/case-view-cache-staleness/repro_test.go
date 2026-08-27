package repro

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

func TestCaseViewCacheInvalidatedAfterWrite(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(application.Services{Store: store, Archive: archive.New(store)})
	ctx := context.Background()
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := app.Create(ctx, "create-cache-0001", domain.CreateCase{
		CaseID:          "cache-case",
		Title:           "缓存一致性复现案件",
		TransferBatch:   "TB-CACHE-1",
		IncidentSummary: "用于复现查询投影缓存跨写入生命周期未失效。",
		LeadActorID:     "lead",
		CreatedBy:       "custodian",
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatal(err)
	}
	first, err := app.Get("cache-case")
	if err != nil {
		t.Fatal(err)
	}
	if first.Case.Status != domain.StatusDraft || first.Case.Revision != 1 {
		t.Fatalf("初始查询异常: 状态=%s 修订=%d", first.Case.Status, first.Case.Revision)
	}
	freeze := application.Command{
		RequestID: "freeze-cache-0001",
		Action:    "freeze_baseline",
		ActorID:   "custodian",
	}
	freeze.Payload = mustJSON(t, domain.FreezeBaseline{
		Samples:               []domain.IceCoreSample{{SampleID: "S-1", CoreSegment: "100m", ContainerSeal: "seal-1", TransferBatch: "TB-CACHE-1", TransferTemperatureCelsius: -20, CustodyHolder: "custodian"}},
		MinTemperatureCelsius: -30,
		MaxTemperatureCelsius: -10,
		FrozenAt:              createdAt.Add(time.Hour),
	})
	if _, err := app.Execute(ctx, "cache-case", freeze); err != nil {
		t.Fatal(err)
	}
	second, err := app.Get("cache-case")
	if err != nil {
		t.Fatal(err)
	}
	if second.Case.Status != domain.StatusBounded || second.Case.Revision != 2 {
		t.Fatalf("写入后查询仍返回旧投影: 状态=%s 修订=%d", second.Case.Status, second.Case.Revision)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
