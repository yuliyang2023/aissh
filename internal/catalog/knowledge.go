package catalog

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gofrs/flock"
)

// Knowledge lives separately from connection settings and credential grants.
// Host and resource IDs are soft references: removing a system never removes
// the documents or experience accumulated about it.
type Knowledge struct {
	ID             string            `json:"id"`
	Title          string            `json:"title"`
	Kind           string            `json:"kind"` // document, experience, data
	Content        string            `json:"content"`
	Tags           []string          `json:"tags"`
	HostID         string            `json:"hostId"`
	ResourceID     string            `json:"resourceId"`
	FilePath       string            `json:"filePath"`
	Source         string            `json:"source"`
	ConversationID string            `json:"conversationId"`
	SourceURL      string            `json:"sourceUrl"`
	Columns        []string          `json:"columns"`
	Rows           [][]string        `json:"rows"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
	Changes        []KnowledgeChange `json:"changes,omitempty"`
}

type KnowledgeChange struct {
	Time           string            `json:"time"`
	HostID         string            `json:"hostId,omitempty"`
	ConversationID string            `json:"conversationId,omitempty"`
	Quote          string            `json:"quote"`
	Key            map[string]string `json:"key,omitempty"`
	Values         map[string]string `json:"values,omitempty"`
}

const maxKnowledgeBytes = 32 * 1024 * 1024

func KnowledgeSourceURL(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || strings.ContainsAny(value, "\x00\r\n") || len(value) > 4096 {
		return errors.New("请输入有效的 HTTP(S) 来源链接")
	}
	return nil
}

func (k *Knowledge) normalize() error {
	if len(k.Changes) > 100 {
		return errors.New("对话保存记录最多保留 100 条")
	}
	for _, change := range k.Changes {
		if len(change.Quote) > 4096 || !utf8.ValidString(change.Quote) || strings.ContainsRune(change.Quote, 0) || len(change.Values) > 50 || len(change.Key) > 10 {
			return errors.New("对话保存记录格式无效")
		}
	}
	k.Title = strings.TrimSpace(k.Title)
	k.SourceURL = strings.TrimSpace(k.SourceURL)
	if err := KnowledgeSourceURL(k.SourceURL); err != nil {
		return err
	}
	if k.Title == "" || !textField(k.Title) || !textField(k.Source) || !textField(k.FilePath) {
		return errors.New("请填写标题，单行信息不能超过 1024 字节或包含换行")
	}
	for _, id := range []string{k.ID, k.HostID, k.ResourceID} {
		if id != "" && ValidateID(id) != nil {
			return errors.New("知识条目标识无效")
		}
	}
	if !textField(k.ConversationID) {
		return errors.New("会话来源无效")
	}
	if len(k.Content) > 256*1024 || !utf8.ValidString(k.Content) || strings.ContainsRune(k.Content, 0) {
		return errors.New("正文必须是 UTF-8 文本，且不能超过 256 KB")
	}
	switch k.Kind {
	case "document":
		if k.FilePath == "" && k.SourceURL == "" {
			return errors.New("请选择本机文档或填写在线文档链接")
		}
		if k.FilePath != "" && !filepath.IsAbs(k.FilePath) {
			return errors.New("本机文档路径必须是绝对路径")
		}
	case "experience":
		if strings.TrimSpace(k.Content) == "" {
			return errors.New("请填写经验内容")
		}
	case "data":
		if len(k.Columns) == 0 || len(k.Columns) > 50 || len(k.Rows) > 5000 {
			return errors.New("数据表需要 1–50 列，最多 5000 条记录")
		}
		names := map[string]bool{}
		for i, name := range k.Columns {
			name = strings.TrimSpace(name)
			if name == "" || !textField(name) || names[name] {
				return errors.New("列名不能为空、重复或包含换行")
			}
			names[name] = true
			k.Columns[i] = name
		}
		for _, row := range k.Rows {
			if len(row) != len(k.Columns) {
				return errors.New("每条记录的单元格数量必须与列数一致")
			}
			for _, cell := range row {
				if len(cell) > 8192 || !utf8.ValidString(cell) || strings.ContainsRune(cell, 0) {
					return errors.New("单元格必须是 UTF-8 文本且不超过 8 KB")
				}
			}
		}
	default:
		return errors.New("不支持的本地知识类型")
	}
	if k.Kind != "document" {
		k.FilePath = ""
	}
	if k.Kind != "data" {
		k.Columns, k.Rows = nil, nil
	}
	if k.Columns == nil {
		k.Columns = []string{}
	}
	if k.Rows == nil {
		k.Rows = [][]string{}
	}
	if len(k.Tags) > 20 {
		return errors.New("最多支持 20 个标签")
	}
	tags := make([]string, 0, len(k.Tags))
	for _, tag := range k.Tags {
		tag = strings.TrimSpace(tag)
		if !textField(tag) {
			return errors.New("标签过长或包含换行")
		}
		if tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	k.Tags = tags
	return nil
}

func (s *Store) ListKnowledge() ([]Knowledge, error) {
	items := []Knowledge{}
	if s.Dir == "" {
		return nil, errors.New("无法定位本机配置目录")
	}
	f, err := os.Open(filepath.Join(s.Dir, "knowledge.json"))
	if errors.Is(err, os.ErrNotExist) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxKnowledgeBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxKnowledgeBytes {
		return nil, errors.New("本地知识库超过 32 MB")
	}
	var data struct {
		Version int         `json:"version"`
		Items   []Knowledge `json:"items"`
	}
	if json.Unmarshal(raw, &data) != nil || data.Version != 1 {
		return nil, errors.New("本地知识库损坏或版本不受支持，请检查 knowledge.json")
	}
	seen := map[string]bool{}
	for _, item := range data.Items {
		if ValidateID(item.ID) != nil || seen[item.ID] || item.normalize() != nil {
			return nil, errors.New("本地知识库包含无效条目，请检查 knowledge.json")
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	slices.SortStableFunc(items, func(a, b Knowledge) int { return strings.Compare(b.UpdatedAt, a.UpdatedAt) })
	return items, nil
}

func (s *Store) mutateKnowledge(fn func([]Knowledge) ([]Knowledge, error)) error {
	if s.Dir == "" {
		return errors.New("无法定位本机配置目录")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(s.Dir, "knowledge.lock"))
	if err := lock.Lock(); err != nil {
		return err
	}
	defer lock.Unlock()
	items, err := s.ListKnowledge()
	if err != nil {
		return err
	}
	items, err = fn(items)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(struct {
		Version int         `json:"version"`
		Items   []Knowledge `json:"items"`
	}{1, items}, "", "  ")
	if err != nil {
		return err
	}
	if len(raw) > maxKnowledgeBytes {
		return errors.New("本地知识库超过 32 MB，请先清理不再使用的条目")
	}
	f, err := os.CreateTemp(s.Dir, ".knowledge-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.Dir, "knowledge.json"))
}

func (s *Store) SaveKnowledge(k Knowledge) (Knowledge, error) {
	if err := k.normalize(); err != nil {
		return Knowledge{}, err
	}
	err := s.mutateKnowledge(func(items []Knowledge) ([]Knowledge, error) {
		if k.ID == "" {
			k.Changes = nil
			k.ID, k.CreatedAt, k.UpdatedAt = NewID(), now(), now()
			return append(items, k), nil
		}
		i := slices.IndexFunc(items, func(item Knowledge) bool { return item.ID == k.ID })
		if i < 0 {
			return nil, errors.New("知识条目已被删除，请刷新后重试")
		}
		if items[i].UpdatedAt != k.UpdatedAt {
			return nil, errors.New("知识条目已在其他窗口更新，请刷新后重试")
		}
		k.CreatedAt, k.UpdatedAt = items[i].CreatedAt, now()
		k.Changes = items[i].Changes
		items[i] = k
		return items, nil
	})
	return k, err
}

func (s *Store) DeleteKnowledge(id, updatedAt string) error {
	return s.mutateKnowledge(func(items []Knowledge) ([]Knowledge, error) {
		i := slices.IndexFunc(items, func(k Knowledge) bool { return k.ID == id })
		if i < 0 {
			return nil, errors.New("知识条目已被删除")
		}
		if items[i].UpdatedAt != updatedAt {
			return nil, errors.New("知识条目已更新，请刷新后再删除")
		}
		return slices.Delete(items, i, i+1), nil
	})
}
