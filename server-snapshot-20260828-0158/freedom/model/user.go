package model

type UserRole string

const (
	UserRoleGuest UserRole = "guest"
	UserRoleUser  UserRole = "user"
	UserRoleAdmin UserRole = "admin"
)

type UserStatus string

const (
	UserStatusActive UserStatus = "active"
	UserStatusBan    UserStatus = "ban"
)

// User 系统用户。
// BalanceCents 账户余额，单位 = 分（cents，1 元 = 100 cents）。
type User struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	Username    string     `json:"username" gorm:"type:varchar(255);uniqueIndex"`
	Password    string     `json:"password,omitempty"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	AvatarURL   string     `json:"avatarUrl"`
	Role        UserRole   `json:"role"`
	BalanceCents int        `json:"balanceCents"`
	AffCode     string     `json:"affCode" gorm:"uniqueIndex"`
	AffCount    int        `json:"affCount"`
	InviterID   string     `json:"inviterId"`
	GithubID    string     `json:"githubId"`
	LinuxDoID   string     `json:"linuxDoId" gorm:"index"`
	WechatID    string     `json:"wechatId"`
	Status      UserStatus `json:"status"`
	LastLoginAt string     `json:"lastLoginAt"`
	Extra       string     `json:"extra" gorm:"type:text"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

// UserList 用户分页结果。
type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

// AuthUser 用户公开信息。
type AuthUser struct {
	ID           string   `json:"id"`
	Username     string   `json:"username"`
	DisplayName  string   `json:"displayName"`
	AvatarURL    string   `json:"avatarUrl"`
	Role         UserRole `json:"role"`
	BalanceCents int      `json:"balanceCents"`
	CreatedAt    string   `json:"createdAt"`
	UpdatedAt    string   `json:"updatedAt"`
}

// AuthSession 登录会话信息。
type AuthSession struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

func PublicUser(user User) AuthUser {
	return AuthUser{
		ID:           user.ID,
		Username:     user.Username,
		DisplayName:  user.DisplayName,
		AvatarURL:    user.AvatarURL,
		Role:         user.Role,
		BalanceCents: user.BalanceCents,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}

type BalanceLogType string

const (
	BalanceLogTypeManualAdjust      BalanceLogType = "manual_adjust"
	BalanceLogTypeGenerationConsume BalanceLogType = "generation_consume"
	BalanceLogTypeGenerationRefund  BalanceLogType = "generation_refund"
	BalanceLogTypeManualRecharge    BalanceLogType = "manual_recharge"
	BalanceLogTypeAffCommission      BalanceLogType = "aff_commission"
)

// AffCommissionStatus 邀请返佣结算状态。
const (
	AffCommissionStatusPending   string = "pending"
	AffCommissionStatusSettled   string = "settled"
	AffCommissionStatusCancelled string = "cancelled"
)

// AffCommissionLog 邀请返佣流水（一级直推：佣金只结算给直接邀请人）。
// 单位均为分（cents）。同一笔充值（recharge_id）只结算一次（UNIQUE 索引兜底幂等）。
type AffCommissionLog struct {
	ID             string `json:"id" gorm:"primaryKey"`
	InviterID      string `json:"inviterId" gorm:"index:idx_aff_inviter,priority:1"`
	InviteeID      string `json:"inviteeId"`
	RechargeID     string `json:"rechargeId" gorm:"uniqueIndex"`
	RechargeCents  int    `json:"rechargeCents"`
	Rate           string `json:"rate"` // 分成比例快照，如 "0.1000"
	CommissionCents int  `json:"commissionCents"`
	Status         string `json:"status"`
	SettledAt      string `json:"settledAt"`
	CreatedAt      string `json:"createdAt"`
}

type AffCommissionLogList struct {
	Items []AffCommissionLog `json:"items"`
	Total int64              `json:"total"`
}

// BalanceLog 用户余额变更流水。
// Amount / Balance 单位 = 分（cents，正数表示入账，负数表示出账）。
type BalanceLog struct {
	ID        string         `json:"id" gorm:"primaryKey"`
	UserID    string         `json:"userId" gorm:"index"`
	Type      BalanceLogType `json:"type"`
	Amount    int            `json:"amount"`
	Balance   int            `json:"balance"`
	RelatedID string         `json:"relatedId"`
	Remark    string         `json:"remark"`
	Extra     string         `json:"extra" gorm:"type:text"`
	CreatedAt string         `json:"createdAt"`
}

type BalanceLogList struct {
	Items []BalanceLog `json:"items"`
	Total int          `json:"total"`
}

// BalanceHoldStatus 余额占用状态。
type BalanceHoldStatus string

const (
	// BalanceHoldHeld 已扣款，待结算（业务进行中）；不可重复扣费。
	BalanceHoldHeld BalanceHoldStatus = "held"
	// BalanceHoldSettled 成功结算（业务完成）；不会再退款。
	BalanceHoldSettled BalanceHoldStatus = "settled"
	// BalanceHoldCancelled 失败取消（已退款）；不会再退款。
	BalanceHoldCancelled BalanceHoldStatus = "cancelled"
)

// BalanceHold 余额占用记录（ConsumeUserBalance 的幂等键，2026-08-17 引入）。
//
// 关键字段：
//   - RequestID 调用方传入的幂等键；同一 user 下重复 Consume 相同 RequestID → 复用 hold，不再扣款。
//   - Amount 占用的余额，正数，单位 = 分。
//   - Status 三态机：held → settled（成功）/ cancelled（失败退款）。反向转换不允许。
//
// 命名约定：字段名不绑定"积分/分"语义，方便后续支付重构改名。
type BalanceHold struct {
	ID          string             `json:"id" gorm:"primaryKey"`
	UserID      string             `json:"userId" gorm:"index:idx_balance_hold_user_request,unique,priority:1"`
	Amount      int                `json:"amount"`
	Status      BalanceHoldStatus  `json:"status"`
	RequestID   string             `json:"requestId" gorm:"index:idx_balance_hold_user_request,unique,priority:2"`
	Model       string             `json:"model"`
	Path        string             `json:"path"`
	CreatedAt   string             `json:"createdAt"`
	SettledAt   string             `json:"settledAt,omitempty"`
	CancelledAt string             `json:"cancelledAt,omitempty"`
}

type BalanceHoldList struct {
	Items []BalanceHold `json:"items"`
	Total int           `json:"total"`
}
