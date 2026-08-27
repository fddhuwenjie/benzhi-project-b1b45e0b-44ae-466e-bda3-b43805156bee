package wrappedruleerrorhttpcode

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"icecoreverdict/internal/api"
	"icecoreverdict/internal/application"
	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/storage"
)

func TestWrappedRuleErrorPreservesHTTPCode(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(application.Services{Store: store, Archive: archive.New(store)})
	server := api.New(app, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := server.Handler()

	create := map[string]any{
		"request_id":       "create-0001",
		"case_id":          "case-wrap",
		"title":            "错误链测试",
		"transfer_batch":   "batch-1",
		"incident_summary": "用于验证跨层错误传播的污染事件",
		"lead_actor_id":    "lead",
		"created_by":       "creator",
	}
	body, _ := json.Marshal(create)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cases", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("创建案件失败: %d %s", resp.Code, resp.Body.String())
	}

	command := map[string]any{
		"request_id": "verify-0001",
		"action":     "verify_remediation",
		"actor_id":   "operator",
		"payload":    map[string]any{"action_id": "missing-action", "retest_evidence_ids": []string{"E1"}},
	}
	body, _ = json.Marshal(command)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/cases/case-wrap/commands", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp = httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusConflict {
		t.Fatalf("业务规则错误未保留 HTTP 409，得到 %d: %s", resp.Code, resp.Body.String())
	}
	var result struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error.Code != "state_conflict" {
		t.Fatalf("错误码未保留 state_conflict，得到 %q", result.Error.Code)
	}
}
