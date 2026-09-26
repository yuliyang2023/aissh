package app

import (
	"context"
	"fmt"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/panel"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
)

type widgetCandidate struct {
	requiresSample     bool
	pipelineRefreshes  int
	pipelineRefreshing bool
	compileID          string
	id                 string
	panel              panel.Panel
	baseRevision       uint64
	baseBinding        string
	expires            time.Time
	cancel             context.CancelFunc
	artifact           *widgets.Artifact
}
type widgetCompiledEvent struct {
	panelID, id string
	artifact    widgets.Artifact
	err         error
}

func (a *App) prepareWidget(candidate panel.Panel, view *widgets.View) (*panel.Panel, string, error) {
	if candidate.ID == "" {
		candidate.ID = "widget-" + catalog.NewID()
	}
	old := a.panels.ByID(candidate.ID)
	base := uint64(0)
	binding := ""
	if old != nil {
		base = old.DefinitionRevision
		binding = old.BindingID
	}
	if candidate.BindingID == "" {
		candidate.BindingID = catalog.NewID()
	}
	a.cancelWidgetCandidate(candidate.ID)
	if a.widgetCandidates == nil {
		a.widgetCandidates = map[string]*widgetCandidate{}
	}
	if len(a.widgetCandidates) >= 4 {
		return nil, "", fmt.Errorf("最多同时创建 4 个组件，请等待预检完成")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &widgetCandidate{id: catalog.NewID(), panel: candidate, baseRevision: base, baseBinding: binding, expires: time.Now().Add(30 * time.Second), cancel: cancel}
	c.compileID = c.id
	a.widgetCandidates[candidate.ID] = c
	if view == nil {
		c.artifact = candidate.Widget
	} else {
		v := *view
		id, panelID := c.id, candidate.ID
		go func() {
			artifact, err := widgets.CompileIsolated(ctx, v)
			select {
			case a.events <- widgetCompiledEvent{panelID: panelID, id: id, artifact: artifact, err: err}:
			case <-ctx.Done():
			}
		}()
	}
	return nil, fmt.Sprintf("组件正在编译和预检：%s（%s）；通过后生效，当前版本继续运行", candidate.Title, candidate.ID), nil
}
func (a *App) widgetCompiled(e widgetCompiledEvent) {
	c := a.widgetCandidates[e.panelID]
	if c == nil || c.compileID != e.id {
		return
	}
	if e.err != nil {
		if c.panel.Pipeline != nil {
			a.pipelineReply("组件编译失败 panel=" + e.panelID + ": " + e.err.Error())
		}
		a.widgetNotice("组件未生效：" + e.err.Error())
		a.cancelWidgetCandidate(e.panelID)
		return
	}
	c.artifact = &e.artifact
	a.dirty = true
}
func (a *App) widgetNotice(text string) {
	text = a.safeText(text)
	a.appendPersistentChat(chatMessage{Role: "system", Content: text, Time: time.Now()})
	a.panelActionResults = append(a.panelActionResults, text)
	if len(a.panelActionResults) > 8 {
		a.panelActionResults = a.panelActionResults[len(a.panelActionResults)-8:]
	}
	a.dirty = true
}
func (a *App) cancelWidgetCandidate(id string) {
	if c := a.widgetCandidates[id]; c != nil {
		c.cancel()
		delete(a.widgetCandidates, id)
	}
}
func (a *App) expireWidgetCandidates(now time.Time) {
	for id, c := range a.widgetCandidates {
		if a.monitorReview != nil && a.monitorReview.PanelID == id {
			c.expires = now.Add(90 * time.Second)
		}
		if now.After(c.expires) {
			a.widgetNotice("组件预检超时，原版本保持运行：" + c.panel.Title)
			a.cancelWidgetCandidate(id)
		}
	}
}
func (a *App) finishWidget(panelID, candidateID, diagnostic string) error {
	c := a.widgetCandidates[panelID]
	if c == nil || c.id != candidateID || time.Now().After(c.expires) {
		return fmt.Errorf("组件候选已过期")
	}
	if diagnostic != "" {
		if c.panel.Pipeline != nil {
			a.pipelineReply("真实数据渲染预检失败 panel=" + panelID + ": " + diagnostic)
		}
		a.widgetNotice("组件未生效：" + diagnostic)
		a.cancelWidgetCandidate(panelID)
		return nil
	}
	if c.artifact == nil {
		return fmt.Errorf("组件尚未编译完成")
	}
	if c.requiresSample && c.panel.WidgetData.Sequence == 0 {
		return fmt.Errorf("缺少真实采样")
	}
	if c.panel.Pipeline != nil {
		if c.pipelineRefreshing {
			return fmt.Errorf("真实采样正在刷新")
		}
		if c.panel.WidgetData.Sequence == 0 {
			return fmt.Errorf("缺少真实数据预检样本")
		}
		if pipelineStale(c.panel, time.Now()) {
			a.refreshPipelineCandidate(c)
			return nil
		}
	}
	old := a.panels.ByID(panelID)
	if old == nil && c.baseRevision != 0 || old != nil && (old.DefinitionRevision != c.baseRevision || old.BindingID != c.baseBinding) {
		a.cancelWidgetCandidate(panelID)
		return fmt.Errorf("组件版本冲突，请重新修改")
	}
	if old == nil && a.panels.CountPurpose(false) >= 16 {
		a.cancelWidgetCandidate(panelID)
		return fmt.Errorf("实时面板已达到数量上限")
	}
	candidate := c.panel
	candidate.Widget = c.artifact
	candidate.WidgetData.BindingID = candidate.BindingID
	candidate.WidgetData.SourceRevision = candidate.SourceRevision
	if old != nil && old.SourceRevision == candidate.SourceRevision {
		candidate.Content = old.Content
		candidate.SampleContent = old.SampleContent
		candidate.LastUpdated = old.LastUpdated
		candidate.LastError = old.LastError
		candidate.RefreshCount = old.RefreshCount
		candidate.WidgetData = old.WidgetData
		candidate.RunID = old.RunID
		if candidate.Pipeline != nil {
			candidate.PipelineEngine = old.PipelineEngine
			candidate.PipelineCaptures = old.PipelineCaptures
		}
	}
	candidate.WidgetData.IntervalMS = candidate.Interval.Milliseconds()
	if old == nil {
		a.panels.Open(candidate)
	} else {
		*old = candidate
	}
	a.cancelWidgetCandidate(panelID)
	if candidate.Pipeline == nil {
		delete(a.monitorLast, panelID)
	} else {
		a.monitorLast[panelID] = candidate.LastUpdated
		delete(a.pipelineDrafts, panelID)
	}
	if candidate.Pipeline != nil {
		a.widgetNotice(fmt.Sprintf("监控面板已生效：%s（%s）；真实 SSH 首采、映射及渲染预检通过", candidate.Title, panelID))
		return nil
	}
	a.widgetNotice(fmt.Sprintf("自定义组件已生效：%s（%s），版本 %d；真实采集状态以面板为准", candidate.Title, panelID, candidate.DefinitionRevision))
	return nil
}
