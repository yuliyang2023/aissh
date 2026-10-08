package app

import (
	"encoding/json"
	"fmt"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
	"reflect"
	"strings"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/agent"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/policy"
)

func (a *App) applyPanelAction(action agent.PanelAction) {
	item, message, err := a.executePanelAction(action)
	role := "system"
	if err != nil {
		role = "error"
		if action.Panel.Pipeline != nil || action.Action == "probe" || action.Action == "map" || action.Panel.View != nil && action.Panel.View.Kind == "custom" {
			a.pipelineReply(a.t("操作失败 panel=%s stage=%s: %s", action.Panel.ID, action.Action, err.Error()))
		}
		message = a.t("监控面板操作失败（%s / %s）：%s", action.Action, action.Panel.ID, err.Error())
	}
	message = a.safeText(message)
	a.appendPersistentChat(chatMessage{Role: role, Content: message, Time: time.Now()})
	result := message
	if item != nil {
		result += "\ncommand=" + agent.LimitOutputContext(a.safeText(item.Command), 500)
	} else if action.Panel.Command != "" {
		result += "\nrequested_command=" + agent.LimitOutputContext(a.safeText(action.Panel.Command), 500)
	}
	a.panelActionResults = append(a.panelActionResults, result)
	if len(a.panelActionResults) > 8 {
		a.panelActionResults = append([]string(nil), a.panelActionResults[len(a.panelActionResults)-8:]...)
	}
	a.dirty = true
}

