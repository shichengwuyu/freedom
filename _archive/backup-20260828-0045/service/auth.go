package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tigerowo/freedom/config"
	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type TokenClaims struct {
	UserID   string         `json:"userId"`
	Username string         `json:"username"`
	Role     model.UserRole `json:"role"`
	jwt.RegisteredClaims
}

type userExtra struct {
	LinuxDo any `json:"linuxDo,omitempty"`
}

func EnsureDefaultAdmin() error {
	adminUser := strings.TrimSpace(config.Cfg.AdminUsername)
	adminPass := strings.TrimSpace(config.Cfg.AdminPassword)
	if adminUser == "" || adminPass == "" {
		return nil
	}
	WarnDefaultSecurityConfig()
	hash, err := hashPassword(adminPass)
	if err != nil {
		return err
	}
	existing, ok, err := repository.GetUserByUsername(adminUser)
	if err != nil {
		return err
	}
	if ok {
		existing.Role = model.UserRoleAdmin
		existing.Password = hash
		existing.Status = model.UserStatusActive
		existing.UpdatedAt = now()
		_, err = repository.SaveUser(existing)
		return err
	}
	_, err = repository.SaveUser(model.User{
		ID:        newID("user"),
		Username:  adminUser,
		Password:  hash,
		Role:      model.UserRoleAdmin,
		Status:    model.UserStatusActive,
		CreatedAt: now(),
		UpdatedAt: now(),
	})
	return err
}

func Register(username string, password string) (model.AuthSession, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.AuthSession{}, err
	}
	normalizedSettings := normalizeSettings(settings)
	if normalizedSettings.Public.Auth.AllowRegister != nil && !*normalizedSettings.Public.Auth.AllowRegister {
		return model.AuthSession{}, safeMessageError{message: "当前未开放注册"}
	}
	username = strings.TrimSpace(username)
	if strings.ContainsAny(username, " \t\r\n") {
		return model.AuthSession{}, safeMessageError{message: "用户名不能包含空格"}
	}
	if username == "" || password == "" {
		return model.AuthSession{}, safeMessageError{message: "用户名和密码不能为空"}
	}
	if _, ok, err := repository.GetUserByUsername(username); err != nil || ok {
		if err != nil {
			return model.AuthSession{}, err
		}
		return model.AuthSession{}, safeMessageError{message: "用户名已存在"}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return model.AuthSession{}, err
	}
	user, err := repository.SaveUser(model.User{
		ID:        newID("user"),
		Username:  username,
		Password:  hash,
		Role:      model.UserRoleUser,
		Status:    model.UserStatusActive,
		CreatedAt: now(),
		UpdatedAt: now(),
	})
	if err != nil {
		return model.AuthSession{}, err
	}
	return newSession(user)
}

func Login(username string, password string) (model.AuthSession, error) {
	user, ok, err := repository.GetUserByUsername(strings.TrimSpace(username))
	if err != nil {
		return model.AuthSession{}, err
	}
	if !ok || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return model.AuthSession{}, safeMessageError{message: "用户名或密码错误"}
	}
	if user.Status == model.UserStatusBan {
		return model.AuthSession{}, safeMessageError{message: "账号已被禁用"}
	}
	normalizeUserDefaults(&user)
	user.LastLoginAt = now()
	user.UpdatedAt = now()
	user, err = repository.SaveUser(user)
	if err != nil {
		return model.AuthSession{}, err
	}
	return newSession(user)
}

const oauthStateCookieName = "oauth_state"

// LinuxDoAuthorizeURL 构造 OAuth 授权 URL，同时设置 CSRF 保护 cookie。
//
// state 格式：base64(nonce) + "." + base64(hmac(redirect))
// 同时把 nonce 写入短期 cookie（oauth_state），回调时校验 cookie 与 state 中的 nonce 匹配，
// 防止登录 CSRF（攻击者用自己的 code 让受害者登录到攻击者账户）。
func LinuxDoAuthorizeURL(w http.ResponseWriter, r *http.Request, redirect string) (string, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return "", err
	}
	settings = normalizeSettings(settings)
	linuxDo := settings.Private.Auth.LinuxDo
	if !settings.Public.Auth.LinuxDo.Enabled {
		return "", safeMessageError{message: "Linux.do 登录未开启"}
	}
	if strings.TrimSpace(linuxDo.ClientID) == "" || strings.TrimSpace(linuxDo.ClientSecret) == "" {
		return "", safeMessageError{message: "Linux.do 登录未配置"}
	}
	nonce, err := randomNonce()
	if err != nil {
		return "", err
	}
	signedState, err := signOAuthState(nonce, redirect)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    nonce,
		Path:     "/",
		MaxAge:   600, // 10 分钟有效
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
	})
	values := url.Values{}
	values.Set("client_id", linuxDo.ClientID)
	values.Set("redirect_uri", linuxDoRedirectURI(r))
	values.Set("response_type", "code")
	values.Set("scope", "read")
	values.Set("state", signedState)
	return config.Cfg.LinuxDoAuthorizeURL + "?" + values.Encode(), nil
}

