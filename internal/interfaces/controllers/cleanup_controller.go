package controllers

import (
	"context"
	"encoding/json"
	"go-drive-duplicates/internal/usecases"
	"net/http"
	"strings"
	"time"
)

// CleanupController handles HTTP requests related to file cleanup operations
type CleanupController struct {
	fileCleanupUseCase *usecases.FileCleanupUseCase
}

// NewCleanupController creates a new cleanup controller
func NewCleanupController(fileCleanupUseCase *usecases.FileCleanupUseCase) *CleanupController {
	return &CleanupController{
		fileCleanupUseCase: fileCleanupUseCase,
	}
}

// DeleteFiles handles the delete files endpoint
func (c *CleanupController) DeleteFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.DeleteFilesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if len(req.FileIDs) == 0 {
		http.Error(w, "FileIDs are required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.DeleteFiles(ctx, &req)
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

// DeleteDuplicatesFromGroup handles the delete duplicates from group endpoint
func (c *CleanupController) DeleteDuplicatesFromGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.DeleteDuplicatesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.GroupID == 0 || req.KeepFileID == "" {
		http.Error(w, "GroupID and KeepFileID are required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.DeleteDuplicatesFromGroup(ctx, &req)
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

// BulkDeleteByPattern handles the bulk delete by pattern endpoint
func (c *CleanupController) BulkDeleteByPattern(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.BulkDeleteByPatternRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FolderID == "" || req.Pattern == "" {
		http.Error(w, "FolderID and Pattern are required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.BulkDeleteByPattern(ctx, &req)
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

// CleanupEmptyFolders handles the cleanup empty folders endpoint
func (c *CleanupController) CleanupEmptyFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.CleanupEmptyFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Try to parse from query parameters if body parsing fails
		req.RootFolderID = r.URL.Query().Get("rootFolderId")
		req.Recursive = r.URL.Query().Get("recursive") == "true"
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.CleanupEmptyFolders(ctx, &req)
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

// GetCleanupProgress handles the get cleanup progress endpoint
func (c *CleanupController) GetCleanupProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get progress
	progress, err := c.fileCleanupUseCase.GetCleanupProgress(ctx)
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

// SearchFilesToDelete handles the search files to delete endpoint (for pattern matching preview)
func (c *CleanupController) SearchFilesToDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req usecases.BulkDeleteByPatternRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.FolderID == "" || req.Pattern == "" {
		http.Error(w, "FolderID and Pattern are required", http.StatusBadRequest)
		return
	}

	// Force dry run for search
	req.DryRun = true

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.BulkDeleteByPattern(ctx, &req)
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

// DeleteFileByID handles deletion of a single file by ID (for legacy compatibility)
func (c *CleanupController) DeleteFileByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse file ID from query parameter
	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Parse cleanup folders option
	cleanupFolders := r.URL.Query().Get("cleanupFolders") == "true"

	// Create delete files request
	req := &usecases.DeleteFilesRequest{
		FileIDs:        []string{fileID},
		CleanupFolders: cleanupFolders,
		SafetyChecks:   true,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.DeleteFiles(ctx, req)
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

// DeleteMultipleFiles handles deletion of multiple files from form data (for legacy compatibility)
func (c *CleanupController) DeleteMultipleFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get file IDs from form
	fileIDsStr := r.FormValue("fileIds")
	if fileIDsStr == "" {
		http.Error(w, "File IDs are required", http.StatusBadRequest)
		return
	}

	// Split file IDs
	fileIDs := strings.Split(fileIDsStr, ",")
	for i, id := range fileIDs {
		fileIDs[i] = strings.TrimSpace(id)
	}

	// Parse cleanup folders option
	cleanupFolders := r.FormValue("cleanupFolders") == "true"

	// Create delete files request
	req := &usecases.DeleteFilesRequest{
		FileIDs:        fileIDs,
		CleanupFolders: cleanupFolders,
		SafetyChecks:   true,
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Execute use case
	response, err := c.fileCleanupUseCase.DeleteFiles(ctx, req)
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