func (a *App) executePanelAction(action agent.PanelAction) (*panel.Panel, string, error) {
	spec := action.Panel
	operation := strings.ToLower(strings.TrimSpace(action.Action))
	item := a.panels.ByID(spec.ID)
	purpose := strings.TrimSpace(spec.Purpose)
	if purpose == "" {
		purpose = "realtime"
		if item != nil && item.IsAlert() {
			purpose = "alert"
		}
	}
	if purpose != "realtime" && purpose != "alert" {
		return nil, "", fmt.Errorf("%s", a.t("purpose 必须为 realtime（实时面板）或 alert（告警任务）"))
	}
	if item != nil && item.IsAlert() != (purpose == "alert") {
		return nil, "", fmt.Errorf("%s", a.t("实时面板与告警任务不能互相覆盖，请使用新的 ID 创建独立任务"))
	}
	if item != nil && (item.Origin != panel.OriginAI || item.Pinned) {
		return nil, "", fmt.Errorf("%s", a.t("AI 只能修改或关闭自己创建的非固定面板"))
	}
	if operation == "probe" || operation == "map" || (operation == "create" || operation == "update" || operation == "open") && (spec.Pipeline != nil || item != nil && item.Pipeline != nil) {
		return a.executePipelineAction(operation, spec)
	}
	switch operation {
	case "open", "create", "update":
		if spec.Type != "" && spec.Type != "monitor" && spec.Type != "log" && spec.Type != "table" {
			return nil, "", fmt.Errorf("%s", a.t("不支持的监控面板类型：%s", spec.Type))
		}
		if operation == "update" && strings.TrimSpace(spec.ID) == "" {
			return nil, "", fmt.Errorf("%s", a.t("更新监控面板必须提供 ID"))
		}
		// An explicit command expresses the requested monitoring scope. A named
		// collector is a fallback, never a replacement for that command.
		command := strings.TrimSpace(spec.Command)
		if command == "" && strings.TrimSpace(spec.Collector) != "" {
			command = collectorCommand(spec.Collector)
			if command == "" {
				return nil, "", fmt.Errorf("%s", a.t("未知的监控采集器：%s", spec.Collector))
			}
		}
		configUpdate := command != ""
		if command == "" && item != nil {
			command = item.Command
		}
		if err := policy.ValidateReadOnlyMonitorCommand(command); err != nil {
			return nil, "", fmt.Errorf("%s", a.t("监控命令被只读策略拒绝：%s", err.Error()))
		}
		if item == nil && a.panels.CountPurpose(purpose == "alert") >= 16 {
			return nil, "", fmt.Errorf("%s", a.t("实时面板与告警任务各最多 16 个，请先关闭同类中不用的项目"))
		}
		// Validate the complete candidate before changing the live panel.
		candidate := panel.Panel{ID: spec.ID, Kind: panel.KindMonitor, Origin: panel.OriginAI,
			Title: a.t("AI 面板"), Interval: 2 * time.Second, Target: a.parsed.Target}
		created := item == nil
		if item != nil {
			candidate = *item
		}
		candidate.Purpose = purpose
		if spec.Type != "" {
			candidate.Kind = panelKind(spec.Type)
		}
		if strings.TrimSpace(spec.Title) != "" {
			candidate.Title = spec.Title
		}
		if spec.IntervalMS > 0 {
			candidate.Interval = time.Duration(spec.IntervalMS) * time.Millisecond
			if candidate.Interval < time.Second {
				candidate.Interval = time.Second
			}
		}
		if spec.Alert != nil {
			if purpose != "alert" {
				return nil, "", fmt.Errorf("%s", a.t("实时面板不配置提醒，请使用 purpose=alert 和新 ID 创建独立告警任务"))
			}
			if err := spec.Alert.Validate(); err != nil {
				return nil, "", fmt.Errorf("%s: %w", a.t("提醒策略无效"), err)
			}
			copy := *spec.Alert
			candidate.Alert = &copy
		}
		if purpose == "alert" && (candidate.Alert == nil || (created && !candidate.Alert.Enabled)) {
			return nil, "", fmt.Errorf("%s", a.t("创建告警任务必须配置有效且启用的 alert 条件"))
		}
		if created || configUpdate {
			candidate.AllowAIRead = spec.AllowAIRead
		}

		if item != nil && (item.Widget != nil || spec.View != nil && spec.View.Kind == "custom") && spec.ExpectedRevision != item.DefinitionRevision {
			return nil, "", fmt.Errorf("%s", a.t("组件版本已变化，请读取最新 definitionRevision=%d 后重试", item.DefinitionRevision))
		}
		if spec.Data != nil {
			if err := spec.Data.Validate(); err != nil {
				return nil, "", err
			}
			copy := *spec.Data
			copy.Fields = append([]widgets.Field(nil), copy.Fields...)
			candidate.DataContract = &copy
		}
		if spec.View != nil {
			if a.safeText(spec.View.Source) != spec.View.Source {
				return nil, "", fmt.Errorf("%s", a.t("组件源码中不能包含凭据，请从采集数据读取指标"))
			}
			if err := spec.View.Validate(); err != nil {
				return nil, "", err
			}
			if spec.View.Kind == "text" {
				candidate.Widget = nil
				candidate.DataContract = nil
			} else if !a.config.DesktopUI || purpose == "alert" {
				return nil, "", fmt.Errorf("%s", a.t("自定义组件仅用于桌面实时面板"))
			}
		}
		needsWidget := spec.View != nil && spec.View.Kind == "custom" || candidate.Widget != nil
		if needsWidget && spec.View != nil && spec.View.SDKVersion == 2 {
			return nil, "", fmt.Errorf("%s", a.t("需要首采：先提交 pipeline probe、map，再使用 mapId 创建 v2 面板"))
		}
		if needsWidget && item == nil && a.config.DesktopUI {
			return nil, "", fmt.Errorf("%s", a.t("新组件需要 v2 首采管线：请先 probe、map，不再接受直接生成 v1 组件"))
		}
		if needsWidget && candidate.DataContract == nil {
			return nil, "", fmt.Errorf("%s", a.t("自定义组件需要 data 契约"))
		}
		contractChanged := item != nil && !reflect.DeepEqual(item.DataContract, candidate.DataContract)
		commandChanged := candidate.Command != command
		if needsWidget && item != nil && item.Widget != nil && (commandChanged || contractChanged) {
			return nil, "", fmt.Errorf("%s", a.t("修改旧组件的数据源需要 v2 首采验证：请先 probe、map，再提交 sdkVersion=2 组件"))
		}
		candidate.Command = command
		if commandChanged || contractChanged {
			candidate.WidgetData = widgets.Data{}
			candidate.SourceRevision++
			candidate.SampleContent = ""
			candidate.Content, candidate.Status, candidate.LastError = "", "", ""
			candidate.LastUpdated = time.Time{}
			candidate.RefreshCount = 0
		}
		if spec.Content != "" {
			candidate.Content = spec.Content
		}
		candidate.Unread = true
		candidate.DefinitionRevision++
		if candidate.DefinitionRevision == 0 {
			candidate.DefinitionRevision = 1
		}
		if needsWidget {
			return a.prepareWidget(candidate, spec.View)
		}
		if item == nil {
			item = a.panels.Open(candidate)
		} else {
			*item = candidate
		}
		if a.monitorLast != nil {
			delete(a.monitorLast, item.ID)
		}
		status := a.t("监控面板已更新：%s（%s），每 %g 秒刷新", item.Title, item.ID, item.Interval.Seconds())
		if created {
			status = a.t("监控面板已创建：%s（%s），每 %g 秒刷新", item.Title, item.ID, item.Interval.Seconds())
		}
		if item.IsAlert() {
			status = a.t("告警任务已更新：%s（%s），每 %g 秒检查", item.Title, item.ID, item.Interval.Seconds())
			if created {
				status = a.t("告警任务已创建：%s（%s），每 %g 秒检查", item.Title, item.ID, item.Interval.Seconds())
			}
		}
		if item.Alert != nil && item.Alert.Enabled {
			if a.config.DesktopUI {
				status += a.t("；已配置异常提醒（%s，持续 %d 秒触发，恢复持续 %d 秒）", item.Alert.Kind, item.Alert.ForSeconds, item.Alert.RecoverySeconds)
			} else {
				status += a.t("；提醒条件已附加，自动告警需在桌面客户端运行")
			}
		}
		return item, status, nil
	case "append":
		if item != nil && item.Widget != nil {
			return nil, "", fmt.Errorf("%s", a.t("自定义组件请通过 view 更新，不能 append"))
		}
		if item == nil {
			return nil, "", fmt.Errorf("%s", a.t("监控面板不存在：%s；请使用 create 或提供完整的 update 配置", spec.ID))
		}
		item.Content += "\n\n" + spec.Content
		item.Status, item.Unread = a.t("AI 更新"), true
		return item, a.t("监控面板内容已追加：%s（%s）", item.Title, item.ID), nil
	case "close":
		a.resetMonitorAudit(spec.ID, false)
		a.cancelPipeline(spec.ID)
		a.cancelWidgetCandidate(spec.ID)
		delete(a.monitorActive, spec.ID)
		if err := a.panels.Close(spec.ID, panel.OriginAI); err != nil {
			return nil, "", err
		}
		delete(a.monitorLast, spec.ID)
		return nil, a.t("监控面板已关闭：%s", spec.ID), nil
	default:
		return nil, "", fmt.Errorf("%s", a.t("未知的监控面板操作：%s", action.Action))
	}
}

