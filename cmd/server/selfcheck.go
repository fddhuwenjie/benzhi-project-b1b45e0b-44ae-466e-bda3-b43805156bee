package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func runSelfCheck(base string) error {
	client := &http.Client{Timeout: 10 * time.Second}
	now := time.Now().UTC().Truncate(time.Second)
	caseID := "self-check-case"
	create := map[string]any{"request_id": "self-create-0001", "case_id": caseID, "title": "自检污染事件", "transfer_batch": "TB-SC-001", "incident_summary": "自检发现离子及微粒指标异常，需要完成闭环裁定。", "lead_actor_id": "lead-a", "created_by": "custodian-a", "created_at": now}
	if err := post(client, base+"/api/v1/cases", create, 201); err != nil {
		return err
	}
	samples := []map[string]any{{"sample_id": "S-001", "core_segment": "深度 101.2-101.4m", "container_seal": "SEAL-001", "transfer_temperature_celsius": -22.4, "custody_holder": "custodian-a"}}
	if err := command(client, base, caseID, "self-freeze-0001", "freeze_baseline", "custodian-a", map[string]any{"samples": samples, "frozen_at": now.Add(time.Minute), "min_temperature_celsius": -30.0, "max_temperature_celsius": -15.0}); err != nil {
		return err
	}
	evidence := []map[string]any{{"evidence_id": "E-001", "sample_id": "S-001", "evidence_type": "ion_metric", "metric_name": "chloride", "value": 18.2, "unit": "ng/L", "collected_at": now.Add(2 * time.Minute), "collector_id": "investigator-a", "content_digest": "sha256:evidence-0000000000000001", "custody_sequence": 1}, {"evidence_id": "E-002", "sample_id": "S-001", "evidence_type": "particle_metric", "metric_name": "particles", "value": 42, "unit": "count/mL", "collected_at": now.Add(3 * time.Minute), "collector_id": "investigator-a", "content_digest": "sha256:evidence-0000000000000002", "custody_sequence": 2}, {"evidence_id": "E-003", "sample_id": "S-001", "evidence_type": "blank_control", "metric_name": "post-clean-blank", "value": 0, "unit": "count", "collected_at": now.Add(4 * time.Minute), "collector_id": "investigator-a", "content_digest": "sha256:evidence-0000000000000003", "custody_sequence": 3}, {"evidence_id": "E-004", "sample_id": "S-001", "evidence_type": "container_inspection", "metric_name": "container-score", "value": 1, "unit": "score", "collected_at": now.Add(5 * time.Minute), "collector_id": "investigator-a", "content_digest": "sha256:evidence-0000000000000004", "custody_sequence": 4}, {"evidence_id": "E-005", "sample_id": "S-001", "evidence_type": "operation_log", "metric_name": "operation-count", "value": 1, "unit": "count", "collected_at": now.Add(6 * time.Minute), "collector_id": "investigator-a", "content_digest": "sha256:evidence-0000000000000005", "custody_sequence": 5}}
	for i, e := range evidence {
		if err := command(client, base, caseID, fmt.Sprintf("self-evidence-%04d", i), "add_evidence", "investigator-a", map[string]any{"evidence": e}); err != nil {
			return err
		}
	}
	hypothesis := map[string]any{"hypothesis_id": "H-001", "source_category": "handling_tool", "statement": "转运器具表面残留是本次污染的主要来源", "relations": []map[string]string{{"evidence_id": "E-001", "relation": "supports"}, {"evidence_id": "E-002", "relation": "supports"}}, "conclusion": "confirmed", "evaluated_at": now.Add(7 * time.Minute)}
	if err := command(client, base, caseID, "self-hypothesis-0001", "evaluate_hypothesis", "investigator-a", hypothesis); err != nil {
		return err
	}
	actionTypes := []string{"sample_isolation", "instrument_cleaning", "sample_retest"}
	for i, actionType := range actionTypes {
		actionID := fmt.Sprintf("A-%03d", i+1)
		action := map[string]any{"action": map[string]any{"action_id": actionID, "action_type": actionType, "target_sample_ids": []string{"S-001"}, "threshold": map[string]any{"metric_name": "post-clean-blank", "comparator": "<=", "value": 0.0, "unit": "count"}, "executor_id": "investigator-a", "witness_id": "witness-a"}}
		if err := command(client, base, caseID, fmt.Sprintf("self-action-%04d", i), "plan_remediation", "investigator-a", action); err != nil {
			return err
		}
		verify := map[string]any{"action_id": actionID, "retest_evidence_ids": []string{"E-003"}, "passed": true, "verified_at": now.Add(time.Duration(6+i) * time.Minute)}
		if err := command(client, base, caseID, fmt.Sprintf("self-verify-%04d", i), "verify_remediation", "witness-a", verify); err != nil {
			return err
		}
	}
	if err := command(client, base, caseID, "self-submit-0001", "submit_review", "lead-a", map[string]any{"submitted_at": now.Add(10 * time.Minute)}); err != nil {
		return err
	}
	review := map[string]any{"review_id": "R-001", "reviewer_id": "reviewer-independent", "outcome": "approved", "findings": []string{"证据链连续，处置验证满足阈值"}, "signed_at": now.Add(11 * time.Minute)}
	if err := command(client, base, caseID, "self-review-0001", "complete_review", "reviewer-independent", review); err != nil {
		return err
	}
	decision := map[string]any{"signer_id": "reviewer-independent", "decisions": []map[string]any{{"sample_id": "S-001", "disposition": "limited_research", "allowed_research_scope": []string{"稳定同位素筛查"}, "reason": "处置有效但保留用途边界"}}, "signed_at": now.Add(12 * time.Minute)}
	if err := command(client, base, caseID, "self-decision-0001", "record_dispositions", "reviewer-independent", decision); err != nil {
		return err
	}
	if err := command(client, base, caseID, "self-archive-0001", "archive", "custodian-a", map[string]any{"archived_at": now.Add(13 * time.Minute)}); err != nil {
		return err
	}
	if err := post(client, base+"/api/v1/cases/"+caseID+"/archive/verify", map[string]any{}, 200); err != nil {
		return err
	}
	resp, err := client.Get(base + "/api/v1/cases/" + caseID)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("案件查询状态码 %d", resp.StatusCode)
	}
	return nil
}
func command(client *http.Client, base, caseID, requestID, action, actor string, payload any) error {
	return post(client, base+"/api/v1/cases/"+caseID+"/commands", map[string]any{"request_id": requestID, "action": action, "actor_id": actor, "payload": payload}, 200)
}
func post(client *http.Client, url string, value any, want int) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != want {
		return fmt.Errorf("POST %s 返回 %d，期望 %d: %s", url, resp.StatusCode, want, string(body))
	}
	return nil
}
