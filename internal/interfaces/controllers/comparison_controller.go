package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/domain/services"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ComparisonController handles HTTP requests related to folder comparison operations
type ComparisonController struct {
	folderComparisonUseCase *usecases.FolderComparisonUseCase
	storageProvider         services.StorageProvider
}

// NewComparisonController creates a new comparison controller
func NewComparisonController(folderComparisonUseCase *usecases.FolderComparisonUseCase, storageProvider services.StorageProvider) *ComparisonController {
	return &ComparisonController{
		folderComparisonUseCase: folderComparisonUseCase,
		storageProvider:         storageProvider,
	}
}

// CompareFolders handles the compare folders endpoint
func (c *ComparisonController) CompareFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.CompareFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.SourceFolderID == "" || req.TargetFolderID == "" {
		http.Error(w, "SourceFolderID and TargetFolderID are required", http.StatusBadRequest)
		return
	}

	// Create context without timeout for folder comparison (large files may take a long time)
	ctx := context.Background()

	// Execute use case
	response, err := c.folderComparisonUseCase.CompareFolders(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 버퍼에 먼저 인코딩하여 오류 확인
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(response); err != nil {
		log.Printf("❌ JSON 인코딩 실패: %v", err)
		http.Error(w, fmt.Sprintf("JSON 인코딩 실패: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("❌ HTTP 응답 쓰기 실패: %v", err)
		return
	}
}

// GetComparisonProgress handles the get comparison progress endpoint
func (c *ComparisonController) GetComparisonProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse comparison ID from query parameter
	comparisonIDStr := r.URL.Query().Get("id")
	if comparisonIDStr == "" {
		http.Error(w, "Comparison ID is required", http.StatusBadRequest)
		return
	}

	comparisonID, err := strconv.Atoi(comparisonIDStr)
	if err != nil {
		http.Error(w, "Invalid comparison ID", http.StatusBadRequest)
		return
	}

	// Create request
	req := &usecases.GetComparisonProgressRequest{
		ComparisonID: comparisonID,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute use case
	progress, err := c.folderComparisonUseCase.GetComparisonProgress(ctx, req)
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

// LoadSavedComparison handles the load saved comparison endpoint
func (c *ComparisonController) LoadSavedComparison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	sourceFolderID := r.URL.Query().Get("sourceFolderId")
	targetFolderID := r.URL.Query().Get("targetFolderId")

	if sourceFolderID == "" || targetFolderID == "" {
		http.Error(w, "sourceFolderId and targetFolderId are required", http.StatusBadRequest)
		return
	}

	// Create request
	req := &usecases.LoadSavedComparisonRequest{
		SourceFolderID: sourceFolderID,
		TargetFolderID: targetFolderID,
	}

	// Create context with timeout (대용량 비교 결과 로드를 위해 충분한 시간 확보)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Execute use case
	comparison, err := c.folderComparisonUseCase.LoadSavedComparison(ctx, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(comparison); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// DeleteComparisonResult handles the delete comparison result endpoint
func (c *ComparisonController) DeleteComparisonResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse comparison ID from query parameter
	comparisonIDStr := r.URL.Query().Get("id")
	if comparisonIDStr == "" {
		http.Error(w, "Comparison ID is required", http.StatusBadRequest)
		return
	}

	comparisonID, err := strconv.Atoi(comparisonIDStr)
	if err != nil {
		http.Error(w, "Invalid comparison ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute use case
	err = c.folderComparisonUseCase.DeleteComparisonResult(ctx, comparisonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GetRecentComparisons handles the get recent comparisons endpoint
func (c *ComparisonController) GetRecentComparisons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit from query parameter
	limit := 10 // default limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute use case
	comparisons, err := c.folderComparisonUseCase.GetRecentComparisons(ctx, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(comparisons); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// DeleteTargetFolder handles the delete target folder endpoint (for 100% duplicated folders)
func (c *ComparisonController) DeleteTargetFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.DeleteTargetFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ComparisonID == 0 || req.TargetFolderID == "" {
		http.Error(w, "ComparisonID and TargetFolderID are required", http.StatusBadRequest)
		return
	}

	// Create context without timeout for folder deletion (may contain large files)
	ctx := context.Background()

	// Execute use case
	response, err := c.folderComparisonUseCase.DeleteTargetFolder(ctx, &req)
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

// DeleteDuplicateFiles handles the delete duplicate files endpoint
func (c *ComparisonController) DeleteDuplicateFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.DeleteDuplicateFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ComparisonID == 0 || len(req.FileIDs) == 0 {
		http.Error(w, "ComparisonID and FileIDs are required", http.StatusBadRequest)
		return
	}

	// Create context without timeout for file deletion (large files may take a long time)
	ctx := context.Background()

	// Execute use case
	response, err := c.folderComparisonUseCase.DeleteDuplicateFiles(ctx, &req)
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

// ResumeComparison handles the resume comparison endpoint
func (c *ComparisonController) ResumeComparison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.ResumeComparisonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ProgressID == 0 {
		http.Error(w, "ProgressID is required", http.StatusBadRequest)
		return
	}

	// Create context without timeout for comparison resumption
	ctx := context.Background()

	// Execute use case
	response, err := c.folderComparisonUseCase.ResumeComparison(ctx, &req)
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

// GetPendingComparisons handles the get pending comparisons endpoint
func (c *ComparisonController) GetPendingComparisons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute use case
	pendingComparisons, err := c.folderComparisonUseCase.GetPendingComparisons(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(pendingComparisons); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// ExtractFolderIdFromUrl handles the extract folder ID from URL endpoint
func (c *ComparisonController) ExtractFolderIdFromUrl(w http.ResponseWriter, r *http.Request) {
	var url string

	if r.Method == http.MethodPost {
		// Parse JSON request body for POST
		var reqBody struct {
			Url string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		url = reqBody.Url
	} else if r.Method == http.MethodGet {
		// Parse query parameter for GET
		url = r.URL.Query().Get("url")
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate required fields
	if url == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Extract folder ID
	folderID, err := c.folderComparisonUseCase.ExtractFolderIdFromUrl(url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{
		"folderId": folderID,
		"url":      url,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// FindDuplicatesInSingleFolder handles the single folder duplicate detection endpoint
func (c *ComparisonController) FindDuplicatesInSingleFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.FindDuplicatesInSingleFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FolderID == "" {
		http.Error(w, "FolderID is required", http.StatusBadRequest)
		return
	}

	// Create context without timeout for folder scanning (large folders may take a long time)
	ctx := context.Background()

	// Execute use case (starts background goroutine)
	response, err := c.folderComparisonUseCase.FindDuplicatesInSingleFolder(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Create a safe response with only the progress ID
	// The background goroutine will modify the response object, so we can't return it directly
	safeResponse := map[string]interface{}{
		"progressId": response.ProgressId,
		"message":    "단일 폴더 중복 검색이 백그라운드에서 시작되었습니다",
		"status":     "started",
	}

	// Return safe response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(safeResponse); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetSingleFolderDuplicateProgress handles the get single folder duplicate progress endpoint
func (c *ComparisonController) GetSingleFolderDuplicateProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse progress ID from query parameter
	progressIDStr := r.URL.Query().Get("progressId")
	if progressIDStr == "" {
		http.Error(w, "Progress ID is required", http.StatusBadRequest)
		return
	}

	progressID, err := strconv.Atoi(progressIDStr)
	if err != nil {
		http.Error(w, "Invalid progress ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create request for getting comparison progress (reuse existing structure)
	req := &usecases.GetComparisonProgressRequest{
		ComparisonID: progressID,
	}

	// Execute use case
	progress, err := c.folderComparisonUseCase.GetComparisonProgress(ctx, req)
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

// GetRecentSingleFolderResults handles the get recent single-folder duplicate results endpoint
func (c *ComparisonController) GetRecentSingleFolderResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results, err := c.folderComparisonUseCase.GetRecentSingleFolderResults(ctx, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// MoveUniqueFiles handles the move unique files endpoint
func (c *ComparisonController) MoveUniqueFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.MoveUniqueFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ComparisonID == 0 {
		http.Error(w, "ComparisonID is required", http.StatusBadRequest)
		return
	}

	// Set defaults
	if req.OnConflict == "" {
		req.OnConflict = "rename" // Default to rename on conflict
	}

	// Create context without timeout for file moving
	ctx := context.Background()

	// Execute use case
	response, err := c.folderComparisonUseCase.MoveUniqueFilesToSource(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return initial response with progress ID
	safeResponse := map[string]interface{}{
		"progressId": response.Progress.ID,
		"message":    "비중복 파일 이동이 백그라운드에서 시작되었습니다",
		"status":     "started",
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(safeResponse); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetUniqueFilesForComparison handles the get unique files for comparison endpoint
func (c *ComparisonController) GetUniqueFilesForComparison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse comparison ID from query parameter
	comparisonIDStr := r.URL.Query().Get("comparisonId")
	if comparisonIDStr == "" {
		http.Error(w, "comparisonId is required", http.StatusBadRequest)
		return
	}

	comparisonID, err := strconv.Atoi(comparisonIDStr)
	if err != nil {
		http.Error(w, "Invalid comparison ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	files, err := c.folderComparisonUseCase.GetUniqueFilesForComparison(ctx, comparisonID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return response
	response := map[string]interface{}{
		"comparisonId": comparisonID,
		"uniqueFiles":  files,
		"totalCount":   len(files),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// CancelComparison handles the cancel comparison endpoint
func (c *ComparisonController) CancelComparison(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		ProgressID int `json:"progressId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ProgressID == 0 {
		http.Error(w, "progressId is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute use case
	err := c.folderComparisonUseCase.CancelComparison(ctx, req.ProgressID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	response := map[string]interface{}{
		"success":    true,
		"message":    "작업이 취소되었습니다",
		"progressId": req.ProgressID,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// GetMoveUniqueFilesProgress handles the get move unique files progress endpoint
func (c *ComparisonController) GetMoveUniqueFilesProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse progress ID from query parameter
	progressIDStr := r.URL.Query().Get("progressId")
	if progressIDStr == "" {
		http.Error(w, "progressId is required", http.StatusBadRequest)
		return
	}

	progressID, err := strconv.Atoi(progressIDStr)
	if err != nil {
		http.Error(w, "Invalid progress ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create request
	req := &usecases.GetComparisonProgressRequest{
		ComparisonID: progressID,
	}

	// Execute use case
	progress, err := c.folderComparisonUseCase.GetComparisonProgress(ctx, req)
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

// CancelMoveUniqueFiles handles the cancel move unique files endpoint
func (c *ComparisonController) CancelMoveUniqueFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse progress ID from query parameter
	progressIDStr := r.URL.Query().Get("progressId")
	if progressIDStr == "" {
		http.Error(w, "progressId is required", http.StatusBadRequest)
		return
	}

	progressID, err := strconv.Atoi(progressIDStr)
	if err != nil {
		http.Error(w, "Invalid progress ID", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Execute cancel
	err = c.folderComparisonUseCase.CancelMoveUniqueFiles(ctx, progressID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "이동 작업이 취소되었습니다",
	})
}

// GetActiveMoveOperations returns all active move_unique_files operations
// GET /api/compare/move-unique-files/active
func (c *ComparisonController) GetActiveMoveOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ops, err := c.folderComparisonUseCase.GetActiveMoveOperations(ctx)
	if err != nil {
		log.Printf("❌ 활성 이동 작업 조회 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if ops == nil {
		ops = []usecases.ActiveMoveOperation{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ops)
}

// ResolveFolderPath handles the resolve folder path endpoint
// GET /api/utils/resolve-folder?id=FOLDER_ID_OR_URL
func (c *ComparisonController) ResolveFolderPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	input := r.URL.Query().Get("id")
	if input == "" {
		http.Error(w, "id 파라미터가 필요합니다", http.StatusBadRequest)
		return
	}

	// URL이면 폴더 ID 추출
	folderID := input
	if strings.Contains(input, "drive.google.com") || strings.Contains(input, "/folders/") {
		extracted, err := c.folderComparisonUseCase.ExtractFolderIdFromUrl(input)
		if err != nil {
			log.Printf("⚠️ 폴더 URL에서 ID 추출 실패: %v", err)
			http.Error(w, fmt.Sprintf("폴더 URL에서 ID를 추출할 수 없습니다: %v", err), http.StatusBadRequest)
			return
		}
		folderID = extracted
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 폴더 메타데이터 조회
	file, err := c.storageProvider.GetFile(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 정보 조회 실패: %s - %v", folderID, err)
		http.Error(w, fmt.Sprintf("폴더 정보를 조회할 수 없습니다: %v", err), http.StatusNotFound)
		return
	}

	// 전체 경로 조회
	path, err := c.storageProvider.GetFolderPath(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 경로 조회 실패: %s - %v", folderID, err)
		// 경로 조회 실패해도 이름은 반환
		path = "/" + file.Name
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"folderId": folderID,
		"name":     file.Name,
		"path":     path,
	})
}
