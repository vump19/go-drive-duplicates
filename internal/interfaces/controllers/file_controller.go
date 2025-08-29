package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"net/http"
	"strconv"
	"time"
)

// FileController handles HTTP requests related to file operations
type FileController struct {
	fileScanningUseCase *usecases.FileScanningUseCase
}

// NewFileController creates a new file controller
func NewFileController(fileScanningUseCase *usecases.FileScanningUseCase) *FileController {
	return &FileController{
		fileScanningUseCase: fileScanningUseCase,
	}
}

// ScanAllFiles handles the scan all files endpoint
func (c *FileController) ScanAllFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request parameters
	var req usecases.ScanAllFilesRequest

	// Parse query parameters
	if resumeStr := r.URL.Query().Get("resume"); resumeStr == "true" {
		req.ResumeFromProgress = true
	}

	if batchSizeStr := r.URL.Query().Get("batchSize"); batchSizeStr != "" {
		if batchSize, err := strconv.Atoi(batchSizeStr); err == nil && batchSize > 0 {
			req.BatchSize = batchSize
		}
	}

	if workerCountStr := r.URL.Query().Get("workerCount"); workerCountStr != "" {
		if workerCount, err := strconv.Atoi(workerCountStr); err == nil && workerCount > 0 {
			req.WorkerCount = workerCount
		}
	}

	// Create context with timeout (5분으로 늘려서 Google Drive API 타임아웃과 맞춤)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileScanningUseCase.ScanAllFiles(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ScanFolder handles the scan folder endpoint
func (c *FileController) ScanFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.ScanFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FolderID == "" {
		http.Error(w, "FolderID is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout (5분으로 늘려서 Google Drive API 타임아웃과 맞춤)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileScanningUseCase.ScanFolder(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetScanProgress handles the get scan progress endpoint
func (c *FileController) GetScanProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get progress
	progress, err := c.fileScanningUseCase.GetScanProgress(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(progress); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ClearFailedProgress handles clearing failed progress records
func (c *FileController) ClearFailedProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Clear failed progress
	err := c.fileScanningUseCase.ClearFailedProgress(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"message": "Failed progress records cleared successfully",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