func LoginWithLinuxDo(r *http.Request, code string, state string) (model.AuthSession, string, error) {
	// 登录 CSRF 唯一拦截点：state 校验失败必须立刻终止，**绝不**继续走 token exchange / profile。
	redirect, err := verifyOAuthState(r, state)
	if err != nil {
		return model.AuthSession{}, "/", err
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	settings = normalizeSettings(settings)
	linuxDo := settings.Private.Auth.LinuxDo
	if !settings.Public.Auth.LinuxDo.Enabled {
		return model.AuthSession{}, redirect, safeMessageError{message: "Linux.do 登录未开启"}
	}
	token, err := linuxDoAccessToken(r, code, linuxDo)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	profile, err := linuxDoProfile(token)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	linuxDoID := fmt.Sprint(profile.ID)
	if strings.TrimSpace(linuxDoID) == "" || linuxDoID == "0" {
		return model.AuthSession{}, redirect, safeMessageError{message: "Linux.do 用户信息无效"}
	}
	user, ok, err := repository.GetUserByLinuxDoID(linuxDoID)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	if !ok {
		if settings.Public.Auth.AllowRegister != nil && !*settings.Public.Auth.AllowRegister {
			return model.AuthSession{}, redirect, safeMessageError{message: "当前未开放注册"}
		}
		user = model.User{
			ID:          newID("user"),
			Username:    linuxDoUsername(profile.Username, linuxDoID),
			DisplayName: strings.TrimSpace(profile.Name),
			AvatarURL:   linuxDoAvatar(profile.AvatarTemplate),
			Role:        model.UserRoleUser,
			LinuxDoID:   linuxDoID,
			Status:      model.UserStatusActive,
			CreatedAt:   now(),
		}
	} else if user.Status == model.UserStatusBan {
		return model.AuthSession{}, redirect, safeMessageError{message: "账号已被禁用"}
	}
	user.DisplayName = firstNonEmpty(profile.Name, user.DisplayName)
	user.AvatarURL = firstNonEmpty(linuxDoAvatar(profile.AvatarTemplate), user.AvatarURL)
	user.LastLoginAt = now()
	user.UpdatedAt = now()
	extra, _ := json.Marshal(userExtra{LinuxDo: profile})
	user.Extra = string(extra)
	user, err = repository.SaveUser(user)
	if err != nil {
		return model.AuthSession{}, redirect, err
	}
	session, err := newSession(user)
	return session, redirect, err
}

func ParseToken(tokenText string) (TokenClaims, error) {
	claims := TokenClaims{}
	token, err := jwt.ParseWithClaims(tokenText, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("登录状态无效")
		}
		return []byte(config.Cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return TokenClaims{}, errors.New("登录状态无效")
	}
	return claims, nil
}

func CurrentAuthUser(tokenText string) (model.AuthUser, bool) {
	claims, err := ParseToken(tokenText)
	if err != nil {
		return model.AuthUser{}, false
	}
	user, ok, err := repository.GetUserByID(claims.UserID)
	if err != nil || !ok {
		return model.AuthUser{}, false
	}
	if user.Status == model.UserStatusBan {
		return model.AuthUser{}, false
	}
	return model.PublicUser(user), true
}

func ListUsers(q model.Query) (model.UserList, error) {
	users, total, err := repository.ListUsers(q)
	if err != nil {
		return model.UserList{}, err
	}
	for i := range users {
		users[i].Password = ""
		normalizeUserDefaults(&users[i])
	}
	return model.UserList{Items: users, Total: int(total)}, nil
}

func SaveUser(user model.User, password string) (model.User, error) {
	user.Username = strings.TrimSpace(user.Username)
	if strings.ContainsAny(user.Username, " \t\r\n") {
		return user, safeMessageError{message: "用户名不能包含空格"}
	}
	if user.Username == "" {
		return user, safeMessageError{message: "用户名不能为空"}
	}
	if user.Role == "" || user.Role == model.UserRoleGuest {
		user.Role = model.UserRoleUser
	}
	if user.Status == "" {
		user.Status = model.UserStatusActive
	}
	if saved, ok, err := repository.GetUserByUsername(user.Username); err != nil {
		return user, err
	} else if ok && saved.ID != user.ID {
		return user, safeMessageError{message: "用户名已存在"}
	}
	isCreate := user.ID == ""
	if isCreate {
		user.ID = newID("user")
		user.CreatedAt = now()
	} else if saved, ok, err := repository.GetUserByID(user.ID); err != nil {
		return user, err
	} else if ok {
		user.CreatedAt = saved.CreatedAt
		user.Password = saved.Password
		user.AvatarURL = saved.AvatarURL
		user.BalanceCents = saved.BalanceCents
		user.Extra = saved.Extra
		if user.LinuxDoID == "" {
			user.LinuxDoID = saved.LinuxDoID
		}
		user.LastLoginAt = saved.LastLoginAt
	}
	if password != "" {
		hash, err := hashPassword(password)
		if err != nil {
			return user, err
		}
		user.Password = hash
	}
	if isCreate && user.Password == "" {
		return user, safeMessageError{message: "密码不能为空"}
	}
	user.UpdatedAt = now()
	user, err := repository.SaveUser(user)
	user.Password = ""
	return user, err
}

func AdjustUserBalance(id string, cents int) (model.User, error) {
	db, err := repository.DB()
	if err != nil {
		return model.User{}, err
	}
	var result model.User
	err = db.Transaction(func(tx *gorm.DB) error {
		// 在事务内读取用户并加行锁，防止并发覆盖
		var user model.User
		if e := tx.Where("id = ?", id).First(&user).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return safeMessageError{message: "用户不存在"}
			}
			return e
		}
		oldBalance := user.BalanceCents
		user.BalanceCents = cents
		user.UpdatedAt = now()
		if e := tx.Model(&user).Where("id = ?", id).Updates(map[string]any{
			"balance_cents": cents,
			"updated_at":    now(),
		}).Error; e != nil {
			return e
		}
		if oldBalance != cents {
			log := model.BalanceLog{
				ID:        newID("balance"),
				UserID:    user.ID,
				Type:      model.BalanceLogTypeManualAdjust,
				Amount:    cents - oldBalance,
				Balance:   cents,
				Remark:    "后台手动调整",
				CreatedAt: now(),
			}
			if e := tx.Create(&log).Error; e != nil {
				return e
			}
		}
		user.Password = ""
		result = user
		return nil
	})
	return result, err
}

