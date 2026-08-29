package repository

import "github.com/tigerowo/freedom/model"

// SaveCanvasAudioTask 新建任务（PR-8：主键已服务端生成；where 仍带 user_id 作为防御性兜底，
// 防止调用方误用客户端字段当 id 时跨用户覆盖）。Update 路径请用 UpdateCanvasAudioTask。
func SaveCanvasAudioTask(task model.CanvasAudioTask) (model.CanvasAudioTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	return task, db.Where("user_id = ?", task.UserID).Save(&task).Error
}

func UpdateCanvasAudioTask(task model.CanvasAudioTask) (model.CanvasAudioTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}

	return task, db.Model(&model.CanvasAudioTask{}).
		Where("user_id = ? AND id = ?", task.UserID, task.ID).
		Select("*").
		Updates(&task).Error
}

func GetUserCanvasAudioTask(userID string, id string) (model.CanvasAudioTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.CanvasAudioTask{}, false, err
	}
	var task model.CanvasAudioTask
	err = db.First(&task, "user_id = ? AND id = ?", userID, id).Error
	if err != nil {
		return model.CanvasAudioTask{}, false, nil
	}
	return task, true, nil
}
