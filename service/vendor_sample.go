package service

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// VendorSampleInput 浏览器插件回传的一条样本（字段含义见 model.VendorApiSample）
// 插件把在官网真实发起的一次请求/响应序列化后通过 /api/v1/vendor/capture-sample 回传。
type VendorSampleInput struct {
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	RequestBody     string            `json:"requestBody,omitempty"`
	ResponseStatus  int               `json:"responseStatus"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	ResponseBody    string            `json:"responseBody,omitempty"`
	ContentType     string            `json:"contentType,omitempty"`
}

const (
	sampleMaxBodyLen = 64 * 1024 // 单条请求/响应体上限 64KB，避免样本表膨胀
	sampleMaxHdrLen = 16 * 1024
)

// 生成类关键词：命中 URL 或请求体即认为“很可能是生图/生视频接口”
var generationKeywords = []string{
	"image", "generate", "gen", "draw", "txt2img", "img2img",
	"creation", "create", "task", "prompt", "diffusion", "paint",
	"render", "upscale", "sd", "imagine", "art", "ai-paint",
}

// 敏感接口关键词：命中则直接排除，避免误抓登录/改密等含凭据的请求
var sensitiveURLKeywords = []string{
	"login", "pass", "auth", "token", "oauth", "signin", "sign-in",
	"register", "signup", "sign-up", "password", "logout", "captcha", "verify",
}

// SaveVendorApiSample 校验 + 入库一条样本。要求用户已绑定并激活该供应商账户
// （否则后端没有该用户的 Cookie，样本无法用于重放）。
func SaveVendorApiSample(userID, vendorType string, in VendorSampleInput) (*model.VendorApiSample, error) {
	if !model.ValidVendorType(vendorType) || vendorType == model.VendorTypeOfficial {
		return nil, errors.New("供应商类型不合法")
	}
	if userID == "" {
		return nil, errors.New("未登录")
	}
	// 必须已经绑定该供应商（样本是带该用户 Cookie 重放的依据）
	acct, err := repository.GetUserVendorAccountByType(userID, vendorType)
	if err != nil {
		return nil, err
	}
	if acct == nil {
		return nil, errors.New("请先在「云端供应商」里绑定该供应商账户，再回到官网生成一次以采集样本")
	}

	sample := buildSampleFromInput(userID, vendorType, in)
	saved, err := repository.CreateVendorApiSample(sample)
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func buildSampleFromInput(userID, vendorType string, in VendorSampleInput) model.VendorApiSample {
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = "POST"
	}
	urlStr := strings.TrimSpace(in.URL)
	reqBody := sampleTruncate(in.RequestBody, sampleMaxBodyLen)
	respBody := sampleTruncate(in.ResponseBody, sampleMaxBodyLen)

	reqHdrJSON := ""
	if len(in.RequestHeaders) > 0 {
		if b, e := json.Marshal(in.RequestHeaders); e == nil {
			reqHdrJSON = sampleTruncate(string(b), sampleMaxHdrLen)
		}
	}
	respHdrJSON := ""
	if len(in.ResponseHeaders) > 0 {
		if b, e := json.Marshal(in.ResponseHeaders); e == nil {
			respHdrJSON = sampleTruncate(string(b), sampleMaxHdrLen)
		}
	}

	isGen := isLikelyGeneration(urlStr, reqBody)
	endpointGroup := normalizeEndpointGroup(urlStr)

	return model.VendorApiSample{
		UserID:              userID,
		VendorType:          vendorType,
		URL:                 urlStr,
		Method:              method,
		RequestHeadersJSON:  reqHdrJSON,
		RequestBody:         reqBody,
		ResponseStatus:      in.ResponseStatus,
		ResponseHeadersJSON: respHdrJSON,
		ResponseBody:        respBody,
		ContentType:         strings.TrimSpace(in.ContentType),
		IsLikelyGeneration:  isGen,
		EndpointGroup:       endpointGroup,
		CreatedAt:           time.Now(),
	}
}

func isLikelyGeneration(rawURL, body string) bool {
	hay := strings.ToLower(rawURL + " " + body)
	// 先排除登录/鉴权类敏感接口
	for _, kw := range sensitiveURLKeywords {
		if strings.Contains(hay, kw) {
			return false
		}
	}
	for _, kw := range generationKeywords {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

// normalizeEndpointGroup 把 URL path 里的 ID/数字/UUID 段替换为 :param，
// 便于把不同请求归到同一接口分组（适配器后续按分组找生成接口）。
func normalizeEndpointGroup(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return rawURL
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, s := range segs {
		if isIDLike(s) {
			segs[i] = ":param"
		}
	}
	return "/" + strings.Join(segs, "/")
}

func isIDLike(s string) bool {
	if len(s) >= 8 && (strings.Contains(s, "-") || isAllDigits(s)) {
		return true
	}
	return false
}

func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func sampleTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ListVendorApiSamples 透传查询
func ListVendorApiSamples(userID, vendorType string, onlyGeneration bool, limit int) ([]model.VendorApiSample, error) {
	return repository.ListVendorApiSamples(userID, vendorType, onlyGeneration, limit)
}

// DeleteVendorApiSamples 透传删除
func DeleteVendorApiSamples(userID, vendorType string) (int64, error) {
	return repository.DeleteVendorApiSamples(userID, vendorType)
}
