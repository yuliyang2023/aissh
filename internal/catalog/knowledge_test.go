package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestKnowledgePersistsSeparatelyAndRejectsStaleEdits(t *testing.T) {
	s := New(t.TempDir(), nil)
	host, err := s.SaveHost(Host{Name: "服务器", Address: "server", Port: 22})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path())
	k, err := s.SaveKnowledge(Knowledge{Title: "  资产台账  ", Kind: "data", HostID: host.ID,
		SourceURL: "https://docs.example.test/sheets/123?tab=assets", Columns: []string{"资产", "部门", "备注"},
		Rows: [][]string{{"应用服务器", "研发", "已验证\n可用"}}, Tags: []string{" CMDB ", "CMDB"}})
	if err != nil {
		t.Fatal(err)
	}
	if k.ID == "" || k.CreatedAt == "" || k.Title != "资产台账" || len(k.Tags) != 1 {
		t.Fatalf("bad saved entry: %+v", k)
	}
	after, _ := os.ReadFile(s.path())
	if !bytes.Equal(before, after) {
		t.Fatal("knowledge changed connection authorization revision")
	}
	restarted := New(s.Dir, nil)
	items, err := restarted.ListKnowledge()
	if err != nil || len(items) != 1 || items[0].Rows[0][2] != "已验证\n可用" {
		t.Fatalf("restart: %+v %v", items, err)
	}
	stale := k
	k.Title = "最新资产"
	updated, err := restarted.SaveKnowledge(k)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveKnowledge(stale); err == nil {
		t.Fatal("stale edit overwrote table")
	}
	if err := s.DeleteKnowledge(stale.ID, stale.UpdatedAt); err == nil {
		t.Fatal("stale delete removed table")
	}
	if err := s.Delete("host", host.ID); err != nil {
		t.Fatal(err)
	}
	items, err = s.ListKnowledge()
	if err != nil || len(items) != 1 || items[0].HostID != host.ID {
		t.Fatal("removing host removed accumulated knowledge")
	}
	if err := s.DeleteKnowledge(updated.ID, updated.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveKnowledge(updated); err == nil {
		t.Fatal("editing deleted knowledge recreated it")
	}
}

func TestKnowledgeValidationPreservesExistingFile(t *testing.T) {
	s := New(t.TempDir(), nil)
	valid := Knowledge{Title: "资产", Kind: "data", Columns: []string{"名称", "说明"}, Rows: [][]string{{"一", "二"}}}
	if _, err := s.SaveKnowledge(valid); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "knowledge.json"))
	tests := []Knowledge{
		{Title: "重复列", Kind: "data", Columns: []string{"资产", " 资产 "}},
		{Title: "不规则行", Kind: "data", Columns: []string{"资产"}, Rows: [][]string{{"a", "b"}}},
		{Title: "空经验", Kind: "experience"},
		{Title: "危险链接", Kind: "document", SourceURL: "javascript:alert(1)"},
		{Title: "账号链接", Kind: "document", SourceURL: "https://user:password@example.com/a"},
		{Title: "无文件", Kind: "document"},
		{Title: "长单元格", Kind: "data", Columns: []string{"名称"}, Rows: [][]string{{strings.Repeat("x", 8193)}}},
	}
	for _, bad := range tests {
		if _, err := s.SaveKnowledge(bad); err == nil {
			t.Errorf("accepted %s", bad.Title)
		}
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "knowledge.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("invalid save altered the knowledge file")
	}
}

func TestKnowledgeConcurrentWritersAndCorruptFile(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := New(dir, nil).SaveKnowledge(Knowledge{Title: fmt.Sprint(i), Kind: "experience", Content: "记录"})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	s := New(dir, nil)
	items, err := s.ListKnowledge()
	if err != nil || len(items) != 12 {
		t.Fatalf("lost concurrent updates: %d %v", len(items), err)
	}
	path := filepath.Join(dir, "knowledge.json")
	broken := []byte(`{"version":1,"items":[`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveKnowledge(Knowledge{Title: "new", Kind: "experience", Content: "记录"}); err == nil {
		t.Fatal("silently replaced corrupt knowledge")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, broken) {
		t.Fatal("lost recoverable corrupt file")
	}
}
