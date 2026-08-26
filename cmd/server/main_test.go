package main

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"

	"icecoreverdict/internal/api"
	"icecoreverdict/internal/application"
	"icecoreverdict/internal/archive"
	"icecoreverdict/internal/storage"
)

func TestRealHTTPWorkflow(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(application.Services{Store: store, Archive: archive.New(store)})
	server := httptest.NewServer(api.New(app, slog.New(slog.NewTextHandler(os.Stderr, nil))).Handler())
	defer server.Close()
	if err := runSelfCheck(server.URL); err != nil {
		t.Fatal(err)
	}
	view, err := app.Get("self-check-case")
	if err != nil {
		t.Fatal(err)
	}
	if view.Case.Status != "archived" {
		t.Fatalf("状态为 %s", view.Case.Status)
	}
	if len(view.Samples) != 1 || view.Samples[0].Disposition != "limited_research" {
		t.Fatalf("样本裁定未保存: %+v", view.Samples)
	}
	if view.OverallEvidenceCompletionRate != 100 || len(view.EvidenceCompletenessMatrix) != 1 {
		t.Fatalf("证据完整性矩阵错误: %+v", view.EvidenceCompletenessMatrix)
	}
	verification, err := app.VerifyArchive("self-check-case")
	if err != nil || !verification.Valid || len(verification.Sections) != 6 {
		t.Fatalf("分区档案校验失败: %+v %v", verification, err)
	}
	_, first, err := app.Archive("self-check-case")
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := app.Archive("self-check-case")
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("重复下载的档案内容不稳定")
	}
	page, err := app.AuditEvents("self-check-case", storage.EventFilter{EventType: "review_approved", ActorID: "reviewer-independent", Limit: 10})
	if err != nil || page.MatchedTotal != 1 || len(page.StatusTrajectory) < 2 {
		t.Fatalf("事件筛选或状态轨迹错误: %+v %v", page, err)
	}
}

func TestConfiguredAddr(t *testing.T) {
	t.Setenv("PORT", "")
	got, err := configuredAddr(defaultAddr)
	if err != nil || got != defaultAddr {
		t.Fatalf("默认地址 %q, %v", got, err)
	}
	t.Setenv("PORT", "19999")
	got, err = configuredAddr(defaultAddr)
	if err != nil || got != "127.0.0.1:19999" {
		t.Fatalf("PORT 地址 %q, %v", got, err)
	}
	if _, err := configuredAddr("0.0.0.0:19091"); err == nil {
		t.Fatal("应拒绝非回环监听")
	}
	t.Setenv("PORT", "80")
	if _, err := configuredAddr(defaultAddr); err == nil {
		t.Fatal("应拒绝低位 PORT")
	}
}
