package model

// PromptStatus 提示词审核状态。
type PromptStatus string

const (
	PromptStatusPending PromptStatus = "pending"
	PromptStatusApproved PromptStatus = "approved"
	PromptStatusRejected PromptStatus = "rejected"
)

// Prompt 提示词记录。
type Prompt struct {
	ID            string       `json:"id" gorm:"primaryKey"`
	Title         string       `json:"title"`
	CoverURL      string       `json:"coverUrl"`
	VideoURL      string       `json:"videoUrl"`
	ExternalURL   string       `json:"externalUrl"`
	Prompt        string       `json:"prompt" gorm:"type:longtext"`
	Tags          []string     `json:"tags" gorm:"serializer:json"`
	Category      string       `json:"category" gorm:"index"`
	GithubURL     string       `json:"githubUrl" gorm:"-"`
	Preview       string       `json:"preview" gorm:"type:longtext"`
	Status        PromptStatus `json:"status" gorm:"default:pending"`
	SubmittedByID string       `json:"submittedById" gorm:"index"`
	ReviewerID    string       `json:"reviewerId"`
	CreatedAt     string       `json:"createdAt"`
	UpdatedAt     string       `json:"updatedAt"`
}

// PromptList 提示词分页结果。
type PromptList struct {
	Items      []Prompt `json:"items"`
	Tags       []string `json:"tags"`
	Categories []string `json:"categories"`
	Total      int      `json:"total"`
}

// PromptCategory 提示词分类。
type PromptCategory struct {
	Category    string `json:"category" gorm:"primaryKey"`
	Name        string `json:"name"`
	Description string `json:"description"`
	GithubURL   string `json:"githubUrl"`
	Remote      bool   `json:"remote"`
	UpdatedAt   string `json:"updatedAt"`
}
