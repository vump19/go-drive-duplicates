package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DuplicateController handles HTTP requests related to duplicate operations
type DuplicateController struct {
	duplicateFindingUseCase *usecases.DuplicateFindingUseCase
	apiTimeout              time.Duration
}

// NewDuplicateController creates a new duplicate controller
func NewDuplicateController(duplicateFindingUseCase *usecases.DuplicateFindingUseCase, apiTimeout time.Duration) *DuplicateController {
	return &DuplicateController{
		duplicateFindingUseCase: duplicateFindingUseCase,
		apiTimeout:              apiTimeout,
	}
}

// FindDuplicates handles the find duplicates endpoint
func (c *DuplicateController) FindDuplicates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.FindDuplicatesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Try to parse from query parameters if body parsing fails
		req.CalculateHashes = r.URL.Query().Get("calculateHashes") == "true"
		req.ForceRecalculate = r.URL.Query().Get("forceRecalculate") == "true"

		if minSizeStr := r.URL.Query().Get("minFileSize"); minSizeStr != "" {
			if minSize, err := strconv.ParseInt(minSizeStr, 10, 64); err == nil && minSize > 0 {
				req.MinFileSize = minSize
			}
		}

		if maxResultsStr := r.URL.Query().Get("maxResults"); maxResultsStr != "" {
			if maxResults, err := strconv.Atoi(maxResultsStr); err == nil && maxResults > 0 {
				req.MaxResults = maxResults
			}
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.duplicateFindingUseCase.FindDuplicates(ctx, &req)
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

// FindDuplicatesInFolder handles the find duplicates in folder endpoint
func (c *DuplicateController) FindDuplicatesInFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.FindDuplicatesInFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FolderID == "" {
		http.Error(w, "FolderID is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.duplicateFindingUseCase.FindDuplicatesInFolder(ctx, &req)
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

// CalculateHashes handles the calculate hashes endpoint
func (c *DuplicateController) CalculateHashes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.CalculateHashesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Try to parse from query parameters if body parsing fails
		req.ForceRecalculate = r.URL.Query().Get("forceRecalculate") == "true"

		if workerCountStr := r.URL.Query().Get("workerCount"); workerCountStr != "" {
			if workerCount, err := strconv.Atoi(workerCountStr); err == nil && workerCount > 0 {
				req.WorkerCount = workerCount
			}
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.duplicateFindingUseCase.CalculateHashes(ctx, &req)
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

// GetDuplicateProgress handles the get duplicate progress endpoint
func (c *DuplicateController) GetDuplicateProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get progress
	progress, err := c.duplicateFindingUseCase.GetDuplicateProgress(ctx)
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

// GetResumableHashCalculation handles checking for resumable hash calculation tasks
func (c *DuplicateController) GetResumableHashCalculation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get resumable task
	progress, err := c.duplicateFindingUseCase.GetResumableHashCalculation(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response
	response := map[string]interface{}{
		"hasResumableTask": progress != nil,
	}

	if progress != nil {
		// 재개 가능한 작업이 있을 때 상세 정보 포함
		currentBatch := 0
		totalSuccessful := 0
		totalFailed := 0

		if batch, ok := progress.GetMetadata("currentBatch"); ok {
			if batchNum, ok := batch.(float64); ok {
				currentBatch = int(batchNum)
			}
		}

		if success, ok := progress.GetMetadata("totalSuccessful"); ok {
			if count, ok := success.(float64); ok {
				totalSuccessful = int(count)
			}
		}

		if fail, ok := progress.GetMetadata("totalFailed"); ok {
			if count, ok := fail.(float64); ok {
				totalFailed = int(count)
			}
		}

		response["progress"] = map[string]interface{}{
			"id":              progress.ID,
			"currentBatch":    currentBatch,
			"totalSuccessful": totalSuccessful,
			"totalFailed":     totalFailed,
			"startTime":       progress.StartTime,
			"lastUpdated":     progress.LastUpdated,
		}
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetDuplicateGroups handles the get duplicate groups endpoint with pagination
func (c *DuplicateController) GetDuplicateGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination parameters
	page := 1
	limit := 20

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Create context with timeout from config
	ctx, cancel := context.WithTimeout(context.Background(), c.apiTimeout)
	defer cancel()

	// Get duplicate groups from use case with pagination
	result, err := c.duplicateFindingUseCase.GetDuplicateGroupsPaginated(ctx, page, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Printf("❌ 중복 그룹 응답 인코딩 실패: %v (page=%d, limit=%d, totalGroups=%d)",
			err, page, limit, result.TotalGroups)
		// 클라이언트 연결이 끊어진 경우 (write: broken pipe, connection reset by peer)
		// 이미 연결이 끊어졌으므로 http.Error를 보내도 의미가 없음
		return
	}
}

// GetDuplicateGroup handles the get specific duplicate group endpoint
func (c *DuplicateController) GetDuplicateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse group ID from query parameters
	groupIDStr := r.URL.Query().Get("id")
	if groupIDStr == "" {
		http.Error(w, "Group ID is required", http.StatusBadRequest)
		return
	}

	groupID, err := strconv.Atoi(groupIDStr)
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout (설정된 apiTimeout 사용, 대용량 그룹 조회 시 필요)
	ctx, cancel := context.WithTimeout(context.Background(), c.apiTimeout)
	defer cancel()

	// Get specific duplicate group
	group, err := c.duplicateFindingUseCase.GetDuplicateGroup(ctx, groupID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(group); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// DeleteDuplicateGroup handles the delete duplicate group endpoint
func (c *DuplicateController) DeleteDuplicateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse group ID from query parameters
	groupIDStr := r.URL.Query().Get("id")
	if groupIDStr == "" {
		http.Error(w, "Group ID is required", http.StatusBadRequest)
		return
	}

	groupID, err := strconv.Atoi(groupIDStr)
	if err != nil {
		http.Error(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Delete duplicate group
	err = c.duplicateFindingUseCase.DeleteDuplicateGroup(ctx, groupID)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "찾을 수 없습니다") {
			http.Error(w, "이미 삭제되었거나 존재하지 않는 그룹입니다", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"message": "Duplicate group deleted successfully",
		"groupId": groupID,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetFilePath handles the get file path endpoint
func (c *DuplicateController) GetFilePath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse file ID from query parameters
	fileID := r.URL.Query().Get("fileId")
	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get file path information
	pathInfo, err := c.duplicateFindingUseCase.GetFilePath(ctx, fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pathInfo); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// TrashFile handles trashing a single file from the duplicate group modal
func (c *DuplicateController) TrashFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse file ID from query parameters
	fileID := r.URL.Query().Get("fileId")
	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Trash the file
	err := c.duplicateFindingUseCase.TrashFile(ctx, fileID)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "notFound") {
			http.Error(w, "파일을 찾을 수 없습니다", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"message": "파일이 휴지통으로 이동되었습니다",
		"fileId":  fileID,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// UpdateGroupMemo handles updating the memo for a duplicate group
func (c *DuplicateController) UpdateGroupMemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		GroupID int    `json:"groupId"`
		Memo    string `json:"memo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.GroupID == 0 {
		http.Error(w, "Group ID is required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := c.duplicateFindingUseCase.UpdateGroupMemo(ctx, req.GroupID, req.Memo)
	if err != nil {
		if strings.Contains(err.Error(), "찾을 수 없습니다") {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "메모가 저장되었습니다",
	})
}

// GetDuplicateGroupByHash handles the get duplicate group by hash endpoint
func (c *DuplicateController) GetDuplicateGroupByHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse hash from query parameters
	hash := r.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "Hash is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get duplicate group by hash
	group, err := c.duplicateFindingUseCase.GetDuplicateGroupByHash(ctx, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(group); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ValidateAndCleanupDuplicateGroups starts validation and cleanup of duplicate groups
// POST /api/duplicates/validate
func (c *DuplicateController) ValidateAndCleanupDuplicateGroups(w http.ResponseWriter, r *http.Request) {
	// Set timeout for the API request (not the background operation)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Parse request body
	var req usecases.ValidateAndCleanupDuplicateGroupsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Start validation process
	response, err := c.duplicateFindingUseCase.ValidateAndCleanupDuplicateGroups(ctx, &req)
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

// CheckResumableTasks checks if there are any resumable duplicate search tasks
// GET /api/duplicates/resumable
func (c *DuplicateController) CheckResumableTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Check for resumable tasks
	taskInfo, err := c.duplicateFindingUseCase.CheckResumableTasks(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Build response
	response := map[string]interface{}{
		"hasResumableTask": taskInfo != nil,
	}

	if taskInfo != nil {
		response["task"] = taskInfo
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ResetProgress deletes all duplicate search progress to start fresh
// DELETE /api/duplicates/reset
func (c *DuplicateController) ResetProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	// Reset all progress
	err := c.duplicateFindingUseCase.ResetAllProgress(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success": true,
		"message": "모든 진행 상황이 삭제되었습니다",
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
