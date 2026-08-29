package service

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

const latestAnnouncementLimit = 10

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ListLatestAnnouncements 获取最新 N 条公告（最多 10 条，按创建时间倒序）。
func ListLatestAnnouncements() ([]model.Announcement, error) {
	return repository.ListLatestAnnouncements(latestAnnouncementLimit)
}

// AdminListAnnouncements 管理员列表。
func AdminListAnnouncements(q model.Query, keyword string) ([]model.Announcement, int64, error) {
	return repository.ListAnnouncements(q, keyword)
}

// AdminCreateAnnouncement 管理员新增公告。
func AdminCreateAnnouncement(content string) (model.Announcement, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return model.Announcement{}, safeMessageError{message: "公告内容不能为空"}
	}
	now := nowUTC()
	item := model.Announcement{
		ID:        uuid.NewString(),
		Content:   content,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repository.CreateAnnouncement(item); err != nil {
		return model.Announcement{}, err
	}
	return item, nil
}

// AdminUpdateAnnouncement 管理员更新公告。
func AdminUpdateAnnouncement(id, content string) (model.Announcement, error) {
	id = strings.TrimSpace(id)
	content = strings.TrimSpace(content)
	if id == "" {
		return model.Announcement{}, safeMessageError{message: "公告 ID 不能为空"}
	}
	if content == "" {
		return model.Announcement{}, safeMessageError{message: "公告内容不能为空"}
	}
	existing, found, err := repository.GetAnnouncementByID(id)
	if err != nil {
		return model.Announcement{}, err
	}
	if !found {
		return model.Announcement{}, safeMessageError{message: "公告不存在"}
	}
	now := nowUTC()
	if err := repository.UpdateAnnouncement(id, content, now); err != nil {
		return model.Announcement{}, err
	}
	existing.Content = content
	existing.UpdatedAt = now
	return existing, nil
}

// AdminDeleteAnnouncement 管理员删除公告。
func AdminDeleteAnnouncement(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return safeMessageError{message: "公告 ID 不能为空"}
	}
	return repository.DeleteAnnouncement(id)
}
