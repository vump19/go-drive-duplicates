package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/usecases"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"
)

// PrepareProgressState stores the progress state for a prepare operation
type PrepareProgressState struct {
	Status   string                          `json:"status"` // "preparing", "completed", "failed"
	Progress *usecases.PrepareProgress       `json:"progress,omitempty"`
	Result   *usecases.PrepareDownloadResult `json:"result,omitempty"`
	Error    string                          `json:"error,omitempty"`
}

// ServerDownloadState stores the state of a server-side download
type ServerDownloadState struct {
	Status   string                           `json:"status"` // "downloading", "creating_zip", "completed", "failed", "cancelled"
	Progress *usecases.ServerDownloadProgress `json:"progress,omitempty"`
	Result   *usecases.ServerDownloadResult   `json:"result,omitempty"`
	Error    string                           `json:"error,omitempty"`
}

// FolderDownloadController handles folder download HTTP requests
type FolderDownloadController struct {
	folderDownloadUseCase *usecases.FolderDownloadUseCase

	// SSE 클라이언트 관리
	sseClients   map[string]chan *usecases.DownloadProgress
	sseClientsMu sync.RWMutex

	// 폴링 방식 진행 상황 관리
	prepareProgress   map[string]*PrepareProgressState
	prepareProgressMu sync.RWMutex

	// 서버 다운로드 진행 상황 관리
	serverDownloads   map[string]*ServerDownloadState
	serverDownloadsMu sync.RWMutex
}

// NewFolderDownloadController creates a new folder download controller
func NewFolderDownloadController(folderDownloadUseCase *usecases.FolderDownloadUseCase) *FolderDownloadController {
	return &FolderDownloadController{
		folderDownloadUseCase: folderDownloadUseCase,
		sseClients:            make(map[string]chan *usecases.DownloadProgress),
		prepareProgress:       make(map[string]*PrepareProgressState),
		serverDownloads:       make(map[string]*ServerDownloadState),
	}
}

// PrepareDownloadRequest represents the request to prepare download
type PrepareDownloadRequest struct {
	FolderID          string `json:"folderId"`
	URL               string `json:"url,omitempty"`
	IncludeSubfolders bool   `json:"includeSubfolders"`
	SkipGoogleDocs    bool   `json:"skipGoogleDocs"`
}

// extractFolderIDFromURL extracts folder ID from Google Drive URL
func extractDownloadFolderIDFromURL(url string) (string, error) {
	// Pattern 1: https://drive.google.com/drive/folders/{folderId}
	pattern1 := regexp.MustCompile(`/folders/([a-zA-Z0-9_-]+)`)
	if matches := pattern1.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	// Pattern 2: https://drive.google.com/drive/u/0/folders/{folderId}
	pattern2 := regexp.MustCompile(`/drive/u/\d+/folders/([a-zA-Z0-9_-]+)`)
	if matches := pattern2.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	// Pattern 3: folder ID in query parameter
	pattern3 := regexp.MustCompile(`[?&]id=([a-zA-Z0-9_-]+)`)
	if matches := pattern3.FindStringSubmatch(url); len(matches) > 1 {
		return matches[1], nil
	}

	return "", fmt.Errorf("유효하지 않은 Google Drive URL입니다")
}

