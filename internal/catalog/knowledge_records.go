package catalog

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type KnowledgeRecordInput struct {
	TableID           string
	Title             string
	Key               map[string]string
	Values            map[string]string
	ExpectedUpdatedAt string
	Change            KnowledgeChange
}

type KnowledgeWriteResult struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Row       int    `json:"row,omitempty"`
	UpdatedAt string `json:"updatedAt"`
}

func appendKnowledgeChange(k *Knowledge, change KnowledgeChange) {
	change.Time = now()
	k.Changes = append(k.Changes, change)
	if len(k.Changes) > 100 {
		k.Changes = k.Changes[len(k.Changes)-100:]
	}
}

// UpsertKnowledgeRecord changes only the identified row while holding the
// knowledge lock. A model never submits a replacement copy of an entire table.
func (s *Store) UpsertKnowledgeRecord(in KnowledgeRecordInput) (KnowledgeWriteResult, error) {
	var result KnowledgeWriteResult
	if len(in.Key) == 0 || len(in.Key) > 10 || len(in.Values) > 50 {
		return result, errors.New("记录需要 1–10 个定位字段，最多 50 个数据字段")
	}
	values := maps.Clone(in.Values)
	if values == nil {
		values = map[string]string{}
	}
	for key, value := range in.Key {
		if v, ok := values[key]; ok && v != value {
			return result, errors.New("定位字段与数据字段的值不一致")
		}
		values[key] = value
	}
	for key, value := range values {
		if strings.TrimSpace(key) != key || key == "" || !textField(key) || strings.TrimSpace(value) == "" {
			return result, errors.New("字段名称和值不能为空，字段名称不能包含换行或首尾空格")
		}
	}
	err := s.mutateKnowledge(func(items []Knowledge) ([]Knowledge, error) {
		index := -1
		for i, item := range items {
			if (in.TableID != "" && item.ID == in.TableID) || (in.TableID == "" && item.Title == strings.TrimSpace(in.Title) && item.Kind == "data") {
				if index >= 0 {
					return nil, errors.New("存在多个同名数据表，请使用查询返回的 tableId")
				}
				index = i
			}
		}
		if index < 0 && in.TableID != "" {
			return nil, errors.New("数据表不存在，请重新查询")
		}
		k := Knowledge{ID: NewID(), Title: strings.TrimSpace(in.Title), Kind: "data", CreatedAt: now(), Tags: []string{"AI 整理"}, Columns: []string{}, Rows: [][]string{}}
		if index >= 0 {
			k = items[index]
		}
		if k.Kind != "data" {
			return nil, errors.New("目标不是数据表")
		}
		if in.ExpectedUpdatedAt != "" && in.ExpectedUpdatedAt != k.UpdatedAt {
			return nil, errors.New("数据表已更新，请重新读取后再保存")
		}
		// Existing columns keep their positions; new fields extend every row.
		columns := slices.Clone(k.Columns)
		for _, key := range slices.Sorted(maps.Keys(values)) {
			if !slices.Contains(columns, key) {
				columns = append(columns, key)
			}
		}
		found := -1
		for i, row := range k.Rows {
			match := true
			for key, value := range in.Key {
				col := slices.Index(k.Columns, key)
				if col < 0 || row[col] != value {
					match = false
					break
				}
			}
			if match {
				if found >= 0 {
					return nil, errors.New("定位字段匹配多条记录，请补充定位字段")
				}
				found = i
			}
		}
		status := "created"
		if found >= 0 {
			status = "unchanged"
			for key, value := range values {
				col := slices.Index(k.Columns, key)
				if col >= 0 && k.Rows[found][col] != "" && k.Rows[found][col] != value && in.ExpectedUpdatedAt == "" {
					return nil, fmt.Errorf("已有记录的「%s」为「%s」，请先读取数据表并携带 updatedAt 更新", key, k.Rows[found][col])
				}
				if col < 0 || k.Rows[found][col] != value {
					status = "updated"
				}
			}
		}
		if status == "unchanged" {
			result = KnowledgeWriteResult{k.ID, k.Title, status, found + 1, k.UpdatedAt}
			return items, nil
		}
		for i := range k.Rows {
			k.Rows[i] = append(k.Rows[i], make([]string, len(columns)-len(k.Columns))...)
		}
		k.Columns = columns
		if found < 0 {
			k.Rows = append(k.Rows, make([]string, len(columns)))
			found = len(k.Rows) - 1
		}
		for key, value := range values {
			k.Rows[found][slices.Index(columns, key)] = value
		}
		k.UpdatedAt = now()
		in.Change.Key, in.Change.Values = maps.Clone(in.Key), maps.Clone(values)
		appendKnowledgeChange(&k, in.Change)
		if err := k.normalize(); err != nil {
			return nil, err
		}
		if index < 0 {
			items = append(items, k)
		} else {
			items[index] = k
		}
		result = KnowledgeWriteResult{k.ID, k.Title, status, found + 1, k.UpdatedAt}
		return items, nil
	})
	return result, err
}