// Snapshot on the app event loop before starting an asynchronous AI request.
// Only configuration/results are included, never unapproved monitor output.
func (a *App) addPanelContext(request *agent.Request) {
	if request.SystemContext == nil {
		request.SystemContext = make(map[string]string)
	}
	if a.config.DesktopUI {
		request.SystemContext["monitorWidgetV2"] = agent.WidgetV2Guidance
	}
	request.SystemContext["monitor_panel_rules"] = "面板操作是否成功，以 monitor_panel_result 和 monitor_panel_state 为准；创建成功不代表采集成功。内置默认采集脚本免审批并静默采集；自定义监控命令（含 probe、告警、历史组件）首次及变更时需用户审计批准，不能绕过审计或宣称已执行。批准后的首次采集在 SSH 终端显示，后续按周期静默更新监控面板。自定义 command 优先于 collector；使用自定义命令时 collector 留空。update 可修改采集配置，面板不存在时需提供 command 或有效 collector 才能创建。"
	for index, result := range a.panelActionResults {
		request.SystemContext[fmt.Sprintf("monitor_panel_result_%d", index)] = a.safeText(result)
	}
	if a.panels == nil {
		return
	}
	for index, item := range a.panels.Panels() {
		state := map[string]any{"id": item.ID, "purpose": item.Purpose, "title": item.Title, "origin": item.Origin,
			"intervalMs": item.Interval.Milliseconds(), "refreshCount": item.RefreshCount}
		state["alert"] = item.Alert
		state["definitionRevision"] = item.DefinitionRevision
		if item.Widget != nil {
			state["view"] = item.Widget.View
			state["data"] = item.DataContract
			if item.Pipeline != nil {
				state["pipeline"] = item.Pipeline
			}
		}
		if item.AllowAIRead {
			state["command"] = agent.LimitOutputContext(a.safeText(item.Command), 500)
			state["lastError"] = agent.LimitOutputContext(a.safeText(item.LastError), 200)
		}
		data, _ := json.Marshal(state)
		request.SystemContext[fmt.Sprintf("monitor_panel_state_%d", index)] = a.safeText(string(data))
	}
}
