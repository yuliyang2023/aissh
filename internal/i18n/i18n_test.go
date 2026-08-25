package i18n

import "testing"

func TestEnglishCatalogAndChineseFallback(t *testing.T) {
	english := New("en_US.UTF-8")
	if english.Language() != English || english.T("系统") != "System" {
		t.Fatalf("language=%q system=%q", english.Language(), english.T("系统"))
	}
	if got := english.T("批准失败：%s", "denied"); got != "Approval failed: denied" {
		t.Fatalf("formatted translation=%q", got)
	}
	if got := english.T("未收录的消息"); got != "未收录的消息" {
		t.Fatalf("missing translation did not fall back: %q", got)
	}
	chinese := New("zh_CN")
	if chinese.Language() != Chinese || chinese.T("系统") != "系统" {
		t.Fatalf("language=%q system=%q", chinese.Language(), chinese.T("系统"))
	}
}

func TestAutoLanguageDetection(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "en_GB.UTF-8")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if got := Resolve("auto"); got != English {
		t.Fatalf("auto language=%q", got)
	}
}

func TestPreferredResponseLanguageFollowsQuestion(t *testing.T) {
	if got := PreferredResponseLanguage("帮我检查 nginx status", English); got != Chinese {
		t.Fatalf("mixed Chinese question language=%q", got)
	}
	if got := PreferredResponseLanguage("check nginx status", Chinese); got != English {
		t.Fatalf("English question language=%q", got)
	}
	if got := PreferredResponseLanguage("12345", English); got != English {
		t.Fatalf("fallback language=%q", got)
	}
}
