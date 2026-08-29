package service

import (
	"context"

	"github.com/tigerowo/freedom/model"
)

type userContextKey struct{}

func WithUser(ctx context.Context, user model.AuthUser) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func UserFromContext(ctx context.Context) (model.AuthUser, bool) {
	user, ok := ctx.Value(userContextKey{}).(model.AuthUser)
	return user, ok
}

// userTokenContextKey 用独立类型避免与 userContextKey 冲突；Sprint 1.1 引入。
type userTokenContextKey struct{}

// WithUserToken 把 sk- token 注入 ctx；只有 Bearer sk- 鉴权通过的请求才会调用。
// 下游 handler / service 用 UserTokenFromContext 取出，用于计费关联、IP/模型白名单等。
func WithUserToken(ctx context.Context, token model.UserToken) context.Context {
	return context.WithValue(ctx, userTokenContextKey{}, token)
}

func UserTokenFromContext(ctx context.Context) (model.UserToken, bool) {
	t, ok := ctx.Value(userTokenContextKey{}).(model.UserToken)
	return t, ok
}