// assertHoldMatches 校验已有 hold 的 (userID, requestID, amount, model, path) 与新请求
// 完全一致；任一不一致即视为不同的请求，返回 error（不复用 hold）。
// 这是 PR-3 加固：避免用户/调用方用同一个 requestID 调不同金额/模型的请求来绕开扣费，
// 也避免已 cancelled/settled hold 被静默复用（repository.GetBalanceHoldByUserAndRequest
// 已经只返回 status=held 的行，此处只补字段比对）。
func assertHoldMatches(existing *model.BalanceHold, userID, modelName string, cents int, path, requestID string) error {
	if existing == nil {
		return safeMessageError{message: "hold 记录不存在"}
	}
	if existing.Status != model.BalanceHoldHeld {
		return safeMessageError{message: "hold 已结算或取消，不能复用（requestID 已用过）"}
	}
	if existing.UserID != userID {
		return safeMessageError{message: "hold 与当前用户不匹配"}
	}
	if existing.RequestID != requestID {
		return safeMessageError{message: "hold 与当前 requestID 不匹配"}
	}
	if existing.Amount != cents {
		return safeMessageError{message: "hold 金额与新请求不一致，请使用新 requestID 重试"}
	}
	if existing.Model != modelName {
		return safeMessageError{message: "hold 模型与新请求不一致，请使用新 requestID 重试"}
	}
	if existing.Path != path {
		return safeMessageError{message: "hold 接口路径与新请求不一致，请使用新 requestID 重试"}
	}
	return nil
}
// 同一 user 下重复调用相同 requestID：若 hold 已存在且字段完全一致 → 复用 holdID（不再扣款）；
// 否则在事务里新建 hold + 扣减余额 + 写扣款流水。返回 holdID 供后续 Settle/Cancel。
//
// 幂等键语义（2026-08-27 收紧）：复用同一 hold 要求 (userID, requestID, amount, model, path)
// 全部一致；任一不一致即视为不同的请求，**不**复用已有 hold —— 这避免以下攻击/误用：
//   1. 用户先用便宜/0 成本请求拿到 requestID，再用同一 requestID 调昂贵模型，绕开扣费；
//   2. 调用方错把已 cancelled/settled hold 当成"待结算"复用。
// 这时 service 返回 error，调用方应改用新 requestID 重试。
//
// requestID 必须是调用方生成的稳定幂等键（前端 UUID 或后端 task ID），失败的请求也必须传
// 相同的 requestID，否则退款会变成凭空涨分（这是 no-hold refund 的最高风险场景）。
func ConsumeUserBalanceWithHold(userID, modelName string, cents int, path, requestID string) (string, error) {
	if cents <= 0 {
		return "", nil
	}
	if strings.TrimSpace(requestID) == "" {
		// 没有幂等键的 consume 不安全 — 失败退费时无法幂等，会凭空涨分。
		return "", safeMessageError{message: "ConsumeUserBalanceWithHold 必须传入非空 requestID"}
	}
	userID = strings.TrimSpace(userID)
	modelName = strings.TrimSpace(modelName)
	path = strings.TrimSpace(path)
	requestID = strings.TrimSpace(requestID)
	// 事务外先查一次幂等命中（快路径，避免无谓开事务）。
	// repository.GetBalanceHoldByUserAndRequest 只返回 status=held 的行；
	// 字段全一致才复用，否则报错要求调用方换 requestID。
	if existing, found, err := repository.GetBalanceHoldByUserAndRequest(userID, requestID); err != nil {
		return "", err
	} else if found {
		if err := assertHoldMatches(existing, userID, modelName, cents, path, requestID); err != nil {
			return "", err
		}
		return existing.ID, nil
	}
	db, err := repository.DB()
	if err != nil {
		return "", err
	}
	holdID := ""
	timestamp := now()
	err = db.Transaction(func(tx *gorm.DB) error {
		// 事务内再查一次幂等（防止两个并发请求都通过事务外检查）；
		// 走"跨状态"查询以便看到并发先建好的任意状态行，再做严格校验。
		if existing, found, err := repository.FindBalanceHoldByUserAndRequest(userID, requestID); err != nil {
			return err
		} else if found {
			if err := assertHoldMatches(existing, userID, modelName, cents, path, requestID); err != nil {
				return err
			}
			holdID = existing.ID
			return nil
		}
		hold := model.BalanceHold{
			ID:        newID("hold"),
			UserID:    userID,
			Amount:    cents,
			Status:    model.BalanceHoldHeld,
			RequestID: requestID,
			Model:     modelName,
			Path:      path,
			CreatedAt: timestamp,
		}
		if err := tx.Create(&hold).Error; err != nil {
			// 如果是 unique 约束冲突（并发下另一事务先建了），查回最新的行并按严格规则判定。
			// 此时不再是"事务外快路径命中"，所以需要从 DB 读最新行做字段比对。
			if existingHold, found2, qErr := repository.FindBalanceHoldByUserAndRequest(userID, requestID); qErr != nil {
				return qErr
			} else if found2 {
				if err := assertHoldMatches(existingHold, userID, modelName, cents, path, requestID); err != nil {
					return err
				}
				holdID = existingHold.ID
				return nil
			}
			return err
		}
		res := tx.Model(&model.User{}).
			Where("id = ? AND balance_cents >= ?", userID, cents).
			Updates(map[string]any{
				"balance_cents": gorm.Expr("balance_cents - ?", cents),
				"updated_at":    now(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return safeMessageError{message: "余额不足"}
		}
		extra, _ := json.Marshal(map[string]string{"model": modelName, "path": path, "holdId": hold.ID})
		var afterUser model.User
		if e := tx.Where("id = ?", userID).First(&afterUser).Error; e != nil {
			return e
		}
		log := model.BalanceLog{
			ID:        newID("balance"),
			UserID:    userID,
			Type:      model.BalanceLogTypeGenerationConsume,
			Amount:    -cents,
			Balance:   afterUser.BalanceCents,
			RelatedID: hold.ID,
			Remark:    "调用模型 " + modelName,
			Extra:     string(extra),
			CreatedAt: now(),
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		holdID = hold.ID
		return nil
	})
	if err != nil {
		return "", err
	}
	return holdID, nil
}

// SettleBalanceHold 成功结算：hold.status=held → settled。已 settled 重复调用返回错误。
func SettleBalanceHold(holdID string) error {
	if strings.TrimSpace(holdID) == "" {
		return nil // 非幂等场景（cents<=0）外的 no-op
	}
	db, err := repository.DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 在事务内读取 hold 并加行锁，避免 TOCTOU 竞态导致双重结算/退款。
		var hold model.BalanceHold
		err := tx.Where("id = ?", holdID).First(&hold).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return safeMessageError{message: "余额占用记录不存在"}
		}
		if err != nil {
			return err
		}
		switch hold.Status {
		case model.BalanceHoldSettled:
			return safeMessageError{message: "余额占用已结算，禁止重复操作"}
		case model.BalanceHoldCancelled:
			return safeMessageError{message: "余额占用已取消，不能再结算"}
		case model.BalanceHoldHeld:
			// 条件更新：WHERE status=held 防止并发竞态。
			res := tx.Model(&model.BalanceHold{}).
				Where("id = ? AND status = ?", holdID, model.BalanceHoldHeld).
				Updates(map[string]any{
					"status":     model.BalanceHoldSettled,
					"settled_at": now(),
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return safeMessageError{message: "余额占用状态已变更，禁止重复操作"}
			}
			return nil
		default:
			return safeMessageError{message: "未知 hold 状态: " + string(hold.Status)}
		}
	})
}

// CancelBalanceHold 失败取消 + 退款：hold.status=held → cancelled，并退还余额 + 写退款流水。
// 幂等：cancelled 状态再次调用返回 no-op（已退过就不退第二次）；settled 拒绝（不能退已结算的）。
func CancelBalanceHold(holdID string) error {
	if strings.TrimSpace(holdID) == "" {
		return nil
	}
	db, err := repository.DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 在事务内读取 hold 并加行锁，避免 TOCTOU 竞态导致双重退款。
		var hold model.BalanceHold
		err := tx.Where("id = ?", holdID).First(&hold).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return safeMessageError{message: "余额占用记录不存在"}
		}
		if err != nil {
			return err
		}
		switch hold.Status {
		case model.BalanceHoldCancelled:
			return nil // 幂等 no-op
		case model.BalanceHoldHeld, model.BalanceHoldSettled:
			// held：业务在扣款后、结算前失败 → 取消并退款。
			// settled：预付费模式下任务已入库（hold 已结算），但下游异步发现业务失败
			// （如视频生成失败）→ 允许从 settled 退回，状态转 cancelled，退款一次。
			// 两种状态共用 cancelled 终态：首次调用退款并置 cancelled，后续调用命中
			// cancelled 分支 → no-op，天然防双退。
			res := tx.Model(&model.BalanceHold{}).
				Where("id = ? AND status = ?", holdID, hold.Status).
				Updates(map[string]any{
					"status":       model.BalanceHoldCancelled,
					"cancelled_at": now(),
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				// 已被并发 settle/cancel，不再退款（幂等安全）
				return nil
			}
			// 退款
			refundRes := tx.Model(&model.User{}).Where("id = ?", hold.UserID).Updates(map[string]any{
				"balance_cents": gorm.Expr("balance_cents + ?", hold.Amount),
				"updated_at":    now(),
			})
			if refundRes.Error != nil {
				return refundRes.Error
			}
		extra, _ := json.Marshal(map[string]string{"model": hold.Model, "path": hold.Path, "holdId": hold.ID})
		var afterUser model.User
		if e := tx.Where("id = ?", hold.UserID).First(&afterUser).Error; e != nil {
			return e
		}
		logEntry := model.BalanceLog{
			ID:        newID("balance"),
			UserID:    hold.UserID,
			Type:      model.BalanceLogTypeGenerationRefund,
			Amount:    hold.Amount,
			Balance:   afterUser.BalanceCents,
			RelatedID: hold.ID,
			Remark:    "模型调用失败返还 " + hold.Model,
			Extra:     string(extra),
			CreatedAt: now(),
		}
			if err := tx.Create(&logEntry).Error; err != nil {
				return err
			}
			return nil
		default:
			return safeMessageError{message: "未知 hold 状态: " + string(hold.Status)}
		}
	})
}

