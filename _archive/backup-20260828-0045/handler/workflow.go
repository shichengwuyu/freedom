package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/tigerowo/freedom/service"
)

func UserWorkflows(w http.ResponseWriter, r *http.Request) {
	workflows, err := service.ListCreativeWorkflows(r.Context())
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, workflows)
}

func SaveUserWorkflow(w http.ResponseWriter, r *http.Request) {
	var request service.CreativeWorkflowPayload
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "工作流数据格式错误")
		return
	}
	workflow, err := service.SaveCreativeWorkflow(r.Context(), request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, workflow)
}

func DeleteUserWorkflow(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteCreativeWorkflow(r.Context(), id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func DraftUserWorkflow(w http.ResponseWriter, r *http.Request) {
	var request service.WorkflowAgentDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "工作流需求格式错误")
		return
	}
	result, err := service.DraftCreativeWorkflow(r.Context(), request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminAICallLogs(w http.ResponseWriter, r *http.Request) {
	list, err := service.ListAICallLogs(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, list)
}

// AdminAICallLogDates 返回所有存在日志文件的日期列表。
func AdminAICallLogDates(w http.ResponseWriter, r *http.Request) {
	dates, err := service.ListAICallLogDates()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, dates)
}

// ClientAICallLog 接收前端本地直连渠道的 AI 调用日志上报。
func ClientAICallLog(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	var request service.AICallLogInput
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	if !service.LocalDirectAILogEnabled() {
		OK(w, true)
		return
	}
	request.UserID = user.ID
	request.UserDisplayName = firstNonEmpty(user.DisplayName, user.Username)
	service.SaveAICallLog(request)
	OK(w, true)
}

func AdminDeleteAICallLogs(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("olderThanDays"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			Fail(w, "olderThanDays 参数必须是正整数")
			return
		}
		days = parsed
	}
	removed, err := service.DeleteAICallLogsOlderThan(days)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]int{"removedFiles": removed})
}

// AdminDeleteAICallLogsByDates 按具体日期数组删除日志，body 为 {"dates": ["2026-08-01", ...]}。
func AdminDeleteAICallLogsByDates(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Dates []string `json:"dates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		Fail(w, "参数格式错误")
		return
	}
	removed, err := service.DeleteAICallLogsByDates(payload.Dates)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]int{"removedFiles": removed})
}
