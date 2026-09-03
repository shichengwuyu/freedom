package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: shot-dubbing-node 调度层 ===
//
// 入口：
//   - DispatchForShot: 单条配音（用户点"重新生成"或"一键启动配音节点"）
//   - DispatchForProject: 项目级遍历（novel-workflow 节点派发时调）
//
// 计费：
//   - TTS 调用前：ConsumeUserBalanceWithHold（cents=成本）
//   - TTS 成功：SettleBalanceHold
//   - TTS 失败：CancelBalanceHold（自动退款）
//
// 失败语义：
//   - 重试 2 次后仍失败 → 该 shot 配音字段保持空 / 状态=failure
//   - 节点整体不算失败（其他 shot 可能成功）—— 节点状态由 service/novel_workflow.go::OnNovelWorkflowNodeFinished 决定

// shotDubbingMaxRetries TTS 调用重试上限（不算首次）。
const shotDubbingMaxRetries = 2

// shotDubbingCostCents 默认 TTS 单次成本（v2 简化：固定 6 分钱 = 0.06 元 / 次）。
// 后续按 model 名差异化定价（system_settings.tts_pricing）。
const shotDubbingCostCents = 6

// DispatchForShot 调度单条 shot 的 TTS 配音。
//   - 创建/更新 ShotDubbing 记录
//   - 扣费（ConsumeUserBalanceWithHold）
//   - 调 TTS provider
//   - 成功 → 落 audio url + 状态=success + settle hold
//   - 失败 → 重试 2 次；都失败 → 状态=failure + cancel hold（自动退款）
//   - 写 balance log（consume + 可能的 refund）
func DispatchForShot(ctx context.Context, userID, projectID, shotID, text, voiceID string, speed float64) error {
	if userID == "" || projectID == "" || shotID == "" {
		return errors.New("userID/projectID/shotID required")
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	requestID := "tts:" + projectID + ":" + shotID
	ttsModel := "mimo"
	if p := GetTTSProvider(); p != nil {
		ttsModel = p.Name()
	}

	// 1) 扣费（hold 模式）
	holdID, err := ConsumeUserBalanceWithHold(userID, "tts:"+ttsModel, shotDubbingCostCents, "/audio/speech", requestID)
	if err != nil {
		// 余额不足等场景：仍创建/更新 ShotDubbing 记录，标记为 failure
		_ = repository.UpsertShotDubbing(&model.ShotDubbing{
			UserID: userID, ProjectID: projectID, ShotID: shotID,
			Text: text, VoiceID: voiceID, Speed: speed, TtsModel: ttsModel,
			Status: "failure", Error: "扣费失败: " + err.Error(),
			UpdatedAt: nowStr, CompletedAt: nowStr,
		})
		return err
	}

	// 2) 调 TTS（带重试）
	var lastErr error
	for attempt := 0; attempt <= shotDubbingMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second) // 1s / 2s backoff
		}
		provider := GetTTSProvider()
		if provider == nil {
			lastErr = errors.New("no TTS provider registered")
			continue
		}
		result, err := provider.Synthesize(ctx, text, TTSOpts{
			VoiceID: voiceID, Speed: speed, Format: "mp3",
		})
		if err == nil && result != nil {
			// 3a) 成功
			_ = repository.UpsertShotDubbing(&model.ShotDubbing{
				UserID: userID, ProjectID: projectID, ShotID: shotID,
				Text: text, VoiceID: voiceID, Speed: speed, TtsModel: ttsModel,
				AudioURL: result.AudioURL, DurationMs: result.DurationMs, Bytes: result.Bytes, MimeType: result.MimeType,
				Status: "success",
				UpdatedAt: nowStr, CompletedAt: nowStr,
				BalanceLogID: holdID, CostCents: shotDubbingCostCents,
			})
			if holdID != "" {
				if e := SettleBalanceHold(holdID); e != nil {
					log.Printf("novel-dubbing: settle hold=%s err=%v", holdID, e)
				}
			}
			return nil
		}
		lastErr = err
		log.Printf("novel-dubbing: attempt=%d TTS err=%v", attempt, err)
	}

	// 3b) 全部重试失败 → cancel hold（自动退款）+ 标 failure
	if holdID != "" {
		if e := CancelBalanceHold(holdID); e != nil {
			log.Printf("novel-dubbing: cancel hold=%s err=%v", holdID, e)
		}
	}
	_ = repository.UpsertShotDubbing(&model.ShotDubbing{
		UserID: userID, ProjectID: projectID, ShotID: shotID,
		Text: text, VoiceID: voiceID, Speed: speed, TtsModel: ttsModel,
		Status: "failure", Error: lastErrString(lastErr),
		UpdatedAt: nowStr, CompletedAt: nowStr,
		CostCents: shotDubbingCostCents,
	})
	return lastErr
}

// DispatchForProject 遍历项目所有 shot，调度配音。
//   - shots：[{shotId, text}]，text 为空时该 shot 跳过
//   - 调 DispatchForShot per shot
//   - 不阻塞：每条 shot 失败不影响其他 shot
func DispatchForProject(ctx context.Context, userID, projectID string, shots []ShotForDubbing, voiceID string, speed float64) error {
	if len(shots) == 0 {
		return nil
	}
	for _, s := range shots {
		if s.Text == "" {
			// 跳过；按 spec 该 shot 配音字段保持空，节点状态="跳过"
			continue
		}
		if err := DispatchForShot(ctx, userID, projectID, s.ShotID, s.Text, voiceID, speed); err != nil {
			log.Printf("novel-dubbing: shot=%s err=%v", s.ShotID, err)
		}
	}
	return nil
}

// ShotForDubbing 调度入参。
type ShotForDubbing struct {
	ShotID string
	Text   string
}

// ListForProject 列项目所有配音记录。
func ListForProject(projectID string) ([]model.ShotDubbing, error) {
	return repository.ListShotDubbingsByProject(projectID)
}

// lastErrString 安全的 error -> string。
func lastErrString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
