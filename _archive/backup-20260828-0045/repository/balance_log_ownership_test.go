package repository

import (
	"os"
	"strings"
	"testing"
)

// PR-9：ListUserBalanceLogs 的安全契约是"user_id 硬绑定 + keyword 限非用户标识列"。
// 我们没有 mock DB 能力，所以用源码静态检查锁住这个不变式——
// 任何"在 ListUserBalanceLogs 函数体内写 user_id LIKE / 跨列 OR"的回归都会被测试抓出来。
// 这不是"测行为"，是"测契约"——和 PR-6 的 sentinel 哨兵属于同一种"形式化"测试策略。
func TestListUserBalanceLogsSQLContract(t *testing.T) {
	body := extractFuncBody(t, "user.go", "ListUserBalanceLogs")
	if body == "" {
		t.Fatal("could not locate ListUserBalanceLogs in user.go")
	}

	// 1) 必须硬绑定 user_id（绝对等于，不模糊）
	if !strings.Contains(body, `"user_id = ?"`) {
		t.Errorf("ListUserBalanceLogs must bind user_id with exact equality\n--- body ---\n%s", body)
	}
	// 2) 绝不允许 user_id LIKE——这是 PR-9 防止跨用户泄漏的核心约束
	if strings.Contains(body, "user_id LIKE") {
		t.Errorf("ListUserBalanceLogs contains forbidden \"user_id LIKE\" — would re-introduce the cross-user leak PR-9 fixed\n--- body ---\n%s", body)
	}
	// 3) keyword 模糊必须同时含 type / remark / related_id 三个非用户标识字段
	for _, must := range []string{"type LIKE", "remark LIKE", "related_id LIKE"} {
		if !strings.Contains(body, must) {
			t.Errorf("ListUserBalanceLogs missing required LIKE column %q\n--- body ---\n%s", must, body)
		}
	}
	// 4) 不允许回归到 4 个 LIKE 子句的旧跨列模式
	if strings.Count(body, " LIKE ?") >= 4 {
		t.Errorf("ListUserBalanceLogs appears to have regressed to the 4-column OR pattern\n--- body ---\n%s", body)
	}
}

// 配套：ListBalanceLogs（admin 用）仍允许跨列——锁住"未误改 admin 能力"。
func TestListBalanceLogsIsAdminScopedAndUntouched(t *testing.T) {
	body := extractFuncBody(t, "user.go", "ListBalanceLogs")
	if body == "" {
		t.Fatal("could not locate ListBalanceLogs in user.go")
	}
	if !strings.Contains(body, "user_id LIKE") {
		t.Errorf("ListBalanceLogs (admin) should still allow cross-column LIKE search\n--- body ---\n%s", body)
	}
	for _, must := range []string{"type LIKE", "remark LIKE", "related_id LIKE"} {
		if !strings.Contains(body, must) {
			t.Errorf("ListBalanceLogs (admin) lost LIKE column %q\n--- body ---\n%s", must, body)
		}
	}
}

// extractFuncBody 用朴素字符串扫描：找 "func ListXxx(" 到下一个顶层 "func " 之间的全部内容。
// 仓库内源文件都是单层缩进包级函数，不处理嵌套——够用且零依赖。
func extractFuncBody(t *testing.T, file, fnName string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	src := string(data)
	marker := "func " + fnName + "("
	start := strings.Index(src, marker)
	if start < 0 {
		return ""
	}
	// 从 start 之后找下一个顶层 "func "（行首）
	rest := src[start+len(marker):]
	endRel := -1
	for i := 0; i < len(rest); i++ {
		if rest[i] == '\n' && i+5 < len(rest) && rest[i+1:i+5] == "func " {
			endRel = i
			break
		}
	}
	if endRel < 0 {
		endRel = len(rest)
	}
	return src[start : start+len(marker)+endRel]
}
