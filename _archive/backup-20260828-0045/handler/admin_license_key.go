package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

type adminModifyBatchFaceValueRequest struct {
	BatchName string `json:"batchName"`
	CostCents int `json:"costCents"`
}

func AdminImportLicenseKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	// 32MB 限制
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Printf("admin import license keys parse multipart failed: %v", err)
		Fail(w, "表单解析失败")
		return
	}
	batchName := strings.TrimSpace(r.FormValue("batchName"))
	faceValueCentsStr := strings.TrimSpace(r.FormValue("faceValueCents"))
	faceValueCents, cerr := strconv.Atoi(faceValueCentsStr)
	if cerr != nil || faceValueCents <= 0 {
		Fail(w, "面额必须是 > 0 的整数")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("admin import license keys form file failed: %v", err)
		Fail(w, "上传文件读取失败")
		return
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		log.Printf("admin import license keys read file failed: %v", err)
		Fail(w, "文件读取失败")
		return
	}
	total, imported, dup, malformed, samples, err := service.ImportLicenseKeys(user.ID, batchName, faceValueCents, content)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{
		"totalLines":       total,
		"importedCount":    imported,
		"duplicateCount":   dup,
		"malformedCount":   malformed,
		"malformedSamples": samples,
	})
}

func AdminListLicenseKeys(w http.ResponseWriter, r *http.Request) {
	q := parseQuery(r)
	qry := r.URL.Query()
	status := qry.Get("status")
	batchName := qry.Get("batchName")
	keyword := qry.Get("keyword")
	items, total, err := service.AdminListLicenseKeys(q, status, batchName, keyword)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, model.LicenseKeyList{Items: items, Total: total})
}

func AdminListRedeemLogs(w http.ResponseWriter, r *http.Request) {
	q := parseQuery(r)
	userKeyword := r.URL.Query().Get("userKeyword")
	items, total, err := service.AdminListRedeemLogs(q, userKeyword)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, model.LicenseRedeemLogList{Items: items, Total: total})
}

func AdminModifyBatchFaceValue(w http.ResponseWriter, r *http.Request) {
	var req adminModifyBatchFaceValueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("admin modify batch face value decode failed: %v", err)
		Fail(w, "参数解析失败")
		return
	}
	if strings.TrimSpace(req.BatchName) == "" {
		Fail(w, "批次名称不能为空")
		return
	}
	if req.CostCents <= 0 {
		Fail(w, "面额必须是 > 0 的整数")
		return
	}
	rows, err := service.AdminModifyBatchUnusedFaceValueCents(req.BatchName, req.CostCents)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{"rowsAffected": rows})
}

type adminGenerateLicenseKeysRequest struct {
	BatchName      string `json:"batchName"`
	FaceValueCents int    `json:"faceValueCents"`
	Count          int    `json:"count"`
}

// AdminGenerateLicenseKeys 管理员自动生成卡密：系统 mint 唯一随机 key，入库 + 落盘 TXT。
func AdminGenerateLicenseKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	var req adminGenerateLicenseKeysRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("admin generate license keys decode failed: %v", err)
		Fail(w, "参数解析失败")
		return
	}
	generated, filePath, err := service.GenerateLicenseKeys(
		user.ID, strings.TrimSpace(req.BatchName), req.FaceValueCents, req.Count,
	)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]any{
		"generatedCount": len(generated),
		"batchName":      strings.TrimSpace(req.BatchName),
		"filePath":       filePath,
	})
}

// AdminExportLicenseKeys 管理员导出某批次卡密为 TXT 附件（优先落盘文件，缺失则从 DB 拼装）。
func AdminExportLicenseKeys(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok || user.ID == "" {
		Fail(w, "请先登录")
		return
	}
	batchName := strings.TrimSpace(r.URL.Query().Get("batchName"))
	if batchName == "" {
		Fail(w, "批次名必填")
		return
	}
	content, err := service.ExportLicenseKeysBatch(batchName)
	if err != nil {
		FailError(w, err)
		return
	}
	safe := service.SanitizeBatchNameForFileExport(batchName)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.txt\"", safe))
	w.Write([]byte(content))
}
