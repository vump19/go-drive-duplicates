package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"go-drive-duplicates/internal/usecases"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FileController handles HTTP requests related to file operations
type FileController struct {
	fileScanningUseCase *usecases.FileScanningUseCase
	fileRepo            repositories.FileRepository
	storageProvider     services.StorageProvider
	apiTimeout          time.Duration
}

// NewFileController creates a new file controller
func NewFileController(fileScanningUseCase *usecases.FileScanningUseCase, fileRepo repositories.FileRepository, storageProvider services.StorageProvider, apiTimeout time.Duration) *FileController {
	return &FileController{
		fileScanningUseCase: fileScanningUseCase,
		fileRepo:            fileRepo,
		storageProvider:     storageProvider,
		apiTimeout:          apiTimeout,
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
	// 기본값: 항상 재개 시도 (resume=false로 명시하지 않는 한)
	resumeStr := r.URL.Query().Get("resume")
	if resumeStr != "false" {
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if progress == nil {
		w.Write([]byte(`{"status":"idle"}`))
		return
	}
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

// GetStatistics handles the get file statistics endpoint
func (c *FileController) GetStatistics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout (설정된 apiTimeout 사용, 대용량 데이터 통계 조회 시 필요)
	ctx, cancel := context.WithTimeout(context.Background(), c.apiTimeout)
	defer cancel()

	// Get statistics
	stats, err := c.fileScanningUseCase.GetStatistics(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetDBFileList handles the GET /api/files/db/list endpoint
// Returns files from database for a given parent folder
func (c *FileController) GetDBFileList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get parentId from query parameter
	parentID := r.URL.Query().Get("parentId")
	if parentID == "" || parentID == "root" {
		// Find actual My Drive root ID from database
		actualRoot, err := c.findMyDriveRootID(context.Background())
		if err == nil && actualRoot != "" {
			parentID = actualRoot
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get files from database
	files, err := c.fileRepo.GetByParent(ctx, parentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"files":    files,
		"parentId": parentID,
		"count":    len(files),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetDBFolderTree handles the GET /api/files/db/tree endpoint
// Returns folder tree structure from database
func (c *FileController) GetDBFolderTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get parentId from query parameter
	parentID := r.URL.Query().Get("parentId")
	if parentID == "" || parentID == "root" {
		// Find actual My Drive root ID from database
		actualRoot, err := c.findMyDriveRootID(context.Background())
		if err == nil && actualRoot != "" {
			parentID = actualRoot
		}
	}

	// Get depth parameter (default 1)
	depth := 1
	if depthStr := r.URL.Query().Get("depth"); depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
			depth = d
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build folder tree recursively
	tree, err := c.buildFolderTree(ctx, parentID, depth, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(tree); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// buildFolderTree recursively builds folder tree structure
func (c *FileController) buildFolderTree(ctx context.Context, parentID string, maxDepth, currentDepth int) (interface{}, error) {
	// Get all items in this folder
	files, err := c.fileRepo.GetByParent(ctx, parentID)
	if err != nil {
		return nil, err
	}

	// Separate folders and files
	folders := []map[string]interface{}{}
	for _, file := range files {
		if file.MimeType == "application/vnd.google-apps.folder" {
			folderNode := map[string]interface{}{
				"id":       file.ID,
				"name":     file.Name,
				"mimeType": file.MimeType,
				"path":     file.Path,
			}

			// Recursively load children if not at max depth
			if currentDepth < maxDepth-1 {
				children, err := c.buildFolderTree(ctx, file.ID, maxDepth, currentDepth+1)
				if err == nil {
					folderNode["children"] = children
				}
			}

			folders = append(folders, folderNode)
		}
	}

	return map[string]interface{}{
		"parentId": parentID,
		"folders":  folders,
		"count":    len(folders),
	}, nil
}

// findMyDriveRootID finds the actual My Drive root ID from database
// It returns the most common parent ID, which is usually the My Drive root
func (c *FileController) findMyDriveRootID(ctx context.Context) (string, error) {
	// Get first file from database to extract parent ID
	files, err := c.fileRepo.GetPaginated(ctx, 0, 1)
	if err != nil {
		return "", err
	}

	if len(files) == 0 {
		return "", nil
	}

	// Get parents from first file
	file := files[0]
	if len(file.Parents) > 0 {
		return file.Parents[0], nil
	}

	return "", nil
}

// CheckFilesExist handles the POST /api/files/check-exists endpoint
// Checks if files exist in Google Drive and returns their existence status
func (c *FileController) CheckFilesExist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		FileIDs []string `json:"fileIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.FileIDs) == 0 {
		http.Error(w, "fileIds is required", http.StatusBadRequest)
		return
	}

	// Limit to 100 files at a time
	if len(req.FileIDs) > 100 {
		req.FileIDs = req.FileIDs[:100]
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check each file's existence
	results := make(map[string]bool)
	for _, fileID := range req.FileIDs {
		fileID = strings.TrimSpace(fileID)
		if fileID == "" {
			continue
		}

		exists, err := c.storageProvider.FileExists(ctx, fileID)
		if err != nil {
			// On error, assume file exists to be safe
			results[fileID] = true
		} else {
			results[fileID] = exists
		}
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"results": results,
		"checked": len(results),
	}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