func ListBalanceLogs(q model.Query) (model.BalanceLogList, error) {
	logs, total, err := repository.ListBalanceLogs(q)
	if err != nil {
		return model.BalanceLogList{}, err
	}
	return model.BalanceLogList{Items: logs, Total: int(total)}, nil
}

func SaveBalanceLog(log model.BalanceLog) (model.BalanceLog, error) {
	if log.ID == "" {
		log.ID = newID("balance")
		log.CreatedAt = now()
	}
	return repository.SaveBalanceLog(log)
}

func DeleteBalanceLog(id string) error {
	return repository.DeleteBalanceLog(id)
}

func DeleteUser(id string) error {
	return repository.DeleteUser(id)
}

// SweepStuckBalanceHolds 扫描并清理卡在 held 状态超过 maxAge 的余额占用（2026-08-17 引入）。
//
// 触发场景：handler 层 holdSettle/holdCancel 自身 DB 抽风/进程 OOM 时，hold 会卡在 held 状态
// → 用户余额被扣但 hold 永不结算/不退。周期性扫描器（balance_hold_sweep_scheduler）会调它。
//
// CancelBalanceHold 自身幂等（cancelled 状态 no-op），重复调用安全；单次最多扫描 200 条
// 避免一次性大批量退款拖死 DB。
//
// 返回 (scanned, cancelled, err)：scanned 是本轮查到的 stuck 数量，cancelled 是实际取消成功的数量。
func SweepStuckBalanceHolds(maxAge time.Duration) (int, int, error) {
	if maxAge <= 0 {
		maxAge = 30 * time.Minute
	}
	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339)
	holds, err := repository.ListStuckBalanceHolds(cutoff, 200)
	if err != nil {
		return 0, 0, err
	}
	cancelled := 0
	for i := range holds {
		hold := holds[i]
		// 在循环内逐条调用；CancelBalanceHold 自身幂等，即便中途被人手动 settle/cancelled 也不会出错。
		if err := CancelBalanceHold(hold.ID); err != nil {
			log.Printf("sweep stuck balance hold failed: holdID=%s userID=%s amount=%d err=%v", hold.ID, hold.UserID, hold.Amount, err)
			continue
		}
		cancelled++
	}
	if cancelled > 0 || len(holds) > 0 {
		log.Printf("sweep stuck balance holds: scanned=%d cancelled=%d maxAge=%s", len(holds), cancelled, maxAge)
	}
	return len(holds), cancelled, nil
}

