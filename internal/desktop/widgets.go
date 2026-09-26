package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"git.example.com/ai/wrd-ai-cli/internal/credentialui"
	"git.example.com/ai/wrd-ai-cli/internal/widgets"
)

// Called with session.mu held, before publishing the metadata snapshot.
func acceptWidgets(session *terminalSession, input credentialui.WorkbenchExchange) error {
	t := input.Widgets
	if len(t.Artifacts) > 20 || len(t.Data) > 16 || len(t.Definitions) > 16 {
		return errors.New("组件传输超过限制")
	}
	active := map[string]credentialui.MonitorPanel{}
	hashes := map[string]bool{}
	for _, p := range input.State.Panels {
		if p.Widget != nil {
			active[p.ID] = p
			hashes[p.Widget.Hash] = true
		}
	}
	if len(input.State.WidgetCandidates) > 4 {
		return errors.New("组件候选过多")
	}
	for _, c := range input.State.WidgetCandidates {
		hashes[c.Hash] = true
	}
	// Validate everything before changing cache state.
	for _, a := range t.Artifacts {
		if !hashes[a.Hash] {
			return errors.New("未引用的组件制品")
		}
		if err := a.Validate(); err != nil {
			return err
		}
	}
	for id, d := range t.Data {
		p, ok := active[id]
		b, _ := json.Marshal(d)
		if !ok || d.BindingID != p.Widget.BindingID || len(b) > 512<<10 {
			return errors.New("组件数据无效")
		}
	}
	for id, d := range t.Definitions {
		p, ok := active[id]
		if !ok || d.Revision != p.Widget.Revision || d.View.Validate() != nil || d.Contract.Validate() != nil {
			return errors.New("组件定义无效")
		}
	}
	if session.widgetArtifacts == nil {
		session.widgetArtifacts = map[string]widgets.Artifact{}
		session.widgetData = map[string]widgets.Data{}
		session.widgetDefinitions = map[string]widgets.Definition{}
		session.widgetSaveErrors = map[string]string{}
	}
	for _, a := range t.Artifacts {
		session.widgetArtifacts[a.Hash] = a
	}
	for hash := range session.widgetArtifacts {
		if !hashes[hash] {
			delete(session.widgetArtifacts, hash)
		}
	}
	for id, d := range t.Data {
		session.widgetData[id] = d
	}
	for id, d := range t.Definitions {
		session.widgetDefinitions[id] = d
	}
	for id := range session.widgetData {
		if _, ok := active[id]; !ok {
			delete(session.widgetData, id)
			delete(session.widgetDefinitions, id)
			delete(session.widgetSaveErrors, id)
		}
	}
	return nil
}
func (s *Service) GetWidgetArtifact(id, hash string) (widgets.Artifact, error) {
	session, err := s.session(id)
	if err != nil {
		return widgets.Artifact{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	a, ok := session.widgetArtifacts[hash]
	if !ok || session.closed {
		return a, errors.New("组件制品不可用")
	}
	return a, nil
}
func (s *Service) GetWidgetData(id, panelID, bindingID string) (widgets.Data, error) {
	session, err := s.session(id)
	if err != nil {
		return widgets.Data{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	d, ok := session.widgetData[panelID]
	if !ok || d.BindingID != bindingID {
		return d, errors.New("组件数据已变化")
	}
	return d, nil
}
func (s *Service) GetSavedWidgets(id string) ([]widgets.Saved, error) {
	session, err := s.session(id)
	if err != nil {
		return nil, err
	}
	s.widgetMu.Lock()
	defer s.widgetMu.Unlock()
	items, err := widgets.ListSaved(filepath.Join(s.store.Dir, "monitor-widgets"), session.hostID)
	for i := range items {
		for j := range items[i].Versions {
			d := &items[i].Versions[j].Definition
			d.View.Source = ""
			d.Command = ""
			d.Contract = widgets.Contract{}
		}
	}
	return items, err
}
func (s *Service) saveWidgetDefinitions(session *terminalSession, definitions map[string]widgets.Definition) {
	s.widgetMu.Lock()
	defer s.widgetMu.Unlock()
	for id, d := range definitions {
		err := widgets.Save(filepath.Join(s.store.Dir, "monitor-widgets"), session.hostID, d)
		session.mu.Lock()
		if err != nil {
			session.widgetSaveErrors[id] = err.Error()
		} else {
			delete(session.widgetSaveErrors, id)
		}
		session.mu.Unlock()
	}
}
func (s *Service) RetryWidgetSave(id, panelID string) error {
	session, err := s.session(id)
	if err != nil {
		return err
	}
	session.mu.Lock()
	d, ok := session.widgetDefinitions[panelID]
	session.mu.Unlock()
	if !ok {
		return errors.New("组件不存在")
	}
	s.saveWidgetDefinitions(session, map[string]widgets.Definition{panelID: d})
	session.mu.Lock()
	defer session.mu.Unlock()
	if message := session.widgetSaveErrors[panelID]; message != "" {
		return errors.New(message)
	}
	return nil
}
func (s *Service) RestoreSavedWidget(id, key string, version int, panelID string) error {
	session, err := s.session(id)
	if err != nil {
		return err
	}
	s.widgetMu.Lock()
	saved, err := widgets.LoadSaved(filepath.Join(s.store.Dir, "monitor-widgets"), key)
	s.widgetMu.Unlock()
	if err != nil {
		return err
	}
	if saved.HostID != session.hostID {
		return errors.New("组件属于其他主机")
	}
	var d *widgets.Definition
	for _, v := range saved.Versions {
		if v.Version == version {
			copy := v.Definition
			d = &copy
			break
		}
	}
	if d == nil {
		return errors.New("组件版本不存在")
	}
	d.ID = ""
	d.Revision = 0
	if panelID != "" {
		session.mu.Lock()
		if session.workbench != nil {
			for _, p := range session.workbench.Panels {
				if p.ID == panelID && p.Origin == "ai" && p.Widget != nil {
					d.ID = p.ID
					d.Revision = p.Widget.Revision
				}
			}
		}
		session.mu.Unlock()
		if d.ID == "" {
			return fmt.Errorf("目标组件已变化")
		}
	}
	return s.WorkbenchAction(id, credentialui.WorkbenchCommand{Kind: "widget-restore", WidgetDefinition: d})
}
