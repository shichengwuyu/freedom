package repository

import (
	"errors"

	"github.com/tigerowo/freedom/model"
	"gorm.io/gorm"
)

func ListCreativeWorkflows(userID string) ([]model.CreativeWorkflow, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	// Limit 兜底：用户累计创建大量 workflow 时，避免一次返回上千条压垮前端 / JSON 序列化。
	// 与 model.Query.Normalize() 的 MaxPageSize 对齐（500）；如需分页后续再加 Page 参数。
	var workflows []model.CreativeWorkflow
	err = db.Where("scope = ? OR owner_user_id = ?", "public", userID).
		Order("updated_at DESC").
		Limit(model.MaxPageSize).
		Find(&workflows).Error
	return workflows, err
}

func GetCreativeWorkflow(id string) (model.CreativeWorkflow, bool, error) {
	db, err := DB()
	if err != nil {
		return model.CreativeWorkflow{}, false, err
	}
	var workflow model.CreativeWorkflow
	err = db.First(&workflow, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.CreativeWorkflow{}, false, nil
		}
		return model.CreativeWorkflow{}, false, err
	}
	return workflow, true, nil
}

func SaveCreativeWorkflow(workflow model.CreativeWorkflow) (model.CreativeWorkflow, error) {
	db, err := DB()
	if err != nil {
		return workflow, err
	}
	return workflow, db.Save(&workflow).Error
}

func DeleteCreativeWorkflow(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.CreativeWorkflow{}, "id = ?", id).Error
}