// RefundFailedVideoTask 视频任务异步失败时退款（2026-08-22 改造：统一走 hold 状态机，杜绝双退）。
//
// 触发场景：handler/video_task.go 的 proxyAIVideoTaskRequest 是"预付费"模式，
// CreateVideoTask 成功就 SettleBalanceHold(taskCostCents)；如果上游异步失败（polling 路径
// 标 task=failed），用户实际没拿到视频，但钱被扣了。RefundFailedVideoTask 反向退账
// + 写一条退款流水，让用户账目平衡。
//
// 设计选择（2026-08-22 修正）：统一走 CancelBalanceHold(task.HoldID)，由 hold 状态机
// 做唯一互斥源——hold 一旦 cancelled（无论原来是 held 还是 settled），再次调用直接
// no-op，绝不会双退。这修复了旧实现（RefundUserBalance 直退、related_id 为空）与
// CancelBalanceHold 两条独立路径互不感知、导致同款退两次的 bug。
//
// 前置条件：CreateVideoTask 必须把 holdID 写进 task.HoldID（service/video_task.go）。
// 若 task 未持 holdID（历史数据 / 非预付费），则 no-op（不凭空涨分）。
func RefundFailedVideoTask(task model.VideoTask) error {
	if task.CostCents <= 0 {
		return nil
	}
	holdID := strings.TrimSpace(task.HoldID)
	if holdID == "" {
		// 没有 hold 关联（历史任务或非预付费），不退款也不涨分。
		log.Printf("refund failed video task skipped (no holdID): id=%s userID=%s costCents=%d", task.ID, task.UserID, task.CostCents)
		return nil
	}
	if err := CancelBalanceHold(holdID); err != nil {
		log.Printf("refund failed video task failed: id=%s holdID=%s costCents=%d err=%v", task.ID, holdID, task.CostCents, err)
		return err
	}
	return nil
}

