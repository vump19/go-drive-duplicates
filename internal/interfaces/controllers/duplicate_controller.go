package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"net/http"
	"strconv"
	"time"
)

// DuplicateController handles HTTP requests related to duplicate operations
type DuplicateController struct {
	duplicateFindingUseCase *usecases.DuplicateFindingUseCase
}

// NewDuplicateController creates a new duplicate controller
func NewDuplicateController(duplicateFindingUseCase *usecases.DuplicateFindingUseCase) *DuplicateController {
	return &DuplicateController{
		duplicateFindingUseCase: duplicateFindingUseCase,
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
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

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
