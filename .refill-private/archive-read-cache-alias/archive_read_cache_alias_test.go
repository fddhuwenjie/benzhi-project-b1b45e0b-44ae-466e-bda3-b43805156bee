package archive_read_cache_alias_test

import (
	"encoding/json"
	"testing"
	"time"

	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
)

func TestArchiveReadMustNotPoisonVerification(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	caseID := "archive-alias-case"
	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	created, err := domain.DecideCreate(domain.CreateCase{
		CaseID:                caseID,
		Title:                 "档案缓存所有权测试",
		TransferBatch:         "batch-archive-alias",
		IncidentSummary:       "用于验证档案读取结果不会反向污染校验缓存。",
		LeadActorID:           "lead-archive",
		CreatedBy:             "creator-archive",
		CreatedAt:             at,
		AllowedResearchScopes: []string{"climate-analysis"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit(caseID, "request-create-alias", 0, created, 201, json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	archived, err := domain.NewEvent(caseID, domain.EventCaseArchived, "archiver", at.Add(time.Hour), domain.ArchivedData{ArchivedAt: at.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit(caseID, "request-archive-alias", 1, []domain.Event{archived}, 200, json.RawMessage(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}

	archives := archive.New(store)
	if _, err := archives.Save(caseID); err != nil {
		t.Fatal(err)
	}
	read, _, err := archives.Read(caseID)
	if err != nil {
		t.Fatal(err)
	}
	read.Case.AllowedResearchScopes[0] = "caller-local-mutation"

	verification, err := archives.Verify(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid {
		t.Fatalf("TestArchiveReadMustNotPoisonVerification: 修改读取结果污染了后续校验")
	}
}
