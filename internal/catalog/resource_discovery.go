package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// Discovered operations describe the service, not permission to execute it.
type ResourceCapability struct {
	Contract   json.RawMessage `json:"contract,omitempty"`
	Name       string          `json:"name"`
	Risk       string          `json:"risk"` // read, write, dangerous, unknown
	Method     string          `json:"method,omitempty"`
	Path       string          `json:"path,omitempty"`
	Parameters string          `json:"parameters,omitempty"`
	Evidence   string          `json:"evidence"`
	Example    string          `json:"example,omitempty"`
}

type ResourceDiscovery struct {
	Status     string               `json:"status"` // ready, partial, failed
	Binding    string               `json:"binding"`
	ExploredAt string               `json:"exploredAt"`
	Summary    string               `json:"summary"`
	Notes      []string             `json:"notes"`
	Operations []ResourceCapability `json:"operations"`
}

// Include credential versions and the SSH route so changed connections cannot
// silently reuse an old system's operation guide.
func ResourceBinding(r Resource, d Data) string {
	r.Discovery, r.Name, r.Description = nil, "", ""
	r.Managed = false
	r.SystemID, r.SystemName, r.Environment = "", "", ""
	c, _ := d.Credential(r.CredentialID)
	hosts := []Host{}
	credentials := []Credential{c}
	if r.Route == "ssh" {
		id := r.HostID
		for i := 0; id != "" && i < 8; i++ {
			h, err := d.Host(id)
			if err != nil {
				break
			}
			hosts = append(hosts, h)
			credential, _ := d.Credential(h.CredentialID)
			credentials = append(credentials, credential)
			id = h.JumpHostID
		}
	}
	b, _ := json.Marshal([]any{r, hosts, credentials})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func CurrentResourceDiscovery(r Resource, d Data) *ResourceDiscovery {
	if r.Discovery == nil || r.Discovery.Binding != ResourceBinding(r, d) {
		return nil
	}
	return r.Discovery
}

func (s *Store) SaveResourceDiscovery(id string, report ResourceDiscovery) error {
	return s.mutateData(func(d *Data) error {
		for i, r := range d.Resources {
			if r.ID != id {
				continue
			}
			if report.Binding != ResourceBinding(r, *d) {
				return errors.New("连接信息已改变，请重新探索系统")
			}
			raw, err := json.Marshal(report)
			if err != nil || len(raw) > 256*1024 || len(report.Operations) > 100 {
				return errors.New("系统探索结果过大")
			}
			d.Resources[i].Discovery = &report
			return nil
		}
		return errors.New("系统连接已删除")
	}, false)
}
