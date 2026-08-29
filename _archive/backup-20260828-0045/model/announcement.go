package model

// Announcement 系统公告。
type Announcement struct {
	ID        string `json:"id" gorm:"primarykey"`
	Content   string `json:"content"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type AnnouncementList struct {
	Items []Announcement `json:"items"`
	Total int64          `json:"total"`
}
