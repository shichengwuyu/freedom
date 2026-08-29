package handler

import (
	"net/http"
	"time"

	"github.com/tigerowo/freedom/service"
)

// pricingResponse 公开定价 API 响应（Sprint 3 引入）。
type pricingResponse struct {
	Groups []service.PricingGroup `json:"groups"`
	Models []service.PricingModel `json:"models"`
	Now    string                 `json:"now"`
}

// GetPricing 公开接口（不需登录）。返回所有 active group 的定价信息。
// 前端 /wallet/pricing 页面用。
func GetPricing(w http.ResponseWriter, r *http.Request) {
	groups, err := service.ListActiveUserGroups()
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "读取用户组失败")
		return
	}
	models, err := service.ListPublicPricing()
	if err != nil {
		FailWithStatus(w, http.StatusInternalServerError, "读取定价失败")
		return
	}
	// 转换 UserGroup → PricingGroup
	groupOut := make([]service.PricingGroup, 0, len(groups))
	for _, g := range groups {
		groupOut = append(groupOut, service.PricingGroup{
			ID:          g.ID,
			DisplayName: g.DisplayName,
			Ratio:       service.GetGroupRatio(g.ID),
			IsDefault:   g.IsDefault,
		})
	}
	OK(w, pricingResponse{
		Groups: groupOut,
		Models: models,
		Now:    time.Now().UTC().Format(time.RFC3339),
	})
}
