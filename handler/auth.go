package handler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

// AuthCookieName 是存 JWT 的 httpOnly cookie 名（与 middleware.AuthCookieName 保持一致）。
const AuthCookieName = "freedom_token"

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	InviterCode string `json:"inviterCode"`
}

type saveUserRequest struct {
	ID          string           `json:"id"`
	Username    string           `json:"username"`
	Password    string           `json:"password"`
	Email       string           `json:"email"`
	DisplayName string           `json:"displayName"`
	Role        model.UserRole   `json:"role"`
	Status      model.UserStatus `json:"status"`
	GroupID     string           `json:"groupId"` // Sprint 3
}

type adjustUserBalanceRequest struct {
	CostCents int `json:"costCents"`
}

// setAuthCookie 把 JWT 写入 httpOnly cookie，前端无需手动管理 token。
func setAuthCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   168 * 3600,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

// clearAuthCookie 清除 httpOnly auth cookie。
func clearAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
	})
}

// isSecureRequest 判断请求是否通过 HTTPS 传输。
// 在 TLS 终止于反向代理时，r.TLS 为 nil，需检查 X-Forwarded-Proto header。
func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// 反向代理（nginx/Cloudflare）设置此 header 表示原始请求是 HTTPS
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// maxJSONBodyLimit 限制 JSON 请求体大小（1MB），防止大 body DoS。
const maxJSONBodyLimit = 1 << 20 // 1MB

func Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	session, err := service.Register(request.Username, request.Password, request.InviterCode)
	if err != nil {
		FailError(w, err)
		return
	}
	setAuthCookie(w, r, session.Token)
	OK(w, session)
}

func Login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	session, err := service.Login(request.Username, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	setAuthCookie(w, r, session.Token)
	OK(w, session)
}

func LinuxDoAuthorize(w http.ResponseWriter, r *http.Request) {
	authURL, err := service.LinuxDoAuthorizeURL(w, r, r.URL.Query().Get("redirect"), r.URL.Query().Get("inviterCode"))
	if err != nil {
		FailError(w, err)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func LinuxDoCallback(w http.ResponseWriter, r *http.Request) {
	session, redirect, err := service.LoginWithLinuxDo(r, r.URL.Query().Get("code"), r.URL.Query().Get("state"))
	// 清除 OAuth state cookie（一次性）
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	if err != nil {
		http.Redirect(w, r, loginRedirect(r, redirect, "", err.Error()), http.StatusFound)
		return
	}
	setAuthCookie(w, r, session.Token)
	// token 已在 httpOnly cookie 中，不再放 URL fragment
	http.Redirect(w, r, loginRedirect(r, redirect, "", ""), http.StatusFound)
}

func AdminLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	session, err := service.Login(request.Username, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	if session.User.Role != model.UserRoleAdmin {
		Fail(w, "需要管理员权限")
		return
	}
	setAuthCookie(w, r, session.Token)
	OK(w, session)
}

func CurrentUser(w http.ResponseWriter, r *http.Request) {
	if user, ok := service.UserFromContext(r.Context()); ok {
		OK(w, user)
		return
	}
	OK(w, service.GuestUser())
}

func AdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := service.ListUsers(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, users)
}

func AdminSaveUser(w http.ResponseWriter, r *http.Request) {
	var request saveUserRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	user, err := service.SaveUser(model.User{
		ID:          request.ID,
		Username:    request.Username,
		Email:       request.Email,
		DisplayName: request.DisplayName,
		Role:        request.Role,
		Status:      request.Status,
		GroupID:     request.GroupID, // Sprint 3
	}, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, user)
}

func AdminAdjustUserBalance(w http.ResponseWriter, r *http.Request, id string) {
	var request adjustUserBalanceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	if request.CostCents < 0 {
		Fail(w, "余额不能为负数")
		return
	}
	user, err := service.AdjustUserBalance(id, request.CostCents)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, user)
}

func AdminBalanceLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := service.ListBalanceLogs(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, logs)
}

// AdminSaveBalanceLog 后台调整某用户余额到目标值。
// 余额流水是对账记录，不可手写插入或编辑，因此这里直接调用 AdjustUserBalance，
// 由它在事务内调整 users.balance_cents 并写入正确的 manual_adjust 流水（含变动后余额）。
func AdminSaveBalanceLog(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UserID string `json:"userId"`
		Balance int   `json:"balance"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	if request.UserID == "" {
		Fail(w, "用户 ID 不能为空")
		return
	}
	if request.Balance < 0 {
		Fail(w, "余额不能为负数")
		return
	}
	user, err := service.AdjustUserBalance(request.UserID, request.Balance)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, user)
}

// AdminDeleteBalanceLog 余额流水是对账记录，不可删除，故禁用此接口。
func AdminDeleteBalanceLog(w http.ResponseWriter, r *http.Request, id string) {
	Fail(w, "余额流水不可删除")
}

func loginRedirect(r *http.Request, redirect string, token string, message string) string {
	// token 放 URL fragment（#token=...）而非 query string，避免出现在服务器访问日志和 Referer 头中。
	values := url.Values{}
	fragment := ""
	if strings.TrimSpace(token) != "" {
		fragValues := url.Values{}
		fragValues.Set("token", token)
		fragment = "#" + fragValues.Encode()
	}
	if strings.TrimSpace(message) != "" {
		values.Set("error", message)
	}
	if strings.TrimSpace(redirect) != "" {
		values.Set("redirect", redirect)
	}
	base := service.RequestOrigin(r) + "/login"
	if len(values) > 0 {
		base += "?" + values.Encode()
	}
	return base + fragment
}

func AdminDeleteUser(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteUser(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

type updateMyProfileRequest struct {
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

func UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	var request updateMyProfileRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyLimit)).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	user, err := service.UpdateCurrentUserProfile(r.Context(), request.DisplayName, request.AvatarURL)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, user)
}
