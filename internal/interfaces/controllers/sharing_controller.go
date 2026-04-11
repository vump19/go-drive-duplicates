package controllers

import (
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"regexp"
)

// SharingController handles sharing management HTTP requests
type SharingController struct {
	sharingManagerUseCase *usecases.SharingManagerUseCase
}

// NewSharingController creates a new sharing controller
func NewSharingController(sharingManagerUseCase *usecases.SharingManagerUseCase) *SharingController {
	return &SharingController{
		sharingManagerUseCase: sharingManagerUseCase,
	}
}

// extractFolderID extracts folder ID from URL or returns it directly
func extractSharingFolderID(input string) string {
	// Check if it looks like a URL
	pattern := regexp.MustCompile(`/folders/([a-zA-Z0-9_-]+)`)
	if matches := pattern.FindStringSubmatch(input); len(matches) > 1 {
		return matches[1]
	}
	return input
}

// StartSharingScan handles POST /api/sharing/scan/start
func (c *SharingController) StartSharingScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FolderID          string `json:"folderId"`
		IncludeSubfolders bool   `json:"includeSubfolders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	folderID := extractSharingFolderID(req.FolderID)
	if folderID == "" {
		http.Error(w, "폴더 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("🔍 공유 스캔 요청: folderID=%s, includeSubfolders=%v", folderID, req.IncludeSubfolders)

	job, err := c.sharingManagerUseCase.StartSharingScan(r.Context(), folderID, req.IncludeSubfolders)
	if err != nil {
		log.Printf("❌ 공유 스캔 시작 실패: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobId":      job.ID,
		"folderId":   job.FolderID,
		"folderName": job.FolderName,
		"message":    "공유 스캔이 시작되었습니다",
	})
}

// GetScanProgress handles GET /api/sharing/scan/progress?jobId=xxx
func (c *SharingController) GetScanProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	job, exists := c.sharingManagerUseCase.GetScanProgress(jobID)
	if !exists {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// GetScanResults handles GET /api/sharing/scan/results?jobId=xxx
func (c *SharingController) GetScanResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	job, exists := c.sharingManagerUseCase.GetScanResults(jobID)
	if !exists {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// CancelScan handles POST /api/sharing/scan/cancel?jobId=xxx
func (c *SharingController) CancelScan(w http.ResponseWriter, r *http.Request) {
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

	err := c.sharingManagerUseCase.CancelScan(jobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "스캔 취소가 요청되었습니다",
	})
}

// ChangePermissions handles POST /api/sharing/permissions/change
func (c *SharingController) ChangePermissions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req usecases.PermissionChangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	log.Printf("🔄 권한 변경 요청: %d개 파일, 액션=%s, 대상=%s", len(req.FileIDs), req.Action, req.TargetEmail)

	job, err := c.sharingManagerUseCase.StartPermissionChange(r.Context(), &req)
	if err != nil {
		log.Printf("❌ 권한 변경 시작 실패: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobId":      job.ID,
		"totalItems": job.TotalItems,
		"message":    "권한 변경이 시작되었습니다",
	})
}

// GetChangeProgress handles GET /api/sharing/permissions/progress?jobId=xxx
func (c *SharingController) GetChangeProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	job, exists := c.sharingManagerUseCase.GetChangeProgress(jobID)
	if !exists {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}
