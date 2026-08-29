package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
	"github.com/tigerowo/freedom/service"
)

// dispatchVendorProxy 在 proxyAIRequest 顶部尝试把请求分发给第三方供应商适配器。
//
// 命中条件：用户存在"激活中"的非官方供应商账户，且本次是图片生成/编辑请求。
// 命中后自行写响应并返回 handled=true（调用方应直接 return）；未命中返回 false 回落官方云端渠道。
//
// 设计要点（与文档 §5.2 对齐）：
//   - 供应商账户走用户自购额度，不经本项目 ModelChannel / 积分体系，因此分发成功后
//     不进入 selectAIRequestChannel / ConsumeUserBalance 等官方逻辑。
//   - 成功响应写 OpenAI 原生格式 { created, data:[{url}] }（不包 {code,data,msg} 壳），
//     前端 parseImagePayload 直接吃 data 数组；失败写 {code,msg} 壳（与官方 Fail 一致）。
func dispatchVendorProxy(w http.ResponseWriter, r *http.Request, path string, user model.AuthUser, body []byte, contentType, modelName string) (handled bool) {
	// 仅接管图片类端点；chat/responses/audio 一律回落官方，避免误伤。
	if path != "/images/generations" && path != "/images/edits" {
		return false
	}

	adapter, account, _, err := service.ResolveActiveVendorAdapter(user.ID)
	if err != nil {
		// 查询失败不阻断官方链路，记录后回落。
		log.Printf("dispatchVendorProxy: resolve active vendor adapter failed: %v", err)
		return false
	}
	if adapter == nil {
		// 用户没激活非官方账户 / 是 official / 已停用 / 适配器尚未实现，均回落官方链路。
		return false
	}

	// Token 刷新（singleflight 合并并发刷新，避免 LibTV 这类 AK/SK 模式反复触发）。
	if service.NeedsVendorTokenRefresh(account) {
		if rerr := service.SingleflightRefreshToken(r.Context(), account, adapter); rerr != nil {
			FailWithStatus(w, http.StatusUnauthorized, "供应商凭证刷新失败："+rerr.Error())
			return true
		}
	}

	input, perr := parseVendorImageInput(body, contentType, modelName)
	if perr != nil {
		FailWithStatus(w, http.StatusBadRequest, perr.Error())
		return true
	}

	output, gerr := adapter.GenerateImage(r.Context(), account, input)
	if gerr != nil {
		// 视频/文本等本期未接入 → 给清晰中文提示；其他生图失败如实透传。
		msg := "供应商生图失败：" + gerr.Error()
		if errors.Is(gerr, service.ErrNotSupported) {
			msg = gerr.Error()
		}
		FailWithStatus(w, http.StatusBadRequest, msg)
		return true
	}

	items := make([]map[string]any, 0, len(output.Items))
	for _, it := range output.Items {
		if len(it.Data) > 0 {
			items = append(items, map[string]any{"b64_json": base64.StdEncoding.EncodeToString(it.Data)})
		} else if it.URL != "" {
			items = append(items, map[string]any{"url": it.URL})
		}
	}
	if len(items) == 0 {
		FailWithStatus(w, http.StatusBadGateway, "供应商生图成功但未返回可用图片")
		return true
	}

	// 更新账户最近使用时间（best-effort，不阻塞响应）。
	if account.ID != "" {
		now := time.Now()
		account.LastUsedAt = now
		SaveUserVendorAccountBestEffort(*account)
	}

	writeJSON(w, map[string]any{
		"created": time.Now().Unix(),
		"data":    items,
	})
	return true
}

// parseVendorImageInput 把 OpenAI 原生图片请求体（JSON 或 multipart）解析为适配器入参。
// 兼容字段：prompt / size / n / negative_prompt / quality / seed。
func parseVendorImageInput(body []byte, contentType, modelName string) (service.GenerateImageInput, error) {
	input := service.GenerateImageInput{Model: modelName, Count: 1}
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			return input, fmt.Errorf("解析表单请求失败")
		}
		form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return input, fmt.Errorf("解析表单请求失败")
		}
		defer form.RemoveAll()
		input.Prompt = firstFormValue(form, "prompt")
		if v := firstFormValue(form, "size"); v != "" {
			input.Size = v
		}
		if v := firstFormValue(form, "n"); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &input.Count)
		}
		if v := firstFormValue(form, "negative_prompt"); v != "" {
			input.NegativePrompt = v
		}
		if strings.TrimSpace(input.Prompt) == "" {
			return input, fmt.Errorf("缺少 prompt 参数")
		}
		return input, nil
	}

	var req struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		Size           string `json:"size"`
		N              int    `json:"n"`
		NegativePrompt string `json:"negative_prompt"`
		Quality        string `json:"quality"`
		Seed           *int64 `json:"seed"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return input, fmt.Errorf("解析请求体失败：%v", err)
	}
	input.Prompt = req.Prompt
	if req.Size != "" {
		input.Size = req.Size
	}
	if req.N > 0 {
		input.Count = req.N
	}
	input.NegativePrompt = req.NegativePrompt
	input.Quality = req.Quality
	input.Seed = req.Seed
	if strings.TrimSpace(input.Prompt) == "" {
		return input, fmt.Errorf("缺少 prompt 参数")
	}
	return input, nil
}

func firstFormValue(form *multipart.Form, name string) string {
	if values := form.Value[name]; len(values) > 0 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

// SaveUserVendorAccountBestEffort 包装 repository 保存，失败仅记录日志不阻断主流程。
func SaveUserVendorAccountBestEffort(account model.UserVendorAccount) {
	if _, err := repository.SaveUserVendorAccount(account); err != nil {
		log.Printf("SaveUserVendorAccountBestEffort failed: %v", err)
	}
}