// PrepareDownload handles download preparation requests
func (c *FolderDownloadController) PrepareDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PrepareDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderID := req.FolderID

	// Extract folder ID from URL if provided
	if folderID == "" && req.URL != "" {
		extractedID, err := extractDownloadFolderIDFromURL(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		folderID = extractedID
	}

	if folderID == "" {
		http.Error(w, "폴더 ID 또는 URL이 필요합니다", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	options := &usecases.DownloadOptions{
		IncludeSubfolders:   req.IncludeSubfolders,
		SkipGoogleDocs:      req.SkipGoogleDocs,
		ConcurrentDownloads: 3,
	}

	// Set defaults
	if !req.IncludeSubfolders && !req.SkipGoogleDocs && req.URL == "" && req.FolderID != "" {
		// Use defaults when only folder ID is provided
		options = usecases.DefaultDownloadOptions()
	}

	log.Printf("📦 다운로드 준비 요청: 폴더 ID %s", folderID)

	result, err := c.folderDownloadUseCase.PrepareDownload(ctx, folderID, options)
	if err != nil {
		log.Printf("❌ 다운로드 준비 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// PrepareDownloadWithSSE handles download preparation with SSE progress streaming
func (c *FolderDownloadController) PrepareDownloadWithSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderID := r.URL.Query().Get("folderId")
	url := r.URL.Query().Get("url")

	// Extract folder ID from URL if provided
	if folderID == "" && url != "" {
		extractedID, err := extractDownloadFolderIDFromURL(url)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		folderID = extractedID
	}

	if folderID == "" {
		http.Error(w, "폴더 ID 또는 URL이 필요합니다", http.StatusBadRequest)
		return
	}

	// Parse options from query parameters
	options := &usecases.DownloadOptions{
		IncludeSubfolders:   r.URL.Query().Get("includeSubfolders") != "false",
		SkipGoogleDocs:      r.URL.Query().Get("skipGoogleDocs") != "false",
		ConcurrentDownloads: 3,
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // Disable proxy buffering

	// Get flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Flush headers immediately
	flusher.Flush()

	log.Printf("📦 다운로드 준비 SSE 시작: 폴더 ID %s", folderID)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	// Send initial event
	sendSSEEvent(w, flusher, "started", map[string]interface{}{
		"folderId": folderID,
		"message":  "다운로드 준비 시작",
	})

	// Prepare download with progress
	result, err := c.folderDownloadUseCase.PrepareDownloadWithProgress(ctx, folderID, options, func(progress *usecases.PrepareProgress) {
		sendSSEEvent(w, flusher, "progress", progress)
	})

	if err != nil {
		log.Printf("❌ 다운로드 준비 SSE 실패: %v", err)
		sendSSEEvent(w, flusher, "error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Send final result
	sendSSEEvent(w, flusher, "completed", result)

	// Send done event to signal end of stream
	sendSSEEvent(w, flusher, "done", map[string]interface{}{
		"status": "completed",
	})

	// Extra flush to ensure all data is sent
	flusher.Flush()

	// Small delay to ensure browser receives all data before connection closes
	time.Sleep(100 * time.Millisecond)

	log.Printf("✅ 다운로드 준비 SSE 완료: %s", folderID)
}

// StreamDownload handles ZIP streaming download
func (c *FolderDownloadController) StreamDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderID := r.URL.Query().Get("folderId")
	url := r.URL.Query().Get("url")
	prepareId := r.URL.Query().Get("prepareId")

	// Extract folder ID from URL if provided
	if folderID == "" && url != "" {
		extractedID, err := extractDownloadFolderIDFromURL(url)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		folderID = extractedID
	}

	if folderID == "" {
		http.Error(w, "폴더 ID 또는 URL이 필요합니다", http.StatusBadRequest)
		return
	}

	// Parse options from query parameters
	options := &usecases.DownloadOptions{
		IncludeSubfolders:   r.URL.Query().Get("includeSubfolders") != "false",
		SkipGoogleDocs:      r.URL.Query().Get("skipGoogleDocs") != "false",
		ConcurrentDownloads: 3,
	}

	log.Printf("📥 ZIP 다운로드 시작: 폴더 ID %s, prepareId %s", folderID, prepareId)

	ctx := r.Context()

	// Check if we have cached prepare result
	var cachedResult *usecases.PrepareDownloadResult
	if prepareId != "" {
		c.prepareProgressMu.RLock()
		state, ok := c.prepareProgress[prepareId]
		c.prepareProgressMu.RUnlock()

		if ok && state.Status == "completed" && state.Result != nil && len(state.Result.Files) > 0 {
			cachedResult = state.Result
			log.Printf("✅ 캐시된 파일 목록 사용: %d개 파일", len(cachedResult.Files))
		}
	}

	// Get folder name from cached result or query parameter
	folderName := r.URL.Query().Get("folderName")
	if folderName == "" && cachedResult != nil {
		folderName = cachedResult.FolderName
	}
	if folderName == "" {
		// 폴더 이름이 없으면 API로 조회
		folderInfo, err := c.folderDownloadUseCase.GetFolderInfo(ctx, folderID)
		if err != nil {
			log.Printf("❌ 폴더 정보 조회 실패: %v", err)
			http.Error(w, fmt.Sprintf("폴더 정보 조회 실패: %v", err), http.StatusInternalServerError)
			return
		}
		folderName = folderInfo.Name
	}

	// Set response headers for ZIP download IMMEDIATELY
	fileName := fmt.Sprintf("%s.zip", folderName)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Transfer-Encoding", "chunked")

	log.Printf("📦 ZIP 스트리밍 시작: %s", folderName)

	var streamErr error
	if cachedResult != nil {
		// Use cached file list - no re-scanning needed
		log.Printf("🚀 캐시된 파일 목록으로 다운로드 시작 (스캔 생략)")
		streamErr = c.folderDownloadUseCase.StreamDownloadWithCachedFiles(ctx, cachedResult, options, w, func(progress *usecases.DownloadProgress) {
			// Send progress to SSE clients
			c.broadcastProgress(folderID, progress)
		})
	} else {
		// Fallback to original method (will re-scan files)
		log.Printf("⚠️ 캐시된 파일 목록 없음, 전체 스캔 진행")
		streamErr = c.folderDownloadUseCase.StreamDownloadAsZip(ctx, folderID, options, w, func(progress *usecases.DownloadProgress) {
			// Send progress to SSE clients
			c.broadcastProgress(folderID, progress)
		})
	}

	if streamErr != nil {
		log.Printf("❌ ZIP 스트리밍 실패: %v", streamErr)
		// Note: Cannot send error response as headers are already sent
		return
	}

	log.Printf("✅ ZIP 다운로드 완료: %s", folderName)
}

// GetDownloadProgress handles SSE progress streaming
func (c *FolderDownloadController) GetDownloadProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		http.Error(w, "폴더 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create client channel
	clientChan := make(chan *usecases.DownloadProgress, 10)
	clientID := fmt.Sprintf("%s_%d", folderID, time.Now().UnixNano())

	// Register client
	c.sseClientsMu.Lock()
	c.sseClients[clientID] = clientChan
	c.sseClientsMu.Unlock()

	// Cleanup on disconnect
	defer func() {
		c.sseClientsMu.Lock()
		delete(c.sseClients, clientID)
		c.sseClientsMu.Unlock()
		close(clientChan)
	}()

	log.Printf("📡 SSE 클라이언트 연결: %s", clientID)

	// Send initial connection message
	fmt.Fprintf(w, "event: connected\ndata: {\"clientId\":\"%s\"}\n\n", clientID)
	flusher.Flush()

	// Listen for progress updates or client disconnect
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			log.Printf("📡 SSE 클라이언트 연결 종료: %s", clientID)
			return
		case progress, ok := <-clientChan:
			if !ok {
				return
			}

			// Only send progress for the requested folder
			if progress.FolderID != folderID {
				continue
			}

			data, err := json.Marshal(progress)
			if err != nil {
				log.Printf("⚠️ 진행 상황 JSON 변환 실패: %v", err)
				continue
			}

			fmt.Fprintf(w, "event: progress\ndata: %s\n\n", string(data))
			flusher.Flush()

			// Close connection if download completed or failed
			if progress.Status == "completed" || progress.Status == "failed" || progress.Status == "cancelled" {
				fmt.Fprintf(w, "event: done\ndata: {\"status\":\"%s\"}\n\n", progress.Status)
				flusher.Flush()
				return
			}
		}
	}
}

// CancelDownload handles download cancellation
func (c *FolderDownloadController) CancelDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FolderID   string `json:"folderId"`
		DownloadID string `json:"downloadId"`
	}

	// Try to get from query params first
	req.FolderID = r.URL.Query().Get("folderId")
	req.DownloadID = r.URL.Query().Get("downloadId")

	// Try to get from body
	if req.FolderID == "" && req.DownloadID == "" {
		json.NewDecoder(r.Body).Decode(&req)
	}

	if req.FolderID == "" && req.DownloadID == "" {
		http.Error(w, "폴더 ID 또는 다운로드 ID가 필요합니다", http.StatusBadRequest)
		return
	}

	var err error
	if req.DownloadID != "" {
		err = c.folderDownloadUseCase.CancelDownload(req.DownloadID)
	} else {
		err = c.folderDownloadUseCase.CancelDownloadByFolderID(req.FolderID)
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "다운로드가 취소되었습니다",
	})
}

// GetActiveDownloads returns all active downloads
func (c *FolderDownloadController) GetActiveDownloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	downloads := c.folderDownloadUseCase.GetAllActiveDownloads()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"downloads": downloads,
		"count":     len(downloads),
	})
}

