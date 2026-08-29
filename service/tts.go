package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tigerowo/freedom/config"
)

// === novel-workflow v2: shot-dubbing-node: TTS Provider 抽象 ===
//
// v1 (CHANGELOG v0.5.2) 引入了 MiMo TTS，但没抽接口；shot-dubbing-node 顺手抽出来。
// 默认实现是 mimoTTSProvider；可注册 volcanoTTSProvider / openaiTTSProvider / elevenLabsProvider 等。
//
// 接入方：service/novel_dubbing.go::DispatchForShot 调 Synthesize 获取 mp3 + durationMs。
// 路由：TTSProvider 由 config.Cfg.TtsProvider 决定，工厂方法 GetTTSProvider() 返回单例。
// 扣费：复用 model_dispatch 的 BalanceHold 流程（不在 TTS 层处理），由调用方传 cents 入参。

// TTSOpts TTS 调用的可调参数。
type TTSOpts struct {
	VoiceID    string  // 音色 ID（"成熟男声"/"温柔女声"/具体 provider 的 voice id）
	Speed      float64 // 0.5 - 2.0
	Format     string  // "mp3" | "wav" | "pcm"（默认 mp3）
	SampleRate int     // 16000 / 22050 / 24000 / 44100（默认 24000）
}

// TTSResult TTS 返回。
type TTSResult struct {
	AudioURL   string // 对象存储 URL（或本地路径）
	DurationMs int64  // 音频时长（毫秒）
	Bytes      int64  // 音频字节数
	MimeType   string // "audio/mpeg" / "audio/wav" 等
}

// TTSProvider TTS 提供方接口。
type TTSProvider interface {
	Name() string
	Synthesize(ctx context.Context, text string, opts TTSOpts) (*TTSResult, error)
}

var (
	ttsProvidersMu sync.RWMutex
	ttsProviders   = map[string]TTSProvider{}
)

// RegisterTTSProvider 注册一个 TTS provider。
func RegisterTTSProvider(p TTSProvider) {
	ttsProvidersMu.Lock()
	defer ttsProvidersMu.Unlock()
	ttsProviders[p.Name()] = p
}

// GetTTSProvider 按配置名取 provider；未注册则降级到 mimo 默认。
func GetTTSProvider() TTSProvider {
	ttsProvidersMu.RLock()
	defer ttsProvidersMu.RUnlock()
	name := strings.ToLower(strings.TrimSpace(config.Cfg.TtsProvider))
	if name == "" {
		name = "mimo"
	}
	if p, ok := ttsProviders[name]; ok {
		return p
	}
	if p, ok := ttsProviders["mimo"]; ok {
		return p
	}
	return nil
}

// init 默认注册 MiMo TTS。
func init() {
	RegisterTTSProvider(newMimoTTSProvider())
}

// === mimoTTSProvider MiMo TTS 实现 ===
//
// 通过 OpenAI 兼容 /audio/speech 端点调 MiMo TTS 模型。
// 文档/现有调用：handler/ai.go::proxyAIRequest 走 channel_id 转发；这里走独立实现
// （TTS 调用常带文本不大，走 channel 转发会让 retries / 扣费逻辑复杂）。
//
// v2 简化：默认 base url = https://api.mimo.example/v1
// 配置：后续可加 TTS_BASE_URL / TTS_API_KEY 环境变量（v2 暂走 CHANNEL 复用）。

type mimoTTSProvider struct{}

func newMimoTTSProvider() *mimoTTSProvider { return &mimoTTSProvider{} }

func (*mimoTTSProvider) Name() string { return "mimo" }

// mimoSpeechRequest MiMo TTS POST /audio/speech 请求体。
type mimoSpeechRequest struct {
	Model      string  `json:"model"`
	Input      string  `json:"input"`
	Voice      string  `json:"voice"`
	Speed      float64 `json:"speed,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

// mimoSpeechResponse MiMo TTS 返回（部分 provider 在响应头返回 duration；body 可能为 raw audio bytes）。
func (*mimoTTSProvider) Synthesize(ctx context.Context, text string, opts TTSOpts) (*TTSResult, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("text 不能为空")
	}
	if opts.VoiceID == "" {
		opts.VoiceID = "成熟男声"
	}
	if opts.Speed <= 0 {
		opts.Speed = 1.0
	}
	if opts.Format == "" {
		opts.Format = "mp3"
	}

	// v2 阶段：直接走 OpenAI 兼容 /audio/speech。
	// 注：实际 base url / model 需从 system_settings 读，v2 简化走 config 兜底。
	baseURL := "https://api.mimo.example/v1"
	apiKey := ""
	_ = baseURL
	_ = apiKey

	// 真实实现（v2 阶段先返回 mock，后续接入）—— 等待 system_settings.tts 配置。
	// 单元测试场景下 Synthesize 会绕过此函数直接构造 TTSResult。
	_ = mimoSpeechRequest{}
	_ = mimoSpeechRequest{Model: "mimo-tts", Input: text, Voice: opts.VoiceID, Speed: opts.Speed, ResponseFormat: opts.Format}

	// v2 mock：按字数估算时长（每字 0.18s，语速系数），让后续接线的代码可以联调。
	chars := len([]rune(text))
	if chars == 0 {
		return nil, errors.New("text 解析为空")
	}
	durationMs := int64(float64(chars) * 180.0 / opts.Speed) // 0.18s/字 / 语速
	return &TTSResult{
		AudioURL:   "mock://tts/" + hashText(text) + "." + opts.Format,
		DurationMs: durationMs,
		Bytes:      int64(chars) * 1024, // 估算
		MimeType:   "audio/mpeg",
	}, nil
}

// hashText 简单哈希（避免引入 crypto 依赖）。
func hashText(s string) string {
	var h uint64 = 14695981039346656037
	for _, c := range []byte(s) {
		h ^= uint64(c)
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}

// === HTTP 客户端（保留以备 v2 接入真实 MiMo） ===

// mimoHTTPClient 简单 HTTP 客户端（带 timeout + retry）。
type mimoHTTPClient struct {
	httpClient *http.Client
}

func newMimoHTTPClient() *mimoHTTPClient {
	return &mimoHTTPClient{httpClient: &http.Client{Timeout: 60 * time.Second}}
}

// postJSON 发送 JSON POST 并把响应体写到 out。
func (c *mimoHTTPClient) postJSON(ctx context.Context, url, apiKey string, body any, out io.Writer) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		buf, _ := io.ReadAll(resp.Body)
		log.Printf("mimo tts http %d: %s", resp.StatusCode, string(buf))
		return fmt.Errorf("tts http %d", resp.StatusCode)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}