func (s *Store) SaveKnowledgeNote(title, content, id, expected string, change KnowledgeChange) (KnowledgeWriteResult, error) {
	title = strings.TrimSpace(title)
	var result KnowledgeWriteResult
	err := s.mutateKnowledge(func(items []Knowledge) ([]Knowledge, error) {
		index := -1
		for i, item := range items {
			if (id != "" && item.ID == id) || (id == "" && item.Kind == "experience" && item.Title == title) {
				if index >= 0 {
					return nil, errors.New("存在多个同名经验，请使用条目 ID")
				}
				index = i
			}
		}
		if index < 0 && id != "" {
			return nil, errors.New("经验条目不存在")
		}
		k := Knowledge{ID: NewID(), Title: title, Kind: "experience", CreatedAt: now(), Content: content, Tags: []string{"AI 整理"}}
		status := "created"
		if index >= 0 {
			k = items[index]
			if k.Kind != "experience" {
				return nil, errors.New("只能使用此操作保存经验")
			}
			if k.Content == content && k.Title == title {
				result = KnowledgeWriteResult{k.ID, k.Title, "unchanged", 0, k.UpdatedAt}
				return items, nil
			}
			if expected == "" || k.UpdatedAt != expected {
				return nil, errors.New("修改已有经验前请读取条目，并携带其 updatedAt")
			}
			k.Title, k.Content, status = title, content, "updated"
		}
		k.UpdatedAt = now()
		appendKnowledgeChange(&k, change)
		if err := k.normalize(); err != nil {
			return nil, err
		}
		if index < 0 {
			items = append(items, k)
		} else {
			items[index] = k
		}
		result = KnowledgeWriteResult{k.ID, k.Title, status, 0, k.UpdatedAt}
		return items, nil
	})
	return result, err
}

// Conversation system edits cannot change credential bindings or grants, nor
// redirect credentials to a newly suggested endpoint.
func (s *Store) SaveConversationSystem(r Resource, revision uint64) (Resource, error) {
	err := s.mutate(func(d *Data) error {
		index := -1
		for i, item := range d.Resources {
			if (r.ID != "" && item.ID == r.ID) || (r.ID == "" && item.Name == r.Name) {
				if index >= 0 {
					return errors.New("有多个同名系统，请指定系统 ID")
				}
				index = i
			}
		}
		if index >= 0 {
			old := d.Resources[index]
			if r.Endpoint != "" && r.Endpoint != old.Endpoint {
				return errors.New("连接地址变更请在外联系统页面修改；对话可更新系统说明")
			}
			if revision != d.Revision {
				return errors.New("系统信息已更新，请重新查询后修改")
			}
			old.Name, old.Description = r.Name, r.Description
			d.Resources[index], r = old, old
			return nil
		}
		if r.ID != "" {
			return errors.New("系统不存在，请重新查询")
		}
		r.ID, r.Route, r.Grant, r.AllowLocal, r.CredentialID = NewID(), "local", "ask", false, ""
		d.Resources = append(d.Resources, r)
		return nil
	})
	return r, err
}
