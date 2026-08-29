package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/tigerowo/freedom/model"
	"github.com/tigerowo/freedom/service"
)

type adminSyncRequest struct {
	Category string `json:"category"`
}

type adminBatchDeleteRequest struct {
	IDs []string `json:"ids"`
}

func AdminPromptCategories(w http.ResponseWriter, r *http.Request) {
	OK(w, service.ListPromptCategories())
}

func AdminSyncAllPromptCategories(w http.ResponseWriter, r *http.Request) {
	service.SyncRemotePromptCategories()
	OK(w, service.ListPromptCategories())
}

func AdminPrompts(w http.ResponseWriter, r *http.Request) {
	result, err := service.ListPrompts(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminSavePrompt(w http.ResponseWriter, r *http.Request) {
	var item model.Prompt
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	result, err := service.SavePrompt(item)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDeletePrompt(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeletePrompt(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func AdminDeletePrompts(w http.ResponseWriter, r *http.Request) {
	var request adminBatchDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	if err := service.DeletePrompts(request.IDs); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func AdminSyncPromptCategories(w http.ResponseWriter, r *http.Request) {
	var request adminSyncRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		Fail(w, "请求参数格式错误")
		return
	}
	log.Printf("sync prompt category start category=%s", request.Category)
	categories, err := service.SyncPromptCategory(request.Category)
	if err != nil {
		log.Printf("sync prompt category failed category=%s err=%v", request.Category, err)
		FailError(w, err)
		return
	}
	log.Printf("sync prompt category done category=%s", request.Category)
	OK(w, categories)
}
