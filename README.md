# IceCoreVerdict

IceCoreVerdict 是面向极地冰芯实验室的污染事件调查与科研用途裁定服务。它以单一案件状态机串联保管基线冻结、结构化证据登记、污染来源假设检验、隔离处置与复测、独立复核、逐样本用途裁定以及确定性档案封存，避免证据不完整或存在职责冲突的样本重新进入科研分析。

## 状态流程

案件依次经过 `draft`、`bounded`、`investigating`、`remediation_validation`、`pending_review`、`decided` 和 `archived`。复核可驳回一次，整改轨迹会保留并回到 `investigating`；第二次提交须通过。归档后所有业务命令均被拒绝。

数据保存在本地追加式 JSONL 事件流中，每个事件包含前序摘要和自身摘要。原子检查点与 `request_id` 索引支持进程重启后的恢复和幂等响应。终局 JSON 档案按稳定顺序生成，并分别校验案件与样本、证据、假设、处置、复核裁定和事件索引六个分区，以及整档摘要与当前事件根摘要。

## 调查门禁

`freeze_baseline` 必须提供 `min_temperature_celsius`、`max_temperature_celsius` 和样本清单；越界样本在自身的 `temperature_exception` 中说明异常。成功响应及案件详情都包含稳定的样本分级冻结回执。

`add_evidence` 沿用五类证据：`blank_control`、`ion_metric`、`particle_metric`、`container_inspection`、`operation_log`。案件详情返回逐样本数量、最新采集时间、缺项、质量问题和总体完成率。证据 `content_digest` 使用 `sha256:<摘要>` 格式，摘要、时间与保管链均执行连续性校验。

`plan_remediation` 的 `action.threshold` 采用 `{ "metric_name": "...", "comparator": "<=", "value": 1.0, "unit": "..." }`。`verify_remediation` 会从关联证据自动复算结果；逐样本隔离、器具清洁、复测覆盖及最新复测均合格后才能提交复核。

驳回复核可提交结构化 `findings`，每项包含可选 `finding_id`、`severity`、`assignee_id`、`due_at` 及必填 `summary`。使用同一命令端点的 `close_corrective_item` 关闭整改项，并关联驳回后产生的 `related_evidence_ids` 或 `related_action_ids`。`record_dispositions` 载荷设置 `precheck: true` 时只返回一致性预检且不追加事件；正式签署会再次执行相同规则。

## 构建与测试

```text
go build ./cmd/server
go test ./...
```

## 运行

```text
go run ./cmd/server -addr=127.0.0.1:19091 -data-dir=./data
```

默认监听 `127.0.0.1:19091`，不会绑定公网地址。也可通过 `PORT` 指定端口，此时绑定 `127.0.0.1:<PORT>`；显式 `-addr` 优先。服务提供版本化 HTTP JSON API：

- `POST /api/v1/cases` 创建案件；
- `POST /api/v1/cases/{case_id}/commands` 提交唯一流程中的业务命令；
- `GET /api/v1/cases/{case_id}` 查询案件投影；
- `GET /api/v1/cases/{case_id}/events` 筛选和分页读取事件；
- `GET /api/v1/cases/{case_id}/archive` 下载只读档案；
- `POST /api/v1/cases/{case_id}/archive/verify` 校验档案。

请求和响应均为 `application/json`。写请求必须提供长度为 8 至 128 个字符的 `request_id`，案件命令还需提供 `action`、`actor_id` 与对应 `payload`。

事件查询支持组合使用 `actor_id`、`event_type`、`from_revision`、`to_revision`、`occurred_from`、`occurred_to`、`limit` 和 `cursor`。响应中的每条事件带前后状态与摘要校验标记，并始终返回完整状态迁移轨迹；`limit` 最大为 100。

## 自检

以下命令会创建临时数据目录，启动真实回环 HTTP 服务，完整执行创建、调查、处置、复核、裁定、归档和校验流程，然后主动关闭并退出：

```text
go run ./cmd/server -self-check -addr=127.0.0.1:19091
```