func GuestUser() model.AuthUser {
	return model.AuthUser{ID: "", Username: "guest", Role: model.UserRoleGuest}
}

func newSession(user model.User) (model.AuthSession, error) {
	token, err := newToken(user)
	if err != nil {
		return model.AuthSession{}, err
	}
	return model.AuthSession{Token: token, User: model.PublicUser(user)}, nil
}

func newToken(user model.User) (string, error) {
	expireHours := config.Cfg.JWTExpireHours
	if expireHours <= 0 {
		expireHours = 168
	}
	claims := TokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Cfg.JWTSecret))
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func now() string {
	return time.Now().Format(time.RFC3339)
}

func newID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func normalizeUserDefaults(user *model.User) {
	if user.Status == "" {
		user.Status = model.UserStatusActive
	}
}

// UpdateCurrentUserProfile 更新当前登录用户的公开资料（仅昵称和头像）。
func UpdateCurrentUserProfile(ctx context.Context, displayName string, avatarURL string) (model.AuthUser, error) {
	var empty model.AuthUser
	current, ok := UserFromContext(ctx)
	if !ok || current.ID == "" {
		return empty, safeMessageError{message: "请先登录"}
	}
	if len(displayName) > 50 {
		return empty, safeMessageError{message: "昵称长度不能超过 50 个字符"}
	}
	if len(avatarURL) > 1024 {
		return empty, safeMessageError{message: "头像地址过长"}
	}
	user, found, err := repository.GetUserByID(current.ID)
	if err != nil {
		return empty, err
	}
	if !found {
		return empty, safeMessageError{message: "用户不存在"}
	}
	user.DisplayName = displayName
	user.AvatarURL = avatarURL
	user.UpdatedAt = now()
	user, err = repository.SaveUser(user)
	if err != nil {
		return empty, err
	}
	return model.PublicUser(user), nil
}

