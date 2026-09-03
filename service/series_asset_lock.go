package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/repository"
)

// === novel-workflow v2: series-asset-lock 服务层 ===

// SeriesAssetLockParams 锁的参数。
type SeriesAssetLockParams struct {
	CharacterIDs     []string `json:"characterIds"`
	SceneIDs         []string `json:"sceneIds"`
	PropIDs          []string `json:"propIds"`
	GlobalStylePrompt string   `json:"globalStylePrompt"`
}

// GetLock 取主资产包。
func GetLock(userID, projectID string) (*model.SeriesAssetLock, error) {
	return repository.GetSeriesAssetLockByProject(userID, projectID)
}

// UpdateLock 改主资产包。
func UpdateLock(userID, projectID string, params SeriesAssetLockParams) (*model.SeriesAssetLock, error) {
	if userID == "" || projectID == "" {
		return nil, errors.New("userID/projectID required")
	}
	existing, _ := repository.GetSeriesAssetLockByProject(userID, projectID)
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	charsJSON, _ := json.Marshal(params.CharacterIDs)
	sceneJSON, _ := json.Marshal(params.SceneIDs)
	propJSON, _ := json.Marshal(params.PropIDs)

	if existing == nil {
		s := &model.SeriesAssetLock{
			ID:                newID("lock"),
			UserID:            userID,
			ProjectID:         projectID,
			CharacterIDsJSON:  string(charsJSON),
			SceneIDsJSON:      string(sceneJSON),
			PropIDsJSON:       string(propJSON),
			GlobalStylePrompt: params.GlobalStylePrompt,
			IsLocked:          false,
			CreatedAt:         nowStr,
			UpdatedAt:         nowStr,
		}
		if err := repository.UpsertSeriesAssetLock(s); err != nil {
			return nil, err
		}
		return s, nil
	}
	existing.CharacterIDsJSON = string(charsJSON)
	existing.SceneIDsJSON = string(sceneJSON)
	existing.PropIDsJSON = string(propJSON)
	existing.GlobalStylePrompt = params.GlobalStylePrompt
	existing.UpdatedAt = nowStr
	if err := repository.UpsertSeriesAssetLock(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Lock 锁定。
func Lock(userID, projectID string) (*model.SeriesAssetLock, error) {
	existing, err := repository.GetSeriesAssetLockByProject(userID, projectID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("主资产包未配置, 请先调用 UpdateLock")
	}
	if existing.IsLocked {
		return existing, nil
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	existing.IsLocked = true
	existing.LockedAt = nowStr
	existing.UpdatedAt = nowStr
	if err := repository.UpsertSeriesAssetLock(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// Unlock 解锁。
func Unlock(userID, projectID string) (*model.SeriesAssetLock, error) {
	existing, err := repository.GetSeriesAssetLockByProject(userID, projectID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	nowStr := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	existing.IsLocked = false
	existing.UnlockedAt = nowStr
	existing.UpdatedAt = nowStr
	if err := repository.UpsertSeriesAssetLock(existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// IsLocked 便捷判断。
func IsLocked(userID, projectID string) (bool, error) {
	lock, err := repository.GetSeriesAssetLockByProject(userID, projectID)
	if err != nil || lock == nil {
		return false, err
	}
	return lock.IsLocked, nil
}

// ResolveStylePrompt 视频生成时, 锁定时拼上全局色调 prompt。
func ResolveStylePrompt(userID, projectID, shotPrompt string) (string, error) {
	lock, err := repository.GetSeriesAssetLockByProject(userID, projectID)
	if err != nil || lock == nil || !lock.IsLocked {
		return shotPrompt, err
	}
	if lock.GlobalStylePrompt == "" {
		return shotPrompt, nil
	}
	return shotPrompt + "\n\n[全局色调 / 风格约束]\n" + lock.GlobalStylePrompt, nil
}

// IsAssetInLock 判断 assetId 是否在主资产包内。
func IsAssetInLock(userID, projectID, assetID, assetType string) (bool, error) {
	lock, err := repository.GetSeriesAssetLockByProject(userID, projectID)
	if err != nil || lock == nil || !lock.IsLocked {
		return true, err
	}
	var ids []string
	switch assetType {
	case "character":
		_ = json.Unmarshal([]byte(lock.CharacterIDsJSON), &ids)
	case "scene":
		_ = json.Unmarshal([]byte(lock.SceneIDsJSON), &ids)
	case "prop":
		_ = json.Unmarshal([]byte(lock.PropIDsJSON), &ids)
	default:
		return true, nil
	}
	for _, id := range ids {
		if id == assetID {
			return true, nil
		}
	}
	return false, nil
}
