package corrupt_event_http_code

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"icecoreverdict/internal/api"
	"icecoreverdict/internal/application"
	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/domain"
	"icecoreverdict/internal/storage"
	"log/slog"
)

func TestCorruptEventMustMapToDataCorrupt(t *testing.T) {
	root := t.TempDir()
	store, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	events, err := domain.DecideCreate(domain.CreateCase{
		CaseID:          "case-corrupt",
		Title:           "摘要校验",
		TransferBatch:   "batch-1",
		IncidentSummary: "这是一个用于验证事件摘要错误映射的案件",
		LeadActorID:     "lead",
		CreatedBy:       "creator",
		CreatedAt:       createdAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit("case-corrupt", "request-create", 0, events, http.StatusCreated, []byte(`{"case_id":"case-corrupt"}`)); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "cases", "case-corrupt", "events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	needle := `"digest":"`
	idx := strings.Index(string(data), needle)
	if idx < 0 {
		t.Fatal("未找到事件摘要")
	}
	pos := idx + len(needle)
	if data[pos] == '0' {
		data[pos] = '1'
	} else {
		data[pos] = '0'
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	app := application.New(application.Services{Store: store, Archive: archive.New(store)})
	server := api.New(app, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cases/case-corrupt", nil).WithContext(context.Background())
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"data_corrupt"`) {
		t.Fatalf("摘要损坏应返回 503/data_corrupt，实际 %d: %s", rec.Code, rec.Body.String())
	}
}