type linuxDoTokenResponse struct {
	AccessToken string `json:"access_token"`
}

type linuxDoUserResponse struct {
	ID             int64  `json:"id"`
	Username       string `json:"username"`
	Name           string `json:"name"`
	AvatarTemplate string `json:"avatar_template"`
}

func linuxDoAccessToken(r *http.Request, code string, setting model.PrivateLinuxDoAuthSetting) (string, error) {
	values := url.Values{}
	values.Set("client_id", setting.ClientID)
	values.Set("client_secret", setting.ClientSecret)
	values.Set("grant_type", "authorization_code")
	values.Set("code", code)
	values.Set("redirect_uri", linuxDoRedirectURI(r))
	req, err := http.NewRequest(http.MethodPost, config.Cfg.LinuxDoTokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var payload linuxDoTokenResponse
	if err := doLinuxDoJSON(req, &payload); err != nil {
		return "", err
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", safeMessageError{message: "Linux.do 登录失败"}
	}
	return payload.AccessToken, nil
}

func linuxDoRedirectURI(r *http.Request) string {
	return RequestOrigin(r) + "/api/auth/linux-do/callback"
}

func linuxDoProfile(token string) (linuxDoUserResponse, error) {
	req, err := http.NewRequest(http.MethodGet, config.Cfg.LinuxDoUserInfoURL, nil)
	if err != nil {
		return linuxDoUserResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	var payload linuxDoUserResponse
	err = doLinuxDoJSON(req, &payload)
	return payload, err
}

// linuxDoHTTPClient 专用 HTTP client，设置 30 秒超时防止 OAuth 流程无限挂起。
var linuxDoHTTPClient = &http.Client{Timeout: 30 * time.Second}

func doLinuxDoJSON(req *http.Request, payload any) error {
	resp, err := linuxDoHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 上限
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return safeMessageError{message: "Linux.do 登录失败"}
	}
	return json.NewDecoder(bytes.NewReader(body)).Decode(payload)
}

func linuxDoUsername(username string, id string) string {
	base := strings.TrimSpace(username)
	if base == "" {
		base = "linuxdo-" + id
	}
	if _, ok, err := repository.GetUserByUsername(base); err != nil || !ok {
		return base
	}
	return base + "-" + id
}

func linuxDoAvatar(template string) string {
	if strings.TrimSpace(template) == "" {
		return ""
	}
	if strings.HasPrefix(template, "//") {
		template = "https:" + template
	}
	if strings.HasPrefix(template, "/") {
		template = "https://linux.do" + template
	}
	return strings.ReplaceAll(template, "{size}", "120")
}

// oauthStatePayload state 内部载荷：nonce 用于 CSRF 校验，redirect 用于登录后回跳。
type oauthStatePayload struct {
	Nonce    string `json:"n"`
	Redirect string `json:"r"`
}

// randomNonce 生成 16 字节随机 nonce（base64 编码）。
func randomNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// signOAuthState 构造 state = base64(json(payload)) + "." + base64(hmac(json(payload)))。
func signOAuthState(nonce, redirect string) (string, error) {
	payload, err := json.Marshal(oauthStatePayload{Nonce: nonce, Redirect: redirect})
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(config.Cfg.JWTSecret))
	mac.Write(payload)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString(payload) + "." + sig, nil
}

