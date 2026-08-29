package model

type LicenseKeyStatus string

const (
	LicenseKeyStatusUnused LicenseKeyStatus = "unused"
	LicenseKeyStatusUsed   LicenseKeyStatus = "used"
)

// LicenseKey 卡密（admin 手动补发通道）。
// FaceValueCents 卡密面额，单位 = 分（cents）。
type LicenseKey struct {
	ID             string           `json:"id" gorm:"primarykey"`
	Key            string           `json:"key" gorm:"uniqueIndex;size:32"`
	FaceValueCents int              `json:"faceValueCents"`
	Status         LicenseKeyStatus `json:"status" gorm:"size:16;index"`
	UsedBy         string           `json:"usedBy"`
	UsedAt         string           `json:"usedAt"`
	BatchName      string           `json:"batchName" gorm:"index"`
	CreatedBy      string           `json:"createdBy"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
}

type LicenseKeyList struct {
	Items []LicenseKey `json:"items"`
	Total int64        `json:"total"`
}

// LicenseRedeemLog 卡密兑换流水。
// FaceValueCents 兑换时入账面额，单位 = 分。
type LicenseRedeemLog struct {
	ID             string `json:"id" gorm:"primarykey"`
	LicenseKeyID   string `json:"licenseKeyId" gorm:"index"`
	KeyMasked      string `json:"keyMasked"`
	UserID         string `json:"userId" gorm:"index"`
	UserName       string `json:"userName"`
	FaceValueCents int    `json:"faceValueCents"`
	CreatedAt      string `json:"createdAt"`
}

type LicenseRedeemLogList struct {
	Items []LicenseRedeemLog `json:"items"`
	Total int64              `json:"total"`
}