// broadcastProgress sends progress to all SSE clients for a folder
func (c *FolderDownloadController) broadcastProgress(folderID string, progress *usecases.DownloadProgress) {
	c.sseClientsMu.RLock()
	defer c.sseClientsMu.RUnlock()

	for _, clientChan := range c.sseClients {
		select {
		case clientChan <- progress:
		default:
			// Skip if channel is full
		}
	}
}

// StartPrepareDownload starts download preparation in background and returns prepareId
func (c *FolderDownloadController) StartPrepareDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PrepareDownloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderID := req.FolderID

	// Extract folder ID from URL if provided
	if folderID == "" && req.URL != "" {
		extractedID, err := extractDownloadFolderIDFromURL(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		folderID = extractedID
	}

	if folderID == "" {
		http.Error(w, "폴더 ID 또는 URL이 필요합니다", http.StatusBadRequest)
		return
	}

	// Generate prepare ID
	prepareId := fmt.Sprintf("%s_%d", folderID, time.Now().UnixNano())

	// Initialize progress state
	c.prepareProgressMu.Lock()
	c.prepareProgress[prepareId] = &PrepareProgressState{
		Status: "preparing",
		Progress: &usecases.PrepareProgress{
			Status:  "scanning",
			Phase:   "starting",
			Message: "다운로드 준비 시작...",
		},
	}
	c.prepareProgressMu.Unlock()

	log.Printf("📦 다운로드 준비 시작 (폴링): 폴더 ID %s, prepareId %s", folderID, prepareId)

	// Start preparation in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		options := &usecases.DownloadOptions{
			IncludeSubfolders:   req.IncludeSubfolders,
			SkipGoogleDocs:      req.SkipGoogleDocs,
			ConcurrentDownloads: 3,
		}

		result, err := c.folderDownloadUseCase.PrepareDownloadWithProgress(ctx, folderID, options, func(progress *usecases.PrepareProgress) {
			c.prepareProgressMu.Lock()
			if state, ok := c.prepareProgress[prepareId]; ok {
				state.Progress = progress
			}
			c.prepareProgressMu.Unlock()
		})

		c.prepareProgressMu.Lock()
		if state, ok := c.prepareProgress[prepareId]; ok {
			if err != nil {
				state.Status = "failed"
				state.Error = err.Error()
				log.Printf("❌ 다운로드 준비 실패 (폴링): %v", err)
			} else {
				state.Status = "completed"
				state.Result = result
				log.Printf("✅ 다운로드 준비 완료 (폴링): %s", folderID)
			}
		}
		c.prepareProgressMu.Unlock()

		// Clean up after 5 minutes
		time.AfterFunc(5*time.Minute, func() {
			c.prepareProgressMu.Lock()
			delete(c.prepareProgress, prepareId)
			c.prepareProgressMu.Unlock()
		})
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"prepareId": prepareId,
		"folderId":  folderID,
		"message":   "다운로드 준비가 시작되었습니다",
	})
}

