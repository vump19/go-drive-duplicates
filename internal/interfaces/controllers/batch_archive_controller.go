package controllers

import (
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
)

// BatchArchiveController handles batch archive HTTP requests
type BatchArchiveController struct {
	batchArchiveUseCase *usecases.BatchArchiveUseCase
}

// NewBatchArchiveController creates a new batch archive controller
func NewBatchArchiveController(batchArchiveUseCase *usecases.BatchArchiveUseCase) *BatchArchiveController {
	return &BatchArchiveController{
		batchArchiveUseCase: batchArchiveUseCase,
	}
}

// BatchArchiveRequest represents the request to start batch archive
type BatchArchiveRequest struct {
	Folders []usecases.FolderInput        `json:"folders"`
	Options *usecases.BatchArchiveOptions `json:"options,omitempty"`
}

// StartBatchArchive handles the request to start a batch archive job
func (c *BatchArchiveController) StartBatchArchive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchArchiveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if len(req.Folders) == 0 {
		http.Error(w, "아카이브할 폴더 목록이 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("📦 일괄 아카이브 요청: %d개 폴더", len(req.Folders))
	for i, f := range req.Folders {
		log.Printf("  📁 폴더 %d: URL=%q, FolderID=%q", i+1, f.URL, f.FolderID)
	}

	job, err := c.batchArchiveUseCase.StartBatchArchive(r.Context(), req.Folders, req.Options)
	if err != nil {
		log.Printf("❌ 일괄 아카이브 시작 실패: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobId":        job.ID,
		"totalFolders": job.TotalFolders,
		"message":      "아카이브 작업이 시작되었습니다",
	})
}

// GetProgress handles the request to get job progress
func (c *BatchArchiveController) GetProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	job, exists := c.batchArchiveUseCase.GetJobProgress(jobID)
	if !exists {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// CancelJob handles the request to cancel a job
func (c *BatchArchiveController) CancelJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		// Try to get from body
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

	err := c.batchArchiveUseCase.CancelJob(jobID)
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

// ListSubfolders handles the request to list subfolders of a parent folder
func (c *BatchArchiveController) ListSubfolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ParentFolderURL string `json:"parentFolderUrl"`
		ParentFolderID  string `json:"parentFolderId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if req.ParentFolderURL == "" && req.ParentFolderID == "" {
		http.Error(w, "부모 폴더 URL 또는 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("📂 하위 폴더 목록 요청: URL=%q, ID=%q", req.ParentFolderURL, req.ParentFolderID)

	parentID, parentName, subfolders, err := c.batchArchiveUseCase.ListSubfolders(r.Context(), req.ParentFolderURL, req.ParentFolderID)
	if err != nil {
		log.Printf("❌ 하위 폴더 목록 조회 실패: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"parentFolderId":   parentID,
		"parentFolderName": parentName,
		"subfolders":       subfolders,
		"count":            len(subfolders),
	})
}

// GetAllJobs handles the request to get all active jobs
func (c *BatchArchiveController) GetAllJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobs := c.batchArchiveUseCase.GetAllJobs()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobs":  jobs,
		"count": len(jobs),
	})
}
