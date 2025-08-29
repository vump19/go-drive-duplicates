package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ComparisonController handles HTTP requests related to folder comparison operations
type ComparisonController struct {
	folderComparisonUseCase *usecases.FolderComparisonUseCase
}

// NewComparisonController creates a new comparison controller
func NewComparisonController(folderComparisonUseCase *usecases.FolderComparisonUseCase) *ComparisonController {
	return &ComparisonController{
		folderComparisonUseCase: folderComparisonUseCase,
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

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
