package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

func LicensePurchaseConfig(w http.ResponseWriter, r *http.Request) {
	OK(w, map[string]any{
		"purchaseURL": service.GetPurchaseConfig(),
	})
}

type redeemLicenseKeyRequest struct {
	Key string `json:"key"`
}

// RedeemLicenseKey 用户自助兑换卡密。
func RedeemLicenseKey(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	var req redeemLicenseKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("redeem license key decode failed: %v", err)
		Fail(w, "参数解析失败")
		return
	}
	if req.Key == "" {
		Fail(w, "卡密不能为空")
		return
	}
	granted, newBalance, err := service.RedeemLicenseKey(user.ID, user.DisplayName, req.Key)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{
		"faceValueCentsGranted": granted,
		"newBalanceCents":       newBalance,
	})
}

func MyRedeemLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	q := parseQuery(r)
	items, total, err := service.ListMyRedeemLogs(user.ID, q)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, model.LicenseRedeemLogList{Items: items, Total: total})
}

func MyBalanceLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	list, err := service.ListMyBalanceLogs(user.ID, parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, list)
}
