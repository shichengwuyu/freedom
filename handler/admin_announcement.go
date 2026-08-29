package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

type adminSaveAnnouncementRequest struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

func AdminListAnnouncements(w http.ResponseWriter, r *http.Request) {
	q := parseQuery(r)
	keyword := r.URL.Query().Get("keyword")
	items, total, err := service.AdminListAnnouncements(q, keyword)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, model.AnnouncementList{Items: items, Total: total})
}

func AdminSaveAnnouncement(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	var req adminSaveAnnouncementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, "参数解析失败")
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		Fail(w, "公告内容不能为空")
		return
	}
	var err error
	if strings.TrimSpace(req.ID) == "" {
		_, err = service.AdminCreateAnnouncement(content)
	} else {
		_, err = service.AdminUpdateAnnouncement(req.ID, content)
	}
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, nil)
}

func AdminDeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Fail(w, "参数解析失败")
		return
	}
	if err := service.AdminDeleteAnnouncement(req.ID); err != nil {
		FailError(w, err)
		return
	}
	OK(w, nil)
}
