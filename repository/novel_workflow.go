package repository

import (
	"errors"
	"time"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

// === novel-workflow v2：CRUD ===

// dbh 取 DB 连接 + 错误返回；用于本文件所有函数的统一入口。
func dbh() (*gorm.DB, error) {
	return DB()
}

// CreateNovelWorkflowRun 插入 run。
func CreateNovelWorkflowRun(run *model.NovelWorkflowRun) error {
	db, err := dbh()
	if err != nil {
		return err
	}
	return db.Create(run).Error
}

// GetNovelWorkflowRun 取 run（by id）。
func GetNovelWorkflowRun(id string) (*model.NovelWorkflowRun, error) {
	db, err := dbh()
	if err != nil {
		return nil, err
	}
	var run model.NovelWorkflowRun
	if err := db.First(&run, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

// UpdateNovelWorkflowRun 写回 run。
func UpdateNovelWorkflowRun(run *model.NovelWorkflowRun) error {
	db, err := dbh()
	if err != nil {
		return err
	}
	return db.Save(run).Error
}

// ListNovelWorkflowRunsByProject 列项目的所有 run。
func ListNovelWorkflowRunsByProject(projectID string, limit, offset int) ([]model.NovelWorkflowRun, int64, error) {
	db, err := dbh()
	if err != nil {
		return nil, 0, err
	}
	var runs []model.NovelWorkflowRun
	var total int64
	q := db.Model(&model.NovelWorkflowRun{}).Where("project_id = ?", projectID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at DESC").Limit(limit).Offset(offset).Find(&runs).Error; err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}

// ListActiveNovelWorkflowRuns 列所有"未完成"的 run（worker 周期用）。
// "未完成" = overall_status = 进行中 OR 未启动。
func ListActiveNovelWorkflowRuns() ([]model.NovelWorkflowRun, error) {
	db, err := dbh()
	if err != nil {
		return nil, err
	}
	var runs []model.NovelWorkflowRun
	if err := db.Where("overall_status = ? OR overall_status = ?", "进行中", "未启动").
		Order("updated_at ASC").
		Find(&runs).Error; err != nil {
		return nil, err
	}
	return runs, nil
}

// CreateNovelWorkflowNode 插入 node。
func CreateNovelWorkflowNode(node *model.NovelWorkflowNode) error {
	db, err := dbh()
	if err != nil {
		return err
	}
	return db.Create(node).Error
}

// GetNovelWorkflowNodeByID 取 node（by id）。
func GetNovelWorkflowNodeByID(id string) (*model.NovelWorkflowNode, error) {
	db, err := dbh()
	if err != nil {
		return nil, err
	}
	var node model.NovelWorkflowNode
	if err := db.First(&node, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// GetNovelWorkflowNodeByRunAndNodeID 取 node（by runId + 节点定义 ID）。
func GetNovelWorkflowNodeByRunAndNodeID(runID, nodeID string) (*model.NovelWorkflowNode, error) {
	db, err := dbh()
	if err != nil {
		return nil, err
	}
	var node model.NovelWorkflowNode
	if err := db.First(&node, "run_id = ? AND node_id = ?", runID, nodeID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// UpdateNovelWorkflowNode 写回 node。
func UpdateNovelWorkflowNode(node *model.NovelWorkflowNode) error {
	db, err := dbh()
	if err != nil {
		return err
	}
	return db.Save(node).Error
}

// ListNovelWorkflowNodesByRun 列 run 的所有节点。
func ListNovelWorkflowNodesByRun(runID string) ([]model.NovelWorkflowNode, error) {
	db, err := dbh()
	if err != nil {
		return nil, err
	}
	var nodes []model.NovelWorkflowNode
	if err := db.Where("run_id = ?", runID).Order("created_at ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	return nodes, nil
}

// UpdateNovelWorkflowNodeStatus 仅写 status / progress / step_message / error。
// 用于 worker 高频更新（避免 Save 覆盖无关字段）。
func UpdateNovelWorkflowNodeStatus(id, status string, progress int, stepMessage, errorMsg string) error {
	db, err := dbh()
	if err != nil {
		return err
	}
	updates := map[string]any{
		"status":       status,
		"progress":     progress,
		"step_message": stepMessage,
		"error":        errorMsg,
		"updated_at":   time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	return db.Model(&model.NovelWorkflowNode{}).Where("id = ?", id).Updates(updates).Error
}
