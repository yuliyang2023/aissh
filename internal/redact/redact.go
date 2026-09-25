// Package redact provides a shared last-line defence for model requests and
// persisted history. It is not an access-control boundary.
package redact

import (
	"encoding/base64"
	"net/url"
	"sort"
	"strings"
	"sync"
)

type Filter struct {
	mu     sync.RWMutex
	values []string
}

func New(values ...string) *Filter { f := &Filter{}; f.Add(values...); return f }
func (f *Filter) Add(values ...string) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	set := map[string]bool{}
	for _, v := range f.values {
		set[v] = true
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		for _, variant := range []string{v, url.QueryEscape(v), url.PathEscape(v), base64.StdEncoding.EncodeToString([]byte(v))} {
			if !set[variant] {
				f.values = append(f.values, variant)
				set[variant] = true
			}
		}
	}
	sort.Slice(f.values, func(i, j int) bool { return len(f.values[i]) > len(f.values[j]) })
}
func (f *Filter) Text(value string) string {
	if f == nil {
		return value
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.values) == 0 {
		return value
	}
	pairs := make([]string, 0, len(f.values)*2)
	for _, v := range f.values {
		pairs = append(pairs, v, "[redacted]")
	}
	return strings.NewReplacer(pairs...).Replace(value)
}
