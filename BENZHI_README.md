# BENZHI_README

## 项目说明
- 项目：benzhi-project-b1b45e0b-44ae-466e-bda3-b43805156bee
- 项目用途：已完整实现极地冰芯污染事件调查与科研用途裁定服务，使用案件状态机、职责隔离、可恢复摘要事件链和确定性终局档案提供闭环 HTTP JSON API。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：IceCoreVerdict
- 项目介绍：面向极地冰芯实验室的污染事件调查与科研用途裁定服务，以单一案件状态机串联移交基线冻结、污染证据采集、来源假设检验、隔离处置、独立复核、用途裁定和可验证档案封存，防止证据不完整或职责冲突的样本重新进入科研分析。
- 项目概述：面向极地冰芯实验室的污染事件调查与科研用途裁定服务，以单一案件状态机串联移交基线冻结、污染证据采集、来源假设检验、隔离处置、独立复核、用途裁定和可验证档案封存，防止证据不完整或职责冲突的样本重新进入科研分析。
- 核心工作流：污染案件从草稿建档开始，冻结涉事样本与移交环境基线后进入调查；调查人员登记证据并检验污染来源假设，完成隔离处置及效果验证；与调查无职责冲突的复核员随后签署复核意见，裁定样本为可用、限用途或不可用，最后封存确定性案件档案并使案件进入不可再写的已归档状态。
- 对外接口：提供版本化 HTTP JSON API，使用方通过案件创建、命令提交、案件查询和档案校验端点推进唯一流程；服务监听地址支持 -addr=127.0.0.1:<port>，默认 127.0.0.1:19091，并在 PORT 为端口号时绑定 127.0.0.1:<PORT>，绝不默认绑定 0.0.0.0 或常见低位端口。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -self-check -addr=127.0.0.1:19091

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-b1b45e0b-44ae-466e-bda3-b43805156bee-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-b1b45e0b-44ae-466e-bda3-b43805156bee-arm64 linux/arm64

docker run -it benzhi-project-b1b45e0b-44ae-466e-bda3-b43805156bee-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -self-check -addr=127.0.0.1:19091`
