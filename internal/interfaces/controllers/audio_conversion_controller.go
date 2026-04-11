package controllers

import (
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
)

// AudioConversionController handles audio conversion HTTP requests
type AudioConversionController struct {
	audioConversionUseCase *usecases.AudioConversionUseCase
}

// NewAudioConversionController creates a new audio conversion controller
func NewAudioConversionController(audioConversionUseCase *usecases.AudioConversionUseCase) *AudioConversionController {
	return &AudioConversionController{
		audioConversionUseCase: audioConversionUseCase,
	}
}

// AudioConversionRequest represents the request to start audio conversion
type AudioConversionRequest struct {
	FolderURL string                          `json:"folderUrl"`
	FolderID  string                          `json:"folderId"`
	Options   *usecases.AudioConversionOptions `json:"options"`
}

// StartConversion handles the request to start an audio conversion job
func (c *AudioConversionController) StartConversion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AudioConversionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "잘못된 요청 형식입니다", http.StatusBadRequest)
		return
	}

	if req.FolderURL == "" && req.FolderID == "" {
		http.Error(w, "폴더 URL 또는 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	if req.Options == nil || req.Options.OutputFormat == "" {
		http.Error(w, "출력 형식이 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("🎵 오디오 변환 요청: 폴더=%q, 형식=%s", req.FolderURL+req.FolderID, req.Options.OutputFormat)

	job, err := c.audioConversionUseCase.StartConversion(r.Context(), req.FolderURL, req.FolderID, req.Options)
	if err != nil {
		log.Printf("❌ 오디오 변환 시작 실패: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobId":   job.ID,
		"message": "오디오 변환 작업이 시작되었습니다",
	})
}

// GetProgress handles the request to get job progress
func (c *AudioConversionController) GetProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		http.Error(w, "jobId가 필요합니다", http.StatusBadRequest)
		return
	}

	job, exists := c.audioConversionUseCase.GetJobProgress(jobID)
	if !exists {
		http.Error(w, "작업을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(job)
}

// CancelJob handles the request to cancel a job
func (c *AudioConversionController) CancelJob(w http.ResponseWriter, r *http.Request) {
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

	err := c.audioConversionUseCase.CancelJob(jobID)
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

// GetFormats handles the request to get supported audio formats
func (c *AudioConversionController) GetFormats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	formats := c.audioConversionUseCase.GetSupportedFormats()
	sourceFormats := c.audioConversionUseCase.GetSupportedSourceFormats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"formats":       formats,
		"sourceFormats": sourceFormats,
	})
}
