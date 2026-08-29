package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: novel-rerun-layer (核心 UX) ===
//
// 入口：
//   - RerunShotLayer(userID, runID, projectID, shotID, layer, params)
//   - RerunFullLayer(userID, runID, projectID, layer, params)  -- "重烧字幕" / "重新合成"
//   - RollbackToVersion(userID, recordID)
//   - ListVersions(userID, projectID, scope, layer, shotID)  -- 列版本历史
//
// v2 简化：
//   - 重做走 service/novel_dubbing.go::DispatchForShot / service/novel_composition.go::ComposeFull 等
//   - 不走通用 task worker（v3 改）
//   - 不做"回滚到旧版本"的真正切换（只标 RerunRecord.version, 实际数据靠 ShotDubbing 字段覆盖）

// RerunShotLayerParams 单分镜重做入参。
type RerunShotLayerParams struct {
	ShotID  string                 `json:"shotId"`
	Layer   string                 `json:"layer"` // "video" | "dubbing" | "subtitle"
	Text    string                  `json:"text,omitempty"`
	VoiceID string                  `json:"voiceId,omitempty"`
	Speed   float64                 `json:"speed,omitempty"`
	Lines   []model.SubtitleLine    `json:"lines,omitempty"`
}

// RerunShotLayer 重做某分镜的某层。
//
//   - layer="dubbing"：调 DispatchForShot（扣费 + TTS + 落库）
//   - layer="subtitle"：写新 Lines（不调任何模型）
//   - layer="video"：v2 不接真实视频生成，仅写重做记录 + 标 running
//
// 写一条 RerunRecord（version 自增）。
func RerunShotLayer(ctx context.Context, userID, runID, projectID string, params RerunShotLayerParams) (*model.RerunRecord, error) {
	if userID == "" || projectID == "" {
		return nil, errors.New("userID/projectID required")
	}
	if params.ShotID == "" {
		return nil, errors.New("shotId required")
	}
	if params.Layer != "video" && params.Layer != "dubbing" && params.Layer != "subtitle" {
		return nil, errors.New("layer 必须是 video/dubbing/subtitle")
	}

	version, err := repository.NextRerunRecordVersion(userID, projectID, "shot", params.Layer, params.ShotID)
	if err != nil {
		return nil, err
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	payloadJSON, _ := json.Marshal(params)

	rec := &model.RerunRecord{
		ID:          newID("rerun"),
		UserID:      userID,
		ProjectID:   projectID,
		RunID:       runID,
		Scope:       "shot",
		Layer:       params.Layer,
		ShotID:      params.ShotID,
		Version:     version,
		PayloadJSON: string(payloadJSON),
		Status:      "running",
		CreatedAt:   nowStr,
		UpdatedAt:   nowStr,
	}
	if err := repository.CreateRerunRecord(rec); err != nil {
		return nil, err
	}

	switch params.Layer {
	case "dubbing":
		if err := DispatchForShot(ctx, userID, projectID, params.ShotID, params.Text, params.VoiceID, params.Speed); err != nil {
			rec.Status = "failure"
			rec.Error = err.Error()
			rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			_ = repository.UpdateRerunRecord(rec)
			return rec, err
		}
		rec.Status = "success"
	case "subtitle":
		if err := UpdateLines(userID, projectID, params.ShotID, params.Lines); err != nil {
			rec.Status = "failure"
			rec.Error = err.Error()
			rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
			_ = repository.UpdateRerunRecord(rec)
			return rec, err
		}
		rec.Status = "success"
	case "video":
		rec.Status = "running"
		rec.Error = "video 重做 v2 暂未实现, 留 v3"
	}
	rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	rec.UpdatedAt = rec.CompletedAt
	_ = repository.UpdateRerunRecord(rec)
	return rec, nil
}

// RerunFullLayerParams 整部重做入参。
type RerunFullLayerParams struct {
	Layer            string             `json:"layer"`
	SubtitleStyle    SubtitleStyleJSON  `json:"subtitleStyle,omitempty"`
	CompositionInput CompositionInput   `json:"compositionInput,omitempty"`
}

// RerunFullLayer 整部成片重做某层。
//
//   - layer="subtitle"：仅重烧字幕（ComposeSubtitleOnly）
//   - layer="composition"：重跑 5 步合成
//   - layer="full"：与 composition 等价
func RerunFullLayer(ctx context.Context, userID, runID, projectID string, params RerunFullLayerParams) (*model.RerunRecord, *model.CompositionTask, error) {
	if userID == "" || projectID == "" {
		return nil, nil, errors.New("userID/projectID required")
	}
	if params.Layer != "subtitle" && params.Layer != "composition" && params.Layer != "full" {
		return nil, nil, errors.New("layer 必须是 subtitle/composition/full")
	}

	version, err := repository.NextRerunRecordVersion(userID, projectID, "full", params.Layer, "")
	if err != nil {
		return nil, nil, err
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	payloadJSON, _ := json.Marshal(params)

	rec := &model.RerunRecord{
		ID:          newID("rerun"),
		UserID:      userID,
		ProjectID:   projectID,
		RunID:       runID,
		Scope:       "full",
		Layer:       params.Layer,
		Version:     version,
		PayloadJSON: string(payloadJSON),
		Status:      "running",
		CreatedAt:   nowStr,
		UpdatedAt:   nowStr,
	}
	if err := repository.CreateRerunRecord(rec); err != nil {
		return nil, nil, err
	}

	taskInput := params.CompositionInput
	if len(taskInput.ShotVideos) == 0 {
		rec.Status = "failure"
		rec.Error = "compositionInput.shotVideos 不能为空"
		rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		_ = repository.UpdateRerunRecord(rec)
		return rec, nil, errors.New(rec.Error)
	}
	task, err := CreateCompositionTask(userID, projectID, runID, "", taskInput)
	if err != nil {
		rec.Status = "failure"
		rec.Error = err.Error()
		rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		_ = repository.UpdateRerunRecord(rec)
		return rec, nil, err
	}
	rec.GenericTaskID = task.ID

	if err := RunCompositionTask(ctx, task); err != nil {
		rec.Status = "failure"
		rec.Error = fmt.Sprintf("composition failed: %v", err)
		rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
		_ = repository.UpdateRerunRecord(rec)
		return rec, task, err
	}

	task, _ = repository.GetCompositionTask(task.ID, userID)
	if task != nil {
		rec.OutputURL = task.OutputURL
	}
	rec.Status = "success"
	rec.CompletedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	rec.UpdatedAt = rec.CompletedAt
	_ = repository.UpdateRerunRecord(rec)
	return rec, task, nil
}

// RollbackToVersion 标某版本为"采用此版本"（v2 简化：只改 RerunRecord 标记 + 注释）。
func RollbackToVersion(userID, recordID string) error {
	rec, err := repository.GetRerunRecord(recordID, userID)
	if err != nil {
		return err
	}
	if rec == nil {
		return errors.New("重做记录不存在")
	}
	if rec.Status != "success" {
		return errors.New("只能回滚到成功的版本")
	}
	rec.Error = fmt.Sprintf("[rolled back to v%d at %s]", rec.Version, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"))
	rec.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return repository.UpdateRerunRecord(rec)
}

// ListVersions 列某 scope+layer+shot 的所有版本历史。
func ListVersions(userID, projectID, scope, layer, shotID string) ([]model.RerunRecord, error) {
	return repository.ListRerunRecordsByScope(userID, projectID, scope, layer, shotID)
}

// GetLatestRerunRecord 取最新一条。
func GetLatestRerunRecord(userID, projectID, scope, layer, shotID string) (*model.RerunRecord, error) {
	return repository.LatestRerunRecordByScope(userID, projectID, scope, layer, shotID)
}
