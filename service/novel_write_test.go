package service

import (
	"testing"

	"github.com/tigerowo/freedom/model"
)

// TestSubstituteVariables_ReplaceKnown 已知变量被替换。
func TestSubstituteVariables_ReplaceKnown(t *testing.T) {
	tpl := "你是分镜师，小说名 {{novel_name}}，主角 {{protagonist}}"
	vars := map[string]string{
		"novel_name": "长安夜雨",
		"protagonist": "李墨",
	}
	got := substituteVariables(tpl, vars)
	want := "你是分镜师，小说名 长安夜雨，主角 李墨"
	if got != want {
		t.Errorf("substituteVariables: got %q, want %q", got, want)
	}
}

// TestSubstituteVariables_KeepUnknown 未定义变量保留原样。
func TestSubstituteVariables_KeepUnknown(t *testing.T) {
	tpl := "小说名 {{novel_name}}，作者 {{author}}"
	vars := map[string]string{
		"novel_name": "长安夜雨",
	}
	got := substituteVariables(tpl, vars)
	want := "小说名 长安夜雨，作者 {{author}}"
	if got != want {
		t.Errorf("substituteVariables: got %q, want %q", got, want)
	}
}

// TestSubstituteVariables_IgnoreInvalidName 非 [a-zA-Z0-9_] 字符变量名不匹配。
func TestSubstituteVariables_IgnoreInvalidName(t *testing.T) {
	tpl := "{{var-name}} and {{var_name}} and {{2var}}"
	vars := map[string]string{
		"var-name":   "skip me",  // 连字符，不匹配
		"var_name":   "kept",
		"2var":       "skip me too",  // 数字开头，不匹配
	}
	got := substituteVariables(tpl, vars)
	// var-name 和 2var 不匹配（保留原样），var_name 替换
	want := "{{var-name}} and kept and {{2var}}"
	if got != want {
		t.Errorf("substituteVariables: got %q, want %q", got, want)
	}
}

// TestSubstituteVariables_EmptyAndWhitespace 处理空白。
func TestSubstituteVariables_EmptyAndWhitespace(t *testing.T) {
	tpl := "{{ x }} and {{y}}"
	vars := map[string]string{"x": "1", "y": "2"}
	got := substituteVariables(tpl, vars)
	want := "1 and 2"
	if got != want {
		t.Errorf("substituteVariables: got %q, want %q", got, want)
	}
}

// TestMarshalVariablesRoundTrip 序列化/反序列化 round-trip。
func TestMarshalVariablesRoundTrip(t *testing.T) {
	vars := model.Variables{
		"novel_name":  "长安夜雨",
		"protagonist": "李墨",
		"era":         "唐",
	}
	s := model.MarshalVariables(vars)
	got := model.UnmarshalVariables(s)
	if len(got) != 3 {
		t.Errorf("len: got %d, want 3", len(got))
	}
	if got["novel_name"] != "长安夜雨" {
		t.Errorf("novel_name: got %q, want %q", got["novel_name"], "长安夜雨")
	}
}

// TestMarshalVariablesEmpty 空 map 序列化为 "{}"。
func TestMarshalVariablesEmpty(t *testing.T) {
	got := model.MarshalVariables(nil)
	if got != "{}" {
		t.Errorf("nil: got %q, want %q", got, "{}")
	}
	got2 := model.MarshalVariables(model.Variables{})
	if got2 != "{}" {
		t.Errorf("empty: got %q, want %q", got2, "{}")
	}
}

// TestUnmarshalVariablesMalformed 非法 JSON 返回空 map。
func TestUnmarshalVariablesMalformed(t *testing.T) {
	got := model.UnmarshalVariables("{not json")
	if len(got) != 0 {
		t.Errorf("malformed: got %d entries, want 0", len(got))
	}
}

// TestBuildChatMessages_SystemFirst 验证 system 在最前。
func TestBuildChatMessages_SystemFirst(t *testing.T) {
	history := []model.NovelWriteMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "system", Content: "should be skipped"},  // 不应该出现
		{Role: "user", Content: "how are you"},
	}
	msgs := buildChatMessages("you are a storyboard artist", history, "now")
	if len(msgs) != 5 {
		t.Errorf("len: got %d, want 5", len(msgs))
	}
	if msgs[0]["role"] != "system" || msgs[0]["content"] != "you are a storyboard artist" {
		t.Errorf("system position: got %+v", msgs[0])
	}
	// 第二个 system（来自 history）应该被跳过
	for _, m := range msgs {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		if role == "system" && content == "should be skipped" {
			t.Errorf("history system should have been skipped")
		}
	}
	// 最后一条是新的 user
	if msgs[len(msgs)-1]["role"] != "user" || msgs[len(msgs)-1]["content"] != "now" {
		t.Errorf("last: got %+v", msgs[len(msgs)-1])
	}
}

// TestStripJSONFence 去 ```json``` 围栏。
func TestStripJSONFence(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{`{"a":1}`, `{"a":1}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"```\n{\"a\":1}\n```", `{"a":1}`},
		{"  \n```json\n{\"a\":1}\n```\n  ", `{"a":1}`},
	}
	for _, tt := range tests {
		got := stripJSONFence(tt.in)
		if got != tt.want {
			t.Errorf("stripJSONFence(%q): got %q, want %q", tt.in, got, tt.want)
		}
	}
}
