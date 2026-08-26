package storage

import (
	"encoding/json"
	"testing"
	"time"

	"icecoreverdict/internal/domain"
)

func TestCommitRestartAndIdempotency(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	events, err := domain.DecideCreate(domain.CreateCase{CaseID: "case-1", Title: "事件", TransferBatch: "B1", IncidentSummary: "污染事件描述足够长且可供存储测试", LeadActorID: "lead", CreatedBy: "creator", CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	response := json.RawMessage(`{"case_id":"case-1","revision":1}`)
	first, err := store.Commit("case-1", "request-0001", 0, events, 201, response)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Commit("case-1", "request-0001", 0, events, 201, response)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != again.Revision {
		t.Fatal("幂等提交改变了修订")
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	agg, stored, err := reopened.Load("case-1")
	if err != nil {
		t.Fatal(err)
	}
	if agg.Case.Revision != 1 || len(stored) != 1 {
		t.Fatalf("恢复结果错误: %+v %d", agg.Case, len(stored))
	}
	record, ok, err := reopened.IdempotentResult("case-1", "request-0001")
	var decoded map[string]any
	decodeErr := json.Unmarshal(record.Response, &decoded)
	if err != nil || !ok || decodeErr != nil || decoded["case_id"] != "case-1" || decoded["revision"] != float64(1) {
		t.Fatalf("幂等响应未恢复: %s, %v", record.Response, err)
	}
}

func TestArchiveStateRejectsWrites(t *testing.T) {
	a := domain.NewAggregate()
	a.Case = domain.ContaminationCase{CaseID: "case-a", Status: domain.StatusArchived, Revision: 1}
	_, err := a.DecideFreeze(domain.FreezeBaseline{}, "actor")
	code, _, _ := domain.ErrorDetails(err)
	if code != domain.CodeArchived {
		t.Fatalf("期望归档拒写，得到 %v", err)
	}
}
