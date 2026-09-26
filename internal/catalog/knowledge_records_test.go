package catalog

import (
	"slices"
	"testing"
)

func TestConversationalRecordsReuseAndUpdateOnlyTheMatchingRow(t *testing.T) {
	s := New(t.TempDir(), nil)
	in := KnowledgeRecordInput{Title: "资产归属", Key: map[string]string{"IP": "192.0.2.10"}, Values: map[string]string{"部门": "研发"}, Change: KnowledgeChange{Quote: "192.0.2.10 属于研发", ConversationID: "create-one"}}
	first, err := s.UpsertKnowledgeRecord(in)
	if err != nil || first.Status != "created" || first.Row != 1 {
		t.Fatalf("first: %+v %v", first, err)
	}
	duplicate, err := s.UpsertKnowledgeRecord(in)
	if err != nil || duplicate.ID != first.ID || duplicate.Status != "unchanged" || duplicate.UpdatedAt != first.UpdatedAt {
		t.Fatalf("duplicate: %+v %v", duplicate, err)
	}
	in.Key["IP"] = "192.0.2.20"
	if _, err := s.UpsertKnowledgeRecord(in); err != nil {
		t.Fatal(err)
	}
	in.Key["IP"] = "192.0.2.10"
	in.Values = map[string]string{"部门": "财务", "负责人": "张三"}
	if _, err := s.UpsertKnowledgeRecord(in); err == nil {
		t.Fatal("overwrote a conflict without reading")
	}
	items, _ := s.ListKnowledge()
	in.ExpectedUpdatedAt = items[0].UpdatedAt
	updated, err := s.UpsertKnowledgeRecord(in)
	if err != nil || updated.Status != "updated" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err := s.UpsertKnowledgeRecord(in); err == nil {
		t.Fatal("stale update accepted")
	}
	items, _ = s.ListKnowledge()
	k := items[0]
	department, owner := slices.Index(k.Columns, "部门"), slices.Index(k.Columns, "负责人")
	if len(items) != 1 || len(k.Rows) != 2 || k.Rows[0][department] != "财务" || k.Rows[1][department] != "研发" || k.Rows[1][owner] != "" || len(k.Changes) != 3 {
		t.Fatalf("unrelated data changed: %+v", k)
	}
	k.Changes = nil
	k, err = s.SaveKnowledge(k)
	if err != nil || len(k.Changes) != 3 {
		t.Fatalf("manual edit lost provenance: %+v %v", k, err)
	}
}

func TestConversationalRecordsRejectAmbiguousRowsAndRetainNotes(t *testing.T) {
	s := New(t.TempDir(), nil)
	k, err := s.SaveKnowledge(Knowledge{Title: "资产", Kind: "data", Columns: []string{"名称", "环境"}, Rows: [][]string{{"应用", "生产"}, {"应用", "测试"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UpsertKnowledgeRecord(KnowledgeRecordInput{TableID: k.ID, Key: map[string]string{"名称": "应用"}, Values: map[string]string{"负责人": "张三"}})
	if err == nil {
		t.Fatal("ambiguous update accepted")
	}
	n, err := s.SaveKnowledgeNote("排障", "先检查 DNS", "", "", KnowledgeChange{ConversationID: "new-chat", Quote: "记住：先检查 DNS"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveKnowledgeNote("排障", "改成其他内容", n.ID, "", KnowledgeChange{}); err == nil {
		t.Fatal("overwrote note without revision")
	}
	items, _ := s.ListKnowledge()
	for _, item := range items {
		if item.ID == n.ID && (item.ConversationID != "" || item.Content != "先检查 DNS") {
			t.Fatalf("creation note linked to SSH: %+v", item)
		}
	}
}

func TestConversationSystemCannotRedirectExistingCredentials(t *testing.T) {
	s := New(t.TempDir(), nil)
	r, err := s.SaveConversationSystem(Resource{Name: "CMDB", Kind: "http", Endpoint: "https://cmdb.example.com"}, 0)
	if err != nil || r.Grant != "ask" || r.AllowLocal || r.HostID != "" || r.CredentialID != "" {
		t.Fatalf("defaults: %+v %v", r, err)
	}
	d, _ := s.Load()
	r.Description = "研发维护"
	if _, err = s.SaveConversationSystem(r, d.Revision); err != nil {
		t.Fatal(err)
	}
	d, _ = s.Load()
	r.Endpoint = "https://other.example.com"
	if _, err = s.SaveConversationSystem(r, d.Revision); err == nil {
		t.Fatal("redirect accepted")
	}
	d, _ = s.Load()
	if len(d.Resources) != 1 || d.Resources[0].Description != "研发维护" || d.Resources[0].Endpoint != "https://cmdb.example.com" {
		t.Fatalf("changed unexpected fields: %+v", d.Resources)
	}
}