// verifyOAuthState 校验 state 签名 + cookie nonce 匹配，返回安全重定向路径。
// 任何校验失败都必须返回 error —— 这是登录 CSRF 的唯一拦截点：
// 如果失败时静默回退 "/" 继续走 token exchange/profile，攻击者用自己的 OAuth code
// 引诱受害者点回调链接，就能把受害者登录到攻击者账户。
// 调用方必须把 error 透传并终止整个登录流程（不再调 setAuthCookie）。
func verifyOAuthState(r *http.Request, state string) (redirect string, err error) {
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		return "", safeMessageError{message: "OAuth state 不合法"}
	}
	payloadBytes, decodeErr := base64.RawURLEncoding.DecodeString(parts[0])
	if decodeErr != nil {
		return "", safeMessageError{message: "OAuth state 不合法"}
	}
	mac := hmac.New(sha256.New, []byte(config.Cfg.JWTSecret))
	mac.Write(payloadBytes)
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return "", safeMessageError{message: "OAuth state 签名校验失败"}
	}
	var payload oauthStatePayload
	if jsonErr := json.Unmarshal(payloadBytes, &payload); jsonErr != nil {
		return "", safeMessageError{message: "OAuth state 解析失败"}
	}
	cookie, cookieErr := r.Cookie(oauthStateCookieName)
	if cookieErr != nil || strings.TrimSpace(cookie.Value) == "" || cookie.Value != payload.Nonce {
		return "", safeMessageError{message: "OAuth state cookie 缺失或不匹配"}
	}
	return safeRedirectPath(payload.Redirect), nil
}

// safeRedirectPath 仅放行站内相对路径，拦截开放重定向。浏览器会忽略 URL 中的
// Tab/换行/回车，并把 //host 或 /\host 解析为协议相对的跨站地址，因此先剥离这些
// 控制字符，再拒绝 // 与 /\ 前缀。
func safeRedirectPath(redirect string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, redirect)
	if !strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "//") || strings.HasPrefix(cleaned, "/\\") {
		return "/"
	}
	return cleaned
}

// RequestOrigin 构造用于 OAuth redirect_uri 和登录回跳的站点 origin。
//
// 安全策略：优先使用配置中的 PUBLIC_BASE_URL（管理员可控、不可被请求伪造）；
// 未配置时回退到请求 Host。不再信任 X-Forwarded-Host / X-Forwarded-Proto header
// —— 在 SetTrustedProxies(nil) 下这些 header 可被客户端伪造，用于构造 redirect_uri
// 或把 JWT token 重定向到攻击者域名。
func RequestOrigin(r *http.Request) string {
	if base := strings.TrimSpace(config.Cfg.PublicBaseURL); base != "" {
		return base
	}
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}
	return proto + "://" + r.Host
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func WarnDefaultSecurityConfig() {
	if config.Cfg.AdminUsername == "admin" && config.Cfg.AdminPassword == "freedom" {
		log.Println("WARNING: using default admin credentials, please set ADMIN_USERNAME and ADMIN_PASSWORD to safer values before deployment")
	}
}
