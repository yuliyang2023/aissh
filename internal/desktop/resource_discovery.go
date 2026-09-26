package desktop

import (
	"context"
	"encoding/json"
	"time"

	"git.example.com/ai/wrd-ai-cli/internal/catalog"
	"git.example.com/ai/wrd-ai-cli/internal/externals"
	"git.example.com/ai/wrd-ai-cli/internal/resources"
)

func (s *Service) ExploreResource(id string) (catalog.ResourceDiscovery, error) {
	return s.exploreResource(s.ctx, id)
}

func (s *Service) exploreResource(ctx context.Context, id string) (catalog.ResourceDiscovery, error) {
	d, err := s.store.Load()
	if err != nil {
		return catalog.ResourceDiscovery{}, err
	}
	r, err := d.Resource(id)
	if err != nil {
		return catalog.ResourceDiscovery{}, err
	}
	helper := ""
	if r.Route == "ssh" {
		helper, err = cliPath()
		if err != nil {
			report := catalog.ResourceDiscovery{Status: "failed", Binding: catalog.ResourceBinding(r, d), ExploredAt: time.Now().UTC().Format(time.RFC3339Nano), Summary: "连接已保存，SSH 探索暂未完成。", Notes: []string{err.Error()}, Operations: []catalog.ResourceCapability{}}
			if r.Managed {
				return report, externals.NewStore(s.store).Import(id, report, nil)
			}
			return report, s.store.SaveResourceDiscovery(id, report)
		}
	}
	return resources.Explore(ctx, s.store, id, helper)
}

func (s *Service) exploreConversationSystem(ctx context.Context, id string) (string, error) {
	if _, err := externals.NewStore(s.store).Ensure(id); err != nil {
		return "", err
	}
	d, err := s.store.Load()
	if err != nil {
		return "", err
	}
	r, err := d.Resource(id)
	if err != nil {
		return "", err
	}
	report := catalog.CurrentResourceDiscovery(r, d)
	if saved, _ := externals.NewStore(s.store).Discovery(id); saved != nil && saved.Binding == catalog.ResourceBinding(r, d) {
		report = saved
	}
	if report == nil {
		// A metadata edit cannot silently use a credential requiring approval.
		if r.CredentialID != "" && r.Grant != "read" {
			return "配置已保存；请在外联系统的操作目录中点击只读探索，以批准本次凭据使用。", nil
		}
		result, err := s.exploreResource(ctx, id)
		if err != nil {
			return "", err
		}
		report = &result
	}
	raw, err := json.Marshal(report)
	return string(raw), err
}
