package checkpointstateforgery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

func TestCheckpointContentMustMatchEventHistory(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	events, err := domain.DecideCreate(domain.CreateCase{
		CaseID: "case-checkpoint", Title: "原始标题", TransferBatch: "batch-1",
		IncidentSummary: "用于验证检查点内容必须由事件历史完整推导", LeadActorID: "lead", CreatedBy: "creator",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit("case-checkpoint", "request-checkpoint-001", 0, events, 201, json.RawMessage(`{"revision":1}`)); err != nil {
		t.Fatal(err)
	}
	cpPath := filepath.Join(root, "cases", "case-checkpoint", "checkpoint.json")
	b, err := os.ReadFile(cpPath)
	if err != nil {
		t.Fatal(err)
	}
	var cp storage.Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		t.Fatal(err)
	}
	cp.Aggregate.Case.Title = "被污染但元数据仍匹配的标题"
	b, err = json.MarshalIndent(cp, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cpPath, b, 0600); err != nil {
		t.Fatal(err)
	}

	reopened, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	agg, _, err := reopened.Load("case-checkpoint")
	if err == nil && agg.Case.Title == "被污染但元数据仍匹配的标题" {
		t.Fatalf("TestCheckpointContentMustMatchEventHistory: 检查点污染绕过事件历史校验")
	}
}
