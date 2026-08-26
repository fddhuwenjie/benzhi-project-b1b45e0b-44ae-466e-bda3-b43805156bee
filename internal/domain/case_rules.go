package domain

import (
	"math"
	"sort"
	"strings"
	"time"
)

func DecideCreate(cmd CreateCase) ([]Event, error) {
	if !nonblank(cmd.CaseID) {
		return nil, invalid("case_id", "不能为空")
	}
	if !nonblank(cmd.Title) {
		return nil, invalid("title", "不能为空")
	}
	if !nonblank(cmd.TransferBatch) {
		return nil, invalid("transfer_batch", "不能为空")
	}
	if len(strings.TrimSpace(cmd.IncidentSummary)) < 10 {
		return nil, invalid("incident_summary", "至少需要 10 个字符")
	}
	if !nonblank(cmd.LeadActorID) {
		return nil, invalid("lead_actor_id", "不能为空")
	}
	if !nonblank(cmd.CreatedBy) {
		return nil, invalid("created_by", "不能为空")
	}
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = time.Now().UTC()
	}
	var err error
	cmd.AllowedResearchScopes, err = normalizeScopes(cmd.AllowedResearchScopes)
	if err != nil {
		return nil, err
	}
	e, err := NewEvent(cmd.CaseID, EventCaseCreated, cmd.CreatedBy, cmd.CreatedAt, CaseCreatedData{Command: cmd})
	return []Event{e}, err
}

func (a *Aggregate) DecideFreeze(cmd FreezeBaseline, actor string) ([]Event, error) {
	if err := a.ensureWritable(); err != nil {
		return nil, err
	}
	if err := a.expect(StatusDraft); err != nil {
		return nil, err
	}
	if len(cmd.Samples) == 0 {
		return nil, invalid("samples", "至少登记一支样本")
	}
	if math.IsNaN(cmd.MinTemperatureCelsius) || math.IsNaN(cmd.MaxTemperatureCelsius) || math.IsInf(cmd.MinTemperatureCelsius, 0) || math.IsInf(cmd.MaxTemperatureCelsius, 0) || cmd.MinTemperatureCelsius < -80 || cmd.MaxTemperatureCelsius > 10 || cmd.MinTemperatureCelsius >= cmd.MaxTemperatureCelsius {
		return nil, invalid("temperature_range", "温控下限必须小于上限且均为有限数值")
	}
	seen, seals := map[string]bool{}, map[string]bool{}
	receipt := BaselineReceipt{FrozenAt: cmd.FrozenAt, FrozenRevision: a.Case.Revision + 1, MinTemperatureCelsius: cmd.MinTemperatureCelsius, MaxTemperatureCelsius: cmd.MaxTemperatureCelsius}
	for i := range cmd.Samples {
		s := &cmd.Samples[i]
		if !nonblank(s.SampleID) || seen[s.SampleID] {
			return nil, invalid("samples.sample_id", "样本编号不能为空且不得重复")
		}
		if !nonblank(s.CoreSegment) || !nonblank(s.ContainerSeal) || !nonblank(s.CustodyHolder) {
			return nil, invalid("samples", "岩芯段、容器封签和保管人均不能为空")
		}
		if seals[s.ContainerSeal] {
			return nil, invalid("samples.container_seal", "同一案件内容器封签不得重复")
		}
		seals[s.ContainerSeal] = true
		if s.TransferBatch == "" {
			s.TransferBatch = a.Case.TransferBatch
		}
		if s.TransferBatch != a.Case.TransferBatch {
			return nil, invalid("samples.transfer_batch", "样本移交批次与案件不一致")
		}
		if s.TransferTemperatureCelsius < -80 || s.TransferTemperatureCelsius > 10 {
			return nil, invalid("transfer_temperature_celsius", "必须在 -80 到 10 摄氏度之间")
		}
		s.CaseID = a.Case.CaseID
		seen[s.SampleID] = true
		check := BaselineSampleCheck{SampleID: s.SampleID, Temperature: s.TransferTemperatureCelsius, Explanation: strings.TrimSpace(s.TemperatureException)}
		if s.TransferTemperatureCelsius < cmd.MinTemperatureCelsius || s.TransferTemperatureCelsius > cmd.MaxTemperatureCelsius {
			check.Level = "out_of_range"
			receipt.OutOfRangeCount++
			if check.Explanation == "" {
				return nil, invalid("samples.temperature_exception", "越界样本必须提供异常说明")
			}
		} else {
			margin := (cmd.MaxTemperatureCelsius - cmd.MinTemperatureCelsius) * 0.1
			if s.TransferTemperatureCelsius-cmd.MinTemperatureCelsius <= margin || cmd.MaxTemperatureCelsius-s.TransferTemperatureCelsius <= margin {
				check.Level = "critical"
				receipt.CriticalCount++
			} else {
				check.Level = "normal"
				receipt.NormalCount++
			}
		}
		receipt.SampleChecks = append(receipt.SampleChecks, check)
	}
	if cmd.FrozenAt.IsZero() {
		cmd.FrozenAt = time.Now().UTC()
	}
	receipt.FrozenAt = cmd.FrozenAt.UTC()
	sort.Slice(receipt.SampleChecks, func(i, j int) bool { return receipt.SampleChecks[i].SampleID < receipt.SampleChecks[j].SampleID })
	e, err := NewEvent(a.Case.CaseID, EventBaselineFrozen, actor, cmd.FrozenAt, BaselineFrozenData{Samples: cmd.Samples, FrozenAt: cmd.FrozenAt, Receipt: receipt})
	return []Event{e}, err
}
