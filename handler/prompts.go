package handler

import (
	"encoding/json"
	"net/http"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

func Prompts(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListPrompts(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

// SubmitPromptRequest 用户提交提示词请求体。
type SubmitPromptRequest struct {
	Title    string   `json:"title"`
	CoverURL string   `json:"coverUrl"`
	VideoURL string   `json:"videoUrl"`
	Prompt   string   `json:"prompt"`
	Tags     []string `json:"tags"`
	Category string   `json:"category"`
	Preview  string   `json:"preview"`
}

// SubmitPrompt 用户提交提示词，状态为 pending。
func SubmitPrompt(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var req SubmitPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, "请求参数错误")
		return
	}
	if req.Title == "" || req.Prompt == "" {
		Fail(w, "标题和提示词不能为空")
		return
	}
	item := model.Prompt{
		Title:      req.Title,
		CoverURL:   req.CoverURL,
		VideoURL:   req.VideoURL,
		Prompt:     req.Prompt,
		Tags:       req.Tags,
		Category:   req.Category,
		Preview:    req.Preview,
		SubmittedByID: user.ID,
	}
	result, err := service.SubmitPrompt(item, user.ID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

// AdminPendingPrompts 管理员查看待审核列表。
func AdminPendingPrompts(w http.ResponseWriter, r *http.Request) {
	q := parseQuery(r)
	items, total, err := service.ListPendingPrompts(q.Page, q.PageSize)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"items": items, "total": int(total)})
}

// AdminRejectedPrompts 管理员查看被拒绝列表。
func AdminRejectedPrompts(w http.ResponseWriter, r *http.Request) {
	q := parseQuery(r)
	items, total, err := service.ListRejectedPrompts(q.Page, q.PageSize)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"items": items, "total": int(total)})
}

// AdminApprovePrompt 管理员通过审核。
func AdminApprovePrompt(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	if err := service.ApprovePrompt(id, user.ID); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

// AdminRejectPrompt 管理员拒绝审核。
func AdminRejectPrompt(w http.ResponseWriter, r *http.Request, id string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	if err := service.RejectPrompt(id, user.ID); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}
