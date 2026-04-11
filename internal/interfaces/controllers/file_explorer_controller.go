package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"strconv"
	"time"
)

// FileExplorerController handles HTTP requests for file explorer operations
// 파일 탐색기 작업을 위한 HTTP 요청 처리
type FileExplorerController struct {
	fileExplorerUseCase *usecases.FileExplorerUseCase
}

// NewFileExplorerController creates a new file explorer controller
func NewFileExplorerController(fileExplorerUseCase *usecases.FileExplorerUseCase) *FileExplorerController {
	return &FileExplorerController{
		fileExplorerUseCase: fileExplorerUseCase,
	}
}

// GetFolderTree handles the get folder tree endpoint
// GET /api/explorer/folder-tree?folderId={id}&depth={depth}
func (c *FileExplorerController) GetFolderTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		folderID = "root" // Default to root
	}

	depth := 3 // Default depth
	if depthStr := r.URL.Query().Get("depth"); depthStr != "" {
		if d, err := strconv.Atoi(depthStr); err == nil && d > 0 {
			depth = d
		}
	}

	// Create options
	options := entities.DefaultFolderTreeOptions()
	options.MaxDepth = depth

	// Create context with timeout (longer for large folder trees)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Get folder tree
	folderTree, err := c.fileExplorerUseCase.GetFolderTree(ctx, folderID, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(folderTree); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ListFolderContents handles the list folder contents endpoint
// GET /api/explorer/folder-contents?folderId={id}&sortBy={field}&order={asc|desc}&search={query}
func (c *FileExplorerController) ListFolderContents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		http.Error(w, "folderId is required", http.StatusBadRequest)
		return
	}

	// Parse list options
	options := entities.DefaultListOptions()
	if sortBy := r.URL.Query().Get("sortBy"); sortBy != "" {
		options.SortBy = sortBy
	}
	if sortOrder := r.URL.Query().Get("sortOrder"); sortOrder != "" {
		options.SortOrder = sortOrder
	}
	if order := r.URL.Query().Get("order"); order != "" {
		options.SortOrder = order
	}
	// Support both 'search' and 'searchQuery' parameter names
	if search := r.URL.Query().Get("searchQuery"); search != "" {
		options.SearchQuery = search
	}
	if search := r.URL.Query().Get("search"); search != "" {
		options.SearchQuery = search
	}
	if pageToken := r.URL.Query().Get("pageToken"); pageToken != "" {
		options.PageToken = pageToken
	}
	if pageSize := r.URL.Query().Get("pageSize"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 {
			options.PageSize = ps
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// List folder contents
	response, err := c.fileExplorerUseCase.ListFolderContents(ctx, folderID, options)
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

// CopyFiles handles the copy files endpoint
// POST /api/explorer/copy-files
func (c *FileExplorerController) CopyFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request entities.FileOperationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set operation type
	request.OperationType = entities.OperationTypeCopy

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Copy files
	response, err := c.fileExplorerUseCase.CopyFiles(ctx, &request)
	if err != nil {
		log.Printf("❌ 파일 복사 실패: %v", err)
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

// MoveFiles handles the move files endpoint
// POST /api/explorer/move-files
func (c *FileExplorerController) MoveFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request entities.FileOperationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set operation type
	request.OperationType = entities.OperationTypeMove

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Move files
	response, err := c.fileExplorerUseCase.MoveFiles(ctx, &request)
	if err != nil {
		log.Printf("❌ 파일 이동 실패: %v", err)
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

// DeleteFiles handles the delete files endpoint
// DELETE /api/explorer/delete-files
func (c *FileExplorerController) DeleteFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		FileIDs         []string `json:"fileIds"`
		PermanentDelete bool     `json:"permanentDelete"` // true = 완전 삭제, false = 휴지통 이동
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Delete files (permanent or trash)
	response, err := c.fileExplorerUseCase.DeleteFiles(ctx, request.FileIDs, request.PermanentDelete)
	if err != nil {
		log.Printf("❌ 파일 삭제 실패: %v", err)
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

// RenameFile handles the rename file endpoint
// PUT /api/explorer/rename-file
func (c *FileExplorerController) RenameFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		FileID  string `json:"fileId"`
		NewName string `json:"newName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if request.FileID == "" || request.NewName == "" {
		http.Error(w, "fileId and newName are required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Rename file
	if err := c.fileExplorerUseCase.RenameFile(ctx, request.FileID, request.NewName); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"message": "File renamed successfully",
		"fileId":  request.FileID,
		"newName": request.NewName,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// CreateFolder handles the create folder endpoint
// POST /api/explorer/create-folder
func (c *FileExplorerController) CreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ParentFolderID string `json:"parentFolderId"`
		FolderName     string `json:"folderName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate request
	if request.ParentFolderID == "" || request.FolderName == "" {
		http.Error(w, "parentFolderId and folderName are required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create folder
	folderItem, err := c.fileExplorerUseCase.CreateFolder(ctx, request.ParentFolderID, request.FolderName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(folderItem); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetFileInfo handles the get file info endpoint
// GET /api/explorer/file-info?fileId={id}
func (c *FileExplorerController) GetFileInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameter
	fileID := r.URL.Query().Get("fileId")
	if fileID == "" {
		http.Error(w, "fileId is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get file info
	fileInfo, err := c.fileExplorerUseCase.GetFileInfo(ctx, fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fileInfo); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// SearchFiles handles the search files endpoint
// GET /api/explorer/search?query={query}&sortBy={field}&order={asc|desc}
func (c *FileExplorerController) SearchFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query is required", http.StatusBadRequest)
		return
	}

	// Parse list options
	options := entities.DefaultListOptions()
	if sortBy := r.URL.Query().Get("sortBy"); sortBy != "" {
		options.SortBy = sortBy
	}
	if order := r.URL.Query().Get("order"); order != "" {
		options.SortOrder = order
	}
	if pageToken := r.URL.Query().Get("pageToken"); pageToken != "" {
		options.PageToken = pageToken
	}
	if pageSize := r.URL.Query().Get("pageSize"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil && ps > 0 {
			options.PageSize = ps
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Search files
	response, err := c.fileExplorerUseCase.SearchFiles(ctx, query, options)
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

// GetOperationProgress handles the get operation progress endpoint
// GET /api/explorer/operation/progress?operationId={id}
func (c *FileExplorerController) GetOperationProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameter
	operationIDStr := r.URL.Query().Get("operationId")
	if operationIDStr == "" {
		http.Error(w, "operationId is required", http.StatusBadRequest)
		return
	}

	operationID, err := strconv.Atoi(operationIDStr)
	if err != nil {
		http.Error(w, "Invalid operationId", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get operation progress
	operation, err := c.fileExplorerUseCase.GetOperationProgress(ctx, operationID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(operation); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// CancelOperation handles the cancel operation endpoint
// DELETE /api/explorer/operation/cancel?operationId={id}
func (c *FileExplorerController) CancelOperation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameter
	operationIDStr := r.URL.Query().Get("operationId")
	if operationIDStr == "" {
		http.Error(w, "operationId is required", http.StatusBadRequest)
		return
	}

	operationID, err := strconv.Atoi(operationIDStr)
	if err != nil {
		http.Error(w, "Invalid operationId", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Cancel operation
	if err := c.fileExplorerUseCase.CancelOperation(ctx, operationID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":     true,
		"message":     "Operation cancelled successfully",
		"operationId": operationID,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetFolderSize handles the get folder size endpoint
// GET /api/explorer/folder-size?folderId={id}
func (c *FileExplorerController) GetFolderSize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameter
	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		http.Error(w, "folderId is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout (longer timeout for large folders)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// Get folder size
	sizeInfo, err := c.fileExplorerUseCase.GetFolderSize(ctx, folderID)
	if err != nil {
		log.Printf("❌ 폴더 크기 계산 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sizeInfo); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