// GetPrepareProgress returns the progress of a prepare operation
func (c *FolderDownloadController) GetPrepareProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	prepareId := r.URL.Query().Get("prepareId")
	if prepareId == "" {
		http.Error(w, "prepareId가 필요합니다", http.StatusBadRequest)
		return
	}

	log.Printf("📡 준비 진행 상황 조회: prepareId=%s", prepareId)

	c.prepareProgressMu.RLock()
	state, ok := c.prepareProgress[prepareId]
	c.prepareProgressMu.RUnlock()

	if !ok {
		log.Printf("⚠️ 진행 상황을 찾을 수 없음: prepareId=%s", prepareId)
		http.Error(w, "진행 상황을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	// 디버그 로깅
	if state.Progress != nil {
		log.Printf("📊 준비 진행 상황: status=%s, phase=%s, scannedFolders=%d, scannedFiles=%d",
			state.Status, state.Progress.Phase, state.Progress.ScannedFolders, state.Progress.ScannedFiles)
	} else {
		log.Printf("📊 준비 상태: status=%s (progress=nil)", state.Status)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// ================================================================================
// 서버 사이드 다운로드 엔드포인트
// ================================================================================

// StartServerDownload starts downloading files to server and creating ZIP
func (c *FolderDownloadController) StartServerDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		PrepareID string `json:"prepareId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.PrepareID == "" {
		http.Error(w, "prepareId가 필요합니다", http.StatusBadRequest)
		return
	}

	// Get cached prepare result
	c.prepareProgressMu.RLock()
	prepareState, ok := c.prepareProgress[req.PrepareID]
	c.prepareProgressMu.RUnlock()

	if !ok {
		http.Error(w, "준비 정보를 찾을 수 없습니다. 다시 준비해주세요.", http.StatusNotFound)
		return
	}

	if prepareState.Status != "completed" || prepareState.Result == nil {
		http.Error(w, "다운로드 준비가 완료되지 않았습니다", http.StatusBadRequest)
		return
	}

	// Generate download ID
	downloadId := fmt.Sprintf("server_dl_%s_%d", prepareState.Result.FolderID, time.Now().UnixNano())

	// Initialize server download state
	c.serverDownloadsMu.Lock()
	c.serverDownloads[downloadId] = &ServerDownloadState{
		Status: "downloading",
		Progress: &usecases.ServerDownloadProgress{
			ID:         downloadId,
			FolderID:   prepareState.Result.FolderID,
			FolderName: prepareState.Result.FolderName,
			Status:     "downloading",
			Phase:      "download",
			TotalFiles: len(prepareState.Result.Files),
			TotalSize:  prepareState.Result.TotalSize,
			StartTime:  time.Now(),
			Message:    "서버 다운로드 시작...",
		},
	}
	c.serverDownloadsMu.Unlock()

	log.Printf("📥 서버 다운로드 시작: %s (%d개 파일)", prepareState.Result.FolderName, len(prepareState.Result.Files))

	// Start download in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute) // 60분 타임아웃
		defer cancel()

		options := usecases.DefaultDownloadOptions()

		result, err := c.folderDownloadUseCase.StartServerDownload(ctx, prepareState.Result, options, func(progress *usecases.ServerDownloadProgress) {
			c.serverDownloadsMu.Lock()
			if state, ok := c.serverDownloads[downloadId]; ok {
				state.Progress = progress
				state.Status = progress.Status
				// 디버그: 진행 상황 업데이트 로깅
				log.Printf("📊 진행 상황 업데이트: %d/%d 파일, 상태=%s, 단계=%s",
					progress.DownloadedFiles, progress.TotalFiles, progress.Status, progress.Phase)
			}
			c.serverDownloadsMu.Unlock()
		})

		c.serverDownloadsMu.Lock()
		if state, ok := c.serverDownloads[downloadId]; ok {
			if err != nil {
				state.Status = "failed"
				state.Error = err.Error()
				log.Printf("❌ 서버 다운로드 실패: %v", err)
			} else {
				state.Status = "completed"
				state.Result = result
				log.Printf("✅ 서버 다운로드 완료: %s", result.ZipPath)
			}
		}
		c.serverDownloadsMu.Unlock()

		// Clean up after 30 minutes
		time.AfterFunc(30*time.Minute, func() {
			c.serverDownloadsMu.Lock()
			if state, ok := c.serverDownloads[downloadId]; ok {
				if state.Result != nil && state.Result.ZipPath != "" {
					os.Remove(state.Result.ZipPath)
					log.Printf("🗑️ ZIP 파일 자동 정리: %s", state.Result.ZipPath)
				}
				delete(c.serverDownloads, downloadId)
			}
			c.serverDownloadsMu.Unlock()
		})
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"downloadId": downloadId,
		"message":    "서버 다운로드가 시작되었습니다",
	})
}

// GetServerDownloadProgress returns the progress of a server download
func (c *FolderDownloadController) GetServerDownloadProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	downloadId := r.URL.Query().Get("downloadId")
	if downloadId == "" {
		http.Error(w, "downloadId가 필요합니다", http.StatusBadRequest)
		return
	}

	c.serverDownloadsMu.RLock()
	state, ok := c.serverDownloads[downloadId]
	c.serverDownloadsMu.RUnlock()

	if !ok {
		http.Error(w, "다운로드 정보를 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	// 디버그: API 응답 로깅
	if state.Progress != nil {
		log.Printf("📡 진행 상황 API 응답: status=%s, phase=%s, downloaded=%d/%d",
			state.Status, state.Progress.Phase, state.Progress.DownloadedFiles, state.Progress.TotalFiles)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

// DownloadCompletedZip serves the completed ZIP file for download
func (c *FolderDownloadController) DownloadCompletedZip(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	downloadId := r.URL.Query().Get("downloadId")
	if downloadId == "" {
		http.Error(w, "downloadId가 필요합니다", http.StatusBadRequest)
		return
	}

	c.serverDownloadsMu.RLock()
	state, ok := c.serverDownloads[downloadId]
	c.serverDownloadsMu.RUnlock()

	if !ok {
		http.Error(w, "다운로드 정보를 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	if state.Status != "completed" || state.Result == nil {
		http.Error(w, "다운로드가 아직 완료되지 않았습니다", http.StatusBadRequest)
		return
	}

	zipPath := state.Result.ZipPath
	if zipPath == "" {
		http.Error(w, "ZIP 파일을 찾을 수 없습니다", http.StatusNotFound)
		return
	}

	// Open ZIP file
	file, err := os.Open(zipPath)
	if err != nil {
		log.Printf("❌ ZIP 파일 열기 실패: %v", err)
		http.Error(w, "ZIP 파일을 열 수 없습니다", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file info
	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "파일 정보를 가져올 수 없습니다", http.StatusInternalServerError)
		return
	}

	// Set headers
	fileName := fmt.Sprintf("%s.zip", state.Result.FolderName)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", stat.Size()))
	w.Header().Set("Cache-Control", "no-cache")

	log.Printf("📥 ZIP 파일 전송 시작: %s (%d bytes)", fileName, stat.Size())

	// Stream file to response
	written, err := io.Copy(w, file)
	if err != nil {
		log.Printf("⚠️ ZIP 파일 전송 중 오류: %v", err)
		return
	}

	log.Printf("✅ ZIP 파일 전송 완료: %s (%d bytes)", fileName, written)
}

// CleanupServerDownload cleans up the ZIP file after download
func (c *FolderDownloadController) CleanupServerDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DownloadID string `json:"downloadId"`
	}

	// Try query param first
	req.DownloadID = r.URL.Query().Get("downloadId")
	if req.DownloadID == "" {
		json.NewDecoder(r.Body).Decode(&req)
	}

	if req.DownloadID == "" {
		http.Error(w, "downloadId가 필요합니다", http.StatusBadRequest)
		return
	}

	c.serverDownloadsMu.Lock()
	state, ok := c.serverDownloads[req.DownloadID]
	if ok {
		if state.Result != nil && state.Result.ZipPath != "" {
			zipPath := state.Result.ZipPath
			if err := c.folderDownloadUseCase.CleanupZipFile(zipPath); err != nil {
				log.Printf("⚠️ ZIP 파일 정리 실패: %v", err)
			}
		}
		delete(c.serverDownloads, req.DownloadID)
	}
	c.serverDownloadsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "정리 완료",
	})
}
