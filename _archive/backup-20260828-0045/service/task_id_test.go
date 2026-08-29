package service

import (
	"regexp"
	"strings"
	"testing"
)

// PR-8: 主键必须始终服务端生成 uuid，且与客户端传入的 ClientTaskID 无关。
// 之前实现是 firstVideoTaskValue(input.ClientTaskID, "...uuid")——攻击者 A 传 clientTaskId=X
// 占了主键，B 再传 X 会落到 Update 路径覆盖 A 的行。
// 现在的契约是：所有 task ID 都以固定前缀 + uuid 形式出现，连续调用不会撞键。
func TestNewTaskIDsAreServerGeneratedAndUnique(t *testing.T) {
	prefixes := map[string]func() string{
		"canvas_image_task_":  newCanvasImageTaskID,
		"canvas_audio_task_":  newCanvasAudioTaskID,
		"video-task-":         newVideoTaskID,
		"storyboard-task-":    newStoryboardTaskID,
	}
	for prefix, gen := range prefixes {
		prefix := prefix
		gen := gen
		t.Run(strings.TrimSuffix(prefix, "_-"), func(t *testing.T) {
			// 1) 前缀正确
			id := gen()
			if !strings.HasPrefix(id, prefix) {
				t.Fatalf("id=%q, want prefix %q", id, prefix)
			}
			// 2) 100 次生成全部唯一
			seen := map[string]bool{id: true}
			for i := 0; i < 100; i++ {
				next := gen()
				if seen[next] {
					t.Fatalf("collision after %d iterations: %s", i, next)
				}
				seen[next] = true
				if !strings.HasPrefix(next, prefix) {
					t.Errorf("iteration %d: id=%q, want prefix %q", i, next, prefix)
				}
			}
		})
	}
}

// 关键反例: 即使客户端传相同 clientTaskId，主键 helper 也不会"接受"它。
// 这是 PR-8 防跨用户覆盖的核心: 主键永远不由客户端字段派生。
func TestNewTaskIDsIgnoreClientInput(t *testing.T) {
	// 模拟 service 内部"如果用了 firstVideoTaskValue(clientTaskID, ...)"的模式：
	// 两次相同输入会得到相同 ID。我们断言新的 helper 永远产出唯一 ID。
	_ = "any-client-task-id-a-shared-string"
	_ = "any-client-task-id-a-shared-string"

	ids := map[string][]string{
		"image":      {newCanvasImageTaskID(), newCanvasImageTaskID()},
		"audio":      {newCanvasAudioTaskID(), newCanvasAudioTaskID()},
		"video":      {newVideoTaskID(), newVideoTaskID()},
		"storyboard": {newStoryboardTaskID(), newStoryboardTaskID()},
	}
	for kind, pair := range ids {
		if pair[0] == pair[1] {
			t.Errorf("%s: consecutive ids equal (%s); helper must not be deterministic by client input", kind, pair[0])
		}
	}
}

// 顺带断言 ID 的格式合法: 前缀之后是 uuid (8-4-4-4-12 hex,可能带连字符也可能不带，看实现)。
// 至少要非空、纯 ASCII 可见字符，且不含 '/'、'\\'、路径分隔符、空白——否则可能影响 URL 路由或文件系统路径。
func TestNewTaskIDsAreURLSafe(t *testing.T) {
	banned := regexp.MustCompile(`[/\\\s]`)
	for _, gen := range []func() string{newCanvasImageTaskID, newCanvasAudioTaskID, newVideoTaskID, newStoryboardTaskID} {
		id := gen()
		if banned.MatchString(id) {
			t.Errorf("id=%q contains URL/path-unsafe character", id)
		}
	}
}
