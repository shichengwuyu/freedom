package repository

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/tigerowo/freedom/model"
)

func newVendorApiSampleID() string {
	return "vs_" + uuid.NewString()[:16]
}

// CreateVendorApiSample 落库一条嗅探样本（填充主键与时间戳）
func CreateVendorApiSample(s model.VendorApiSample) (model.VendorApiSample, error) {
	db, err := ensureVendorsTableReady()
	if err != nil {
		return s, err
	}
	if s.ID == "" {
		s.ID = newVendorApiSampleID()
	}
	s.CreatedAt = time.Now()
	if e := db.Create(&s).Error; e != nil {
		return s, e
	}
	return s, nil
}

// ListVendorApiSamples 列出某用户的样本（可按供应商 + 是否仅生成类过滤），按时间倒序
func ListVendorApiSamples(userID, vendorType string, onlyGeneration bool, limit int) ([]model.VendorApiSample, error) {
	if userID == "" {
		return nil, errors.New("userID 不能为空")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return nil, err
	}
	q := db.Where("user_id = ?", userID)
	if vendorType != "" {
		q = q.Where("vendor_type = ?", vendorType)
	}
	if onlyGeneration {
		q = q.Where("is_likely_generation = ?", true)
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var samples []model.VendorApiSample
	if e := q.Order("created_at desc").Limit(limit).Find(&samples).Error; e != nil {
		return nil, e
	}
	return samples, nil
}

// CountVendorApiSamples 统计某用户某供应商的样本数（调试/前端展示用）
func CountVendorApiSamples(userID, vendorType string) (int64, error) {
	if userID == "" {
		return 0, errors.New("userID 不能为空")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return 0, err
	}
	q := db.Model(&model.VendorApiSample{}).Where("user_id = ?", userID)
	if vendorType != "" {
		q = q.Where("vendor_type = ?", vendorType)
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return 0, e
	}
	return n, nil
}

// DeleteVendorApiSamples 删除某用户某供应商的全部样本（clearSamples 用，vendorType 空则清空该用户全部）
func DeleteVendorApiSamples(userID, vendorType string) (int64, error) {
	if userID == "" {
		return 0, errors.New("userID 不能为空")
	}
	db, err := ensureVendorsTableReady()
	if err != nil {
		return 0, err
	}
	q := db.Where("user_id = ?", userID)
	if vendorType != "" {
		q = q.Where("vendor_type = ?", vendorType)
	}
	res := q.Delete(&model.VendorApiSample{})
	if res.Error != nil {
		return 0, res.Error
	}
	return res.RowsAffected, nil
}
