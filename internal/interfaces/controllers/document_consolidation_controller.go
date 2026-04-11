package controllers

import (
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"strings"
)

// DocumentConsolidationController handles document consolidation HTTP requests
type DocumentConsolidationController struct {
	useCase *usecases.DocumentConsolidationUseCase
}

// NewDocumentConsolidationController creates a new document consolidation controller
func NewDocumentConsolidationController(useCase *usecases.DocumentConsolidationUseCase) *DocumentConsolidationController {
	return &DocumentConsolidationController{
		useCase: useCase,
	}
}

// StartConsolidationRequest represents the request body
type StartConsolidationRequest struct {
	FolderID          string   `json:"folderId"`
	FolderURL         string   `json:"folderUrl"`
	IncludeSubfolders bool     `json:"includeSubfolders"`
	MaxFileSizeMB     int      `json:"maxFileSizeMB"`
	FileExtensions    []string `json:"fileExtensions"`
}

// StartConsolidation handles the request to start a document consolidation job
func (c *DocumentConsolidationController) StartConsolidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartConsolidationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	// URL에서 폴더 ID 추출
	folderID := req.FolderID
	if folderID == "" && req.FolderURL != "" {
		id, err := extractConsolidateFolderID(req.FolderURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		folderID = id
	}

	if folderID == "" {
		http.Error(w, "폴더 ID 또는 URL이 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("📄 문서 통합 요청: 폴더=%s, 하위폴더=%v, 최대크기=%dMB",
		folderID, req.IncludeSubfolders, req.MaxFileSizeMB)

	ucReq := &usecases.ConsolidationRequest{
		FolderID:          folderID,
		IncludeSubfolders: req.IncludeSubfolders,
		MaxFileSizeMB:     req.MaxFileSizeMB,
		FileExtensions:    req.FileExtensions,
	}

	job, err := c.useCase.StartConsolidation(r.Context(), ucReq)
	if err != nil {
		log.Printf("❌ 문서 통합 시작 실패: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobId":   job.ID,
		"message": "문서 통합 작업이 시작되었습니다",
	})
}

// GetProgress handles the request to get job progress
func (c *DocumentConsolidationController) GetProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	job, exists := c.useCase.GetJobProgress(jobID)
	if !exists {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// CancelConsolidation handles the request to cancel a job
func (c *DocumentConsolidationController) CancelConsolidation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		var req struct {
			JobID string `json:"jobId"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		jobID = req.JobID
	}

	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	err := c.useCase.CancelJob(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "작업 취소가 요청되었습니다",
	})
}

// extractConsolidateFolderID extracts folder ID from a Google Drive URL
func extractConsolidateFolderID(url string) (string, error) {
	// "folders/" 뒤의 ID 추출
	idx := strings.Index(url, "folders/")
	if idx >= 0 {
		idPart := url[idx+8:]
		// ? / # 등의 구분자에서 잘라내기
		for i, ch := range idPart {
			if ch == '?' || ch == '/' || ch == '#' {
				idPart = idPart[:i]
				break
			}
		}
		if len(idPart) > 0 {
			return idPart, nil
		}
	}

	// URL이 아니라 순수 폴더 ID일 수 있음
	if !strings.Contains(url, "/") && len(url) > 10 {
		return url, nil
	}

	return "", fmt.Errorf("유효하지 않은 Google Drive URL입니다: %s", url)
}
