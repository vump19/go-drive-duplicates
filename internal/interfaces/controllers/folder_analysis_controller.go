package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/usecases"
	"log"
	"net/http"
	"regexp"
	"sync"
	"time"
)

// FolderAnalysisController handles folder analysis HTTP requests
type FolderAnalysisController struct {
	folderAnalysisUseCase *usecases.FolderAnalysisUseCase

	// 취소 기능을 위한 맵 (folderID -> cancel function)
	cancelFuncs   map[string]context.CancelFunc
	cancelFuncsMu sync.Mutex
}

// NewFolderAnalysisController creates a new folder analysis controller
func NewFolderAnalysisController(folderAnalysisUseCase *usecases.FolderAnalysisUseCase) *FolderAnalysisController {
	return &FolderAnalysisController{
		folderAnalysisUseCase: folderAnalysisUseCase,
		cancelFuncs:           make(map[string]context.CancelFunc),
	}
}

// AnalyzeFolderRequest represents the request to analyze a folder
type AnalyzeFolderRequest struct {
	FolderID           string   `json:"folderId"`
	URL                string   `json:"url,omitempty"`
	ExcludeFolderNames []string `json:"excludeFolderNames,omitempty"` // 제외할 폴더명 패턴
}

// DeleteEmptyFoldersRequest represents the request to delete empty folders
type DeleteEmptyFoldersRequest struct {
	FolderIDs          []string `json:"folderIds"`
	ExcludeFolderNames []string `json:"excludeFolderNames,omitempty"` // 분석 시 사용된 제외 패턴 (제외된 폴더도 함께 삭제)
	PermanentDelete    bool     `json:"permanentDelete,omitempty"`    // true: 완전 삭제, false: 휴지통으로 이동 (기본값)
}

// AnalyzeFolder handles folder analysis requests
func (c *FolderAnalysisController) AnalyzeFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderID := req.FolderID

	// Extract folder ID from URL if provided instead of direct ID
	if folderID == "" && req.URL != "" {
		extractedID, err := extractFolderIDFromURL(req.URL)
		if err != nil {
			http.Error(w, "Invalid Google Drive URL", http.StatusBadRequest)
			return
		}
		folderID = extractedID
	}

	if folderID == "" {
		http.Error(w, "Folder ID or URL is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout and cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)

	// 취소 함수 등록
	c.cancelFuncsMu.Lock()
	c.cancelFuncs[folderID] = cancel
	c.cancelFuncsMu.Unlock()

	// 완료 후 취소 함수 제거
	defer func() {
		c.cancelFuncsMu.Lock()
		delete(c.cancelFuncs, folderID)
		c.cancelFuncsMu.Unlock()
		cancel()
	}()

	// 제외 폴더 로깅
	if len(req.ExcludeFolderNames) > 0 {
		log.Printf("📊 폴더 분석 시작 (취소 가능): %s (제외 폴더: %v)", folderID, req.ExcludeFolderNames)
	} else {
		log.Printf("📊 폴더 분석 시작 (취소 가능): %s", folderID)
	}

	// Analyze folder with exclude patterns
	result, err := c.folderAnalysisUseCase.AnalyzeFolderWithExcludes(ctx, folderID, req.ExcludeFolderNames)
	if err != nil {
		// 취소된 경우 특별한 응답
		if ctx.Err() == context.Canceled {
			log.Printf("⚠️ 폴더 분석 취소됨: %s", folderID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestTimeout)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"cancelled": true,
				"message":   "폴더 분석이 취소되었습니다",
			})
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 버퍼에 먼저 인코딩하여 오류 확인
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(result); err != nil {
		log.Printf("❌ JSON 인코딩 실패: %v (빈 폴더 수: %d)", err, len(result.EmptyFolders))
		http.Error(w, fmt.Sprintf("JSON 인코딩 실패: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("📤 폴더 분석 결과 전송 중: %d 바이트, 빈 폴더 %d개", buf.Len(), len(result.EmptyFolders))

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("❌ HTTP 응답 쓰기 실패: %v", err)
		// 이미 일부 데이터가 전송되었을 수 있으므로 http.Error 사용 불가
		return
	}
}

// CancelFolderAnalysis handles cancellation of folder analysis
func (c *FolderAnalysisController) CancelFolderAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderID := r.URL.Query().Get("folderId")
	if folderID == "" {
		// JSON body에서 folderID 읽기 시도
		var req struct {
			FolderID string `json:"folderId"`
			URL      string `json:"url,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			folderID = req.FolderID
			if folderID == "" && req.URL != "" {
				extractedID, err := extractFolderIDFromURL(req.URL)
				if err == nil {
					folderID = extractedID
				}
			}
		}
	}

	if folderID == "" {
		http.Error(w, "Folder ID is required", http.StatusBadRequest)
		return
	}

	// 취소 함수 찾아서 호출
	c.cancelFuncsMu.Lock()
	cancel, exists := c.cancelFuncs[folderID]
	if exists {
		cancel()
		delete(c.cancelFuncs, folderID)
	}
	c.cancelFuncsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if exists {
		log.Printf("🛑 폴더 분석 취소 요청 처리됨: %s", folderID)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "폴더 분석이 취소되었습니다",
		})
	} else {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "진행 중인 분석이 없습니다",
		})
	}
}

// DeleteEmptyFolderRequest represents the request to delete a single empty folder
type DeleteEmptyFolderRequest struct {
	FolderID           string   `json:"folderId"`
	ExcludeFolderNames []string `json:"excludeFolderNames,omitempty"`
	PermanentDelete    bool     `json:"permanentDelete,omitempty"` // true: 완전 삭제, false: 휴지통으로 이동 (기본값)
}

// DeleteEmptyFolder handles single empty folder deletion
func (c *FolderAnalysisController) DeleteEmptyFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Query parameter에서 폴더 ID 읽기 (기존 호환성)
	folderID := r.URL.Query().Get("id")
	var excludeFolderNames []string
	var permanentDelete bool

	// JSON body가 있으면 읽기 (새로운 방식)
	if r.Body != nil && r.ContentLength > 0 {
		var req DeleteEmptyFolderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.FolderID != "" {
				folderID = req.FolderID
			}
			excludeFolderNames = req.ExcludeFolderNames
			permanentDelete = req.PermanentDelete
		}
	}

	if folderID == "" {
		http.Error(w, "Folder ID is required", http.StatusBadRequest)
		return
	}

	// 삭제 방식 결정
	deleteMethod := "휴지통 이동"
	if permanentDelete {
		deleteMethod = "완전 삭제"
	}

	// 디버그 로그
	log.Printf("🗑️ 빈 폴더 삭제 요청: folderID=%s, excludeFolderNames=%v, 삭제방식=%s", folderID, excludeFolderNames, deleteMethod)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Delete empty folder with options
	err := c.folderAnalysisUseCase.DeleteEmptyFolderWithOptions(ctx, folderID, excludeFolderNames, permanentDelete)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "빈 폴더가 삭제되었습니다",
	})
}

// DeleteAllEmptyFolders handles multiple empty folders deletion with SSE progress
func (c *FolderAnalysisController) DeleteAllEmptyFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeleteEmptyFoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if len(req.FolderIDs) == 0 {
		http.Error(w, "Folder IDs are required", http.StatusBadRequest)
		return
	}

	// 삭제 방식 로깅
	deleteMethod := "휴지통 이동"
	if req.PermanentDelete {
		deleteMethod = "완전 삭제"
	}
	log.Printf("🗑️ 빈 폴더 삭제 요청: %d개 폴더, 삭제방식=%s", len(req.FolderIDs), deleteMethod)

	// Check if client wants SSE
	acceptHeader := r.Header.Get("Accept")
	useSSE := acceptHeader == "text/event-stream"

	if useSSE {
		c.deleteEmptyFoldersWithSSE(w, r, req)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Delete all empty folders with options
	successCount, err := c.folderAnalysisUseCase.DeleteAllEmptyFoldersWithOptions(ctx, req.FolderIDs, req.ExcludeFolderNames, req.PermanentDelete)

	response := map[string]interface{}{
		"successCount": successCount,
		"totalCount":   len(req.FolderIDs),
		"message":      "빈 폴더 삭제가 완료되었습니다",
	}

	if err != nil {
		response["warning"] = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// deleteEmptyFoldersWithSSE handles deletion with Server-Sent Events for progress
func (c *FolderAnalysisController) deleteEmptyFoldersWithSSE(w http.ResponseWriter, r *http.Request, req DeleteEmptyFoldersRequest) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx 버퍼링 비활성화

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Use background context for deletion operations - should not be canceled when client disconnects
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	// Note: Don't defer cancel() here - we want deletions to complete even if client disconnects

	// 클라이언트 연결 끊김 감지
	clientGone := r.Context().Done()
	clientDisconnected := false

	totalCount := len(req.FolderIDs)
	successCount := 0
	failedCount := 0
	cascadeDeletedCount := 0

	// Track parent folders for cascade deletion
	var parentIDsMu sync.Mutex
	parentIDsToCheck := make(map[string]bool)
	deletedIDs := make(map[string]bool)

	// 진행 상황을 안전하게 전송하는 뮤텍스
	var sendMu sync.Mutex
	sendProgress := func(event string, data map[string]interface{}) bool {
		sendMu.Lock()
		defer sendMu.Unlock()

		select {
		case <-clientGone:
			log.Printf("⚠️ 클라이언트 연결 끊김, SSE 전송 중단")
			return false
		default:
			sendSSEEvent(w, flusher, event, data)
			return true
		}
	}

	// Keep-alive 고루틴 시작
	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-clientGone:
				return
			case <-ticker.C:
				sendMu.Lock()
				// SSE 주석 형식으로 heartbeat 전송 (클라이언트에서 무시됨)
				fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
				flusher.Flush()
				sendMu.Unlock()
			}
		}
	}()

	// Send initial progress
	if !sendProgress("progress", map[string]interface{}{
		"completed": 0,
		"total":     totalCount,
		"current":   "삭제 시작...",
	}) {
		return
	}

	// 병렬 삭제를 위한 결과 채널
	type deleteResult struct {
		folderID string
		parentID string
		success  bool
		err      error
	}
	resultChan := make(chan deleteResult, totalCount)

	// 동시 처리 수 제한 (최대 5개)
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)

	// 모든 폴더에 대해 고루틴 시작
	var wg sync.WaitGroup
	for _, folderID := range req.FolderIDs {
		wg.Add(1)
		go func(fID string) {
			defer wg.Done()

			// Acquire semaphore - don't check clientGone here, continue deletion anyway
			sem <- struct{}{}
			defer func() { <-sem }()

			// Get parent ID before deletion for cascade check
			parentID, _ := c.folderAnalysisUseCase.GetParentFolderID(ctx, fID)

			// Delete with options (permanentDelete 적용)
			// Continue even if client disconnected - deletion should complete
			err := c.folderAnalysisUseCase.DeleteEmptyFolderWithOptions(ctx, fID, req.ExcludeFolderNames, req.PermanentDelete)

			if err != nil {
				log.Printf("❌ 폴더 삭제 실패 (%s): %v", fID, err)
				resultChan <- deleteResult{folderID: fID, parentID: "", success: false, err: err}
			} else {
				resultChan <- deleteResult{folderID: fID, parentID: parentID, success: true, err: nil}
			}
		}(folderID)
	}

	// 결과 수집 고루틴
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 결과 수신 및 진행 상황 업데이트
	completedCount := 0
	for result := range resultChan {
		completedCount++
		if result.success {
			successCount++
			// Track parent folder for cascade check
			if result.parentID != "" {
				parentIDsMu.Lock()
				parentIDsToCheck[result.parentID] = true
				deletedIDs[result.folderID] = true
				parentIDsMu.Unlock()
			}
		} else {
			failedCount++
		}

		// Check if client disconnected - stop sending SSE but continue deletion
		if clientDisconnected {
			continue
		}

		// 클라이언트 연결 확인
		select {
		case <-clientGone:
			log.Printf("⚠️ 클라이언트 연결 끊김, 진행 상황 업데이트 중단 (%d/%d 완료) - 삭제 작업은 계속 진행됩니다", completedCount, totalCount)
			clientDisconnected = true
			continue
		default:
		}

		// Send progress update
		if !sendProgress("progress", map[string]interface{}{
			"completed": completedCount,
			"total":     totalCount,
			"success":   successCount,
			"failed":    failedCount,
			"current":   fmt.Sprintf("%d/%d 처리 중... (동시 %d개)", completedCount, totalCount, maxConcurrent),
		}) {
			clientDisconnected = true
			continue
		}
	}

	// Cascade deletion: check and delete parent folders that became empty
	if len(parentIDsToCheck) > 0 {
		log.Printf("🔄 연쇄 삭제 시작: %d개 부모 폴더 확인 중...", len(parentIDsToCheck))

		if !clientDisconnected {
			sendProgress("progress", map[string]interface{}{
				"completed": completedCount,
				"total":     totalCount,
				"success":   successCount,
				"failed":    failedCount,
				"current":   "연쇄 삭제 중: 부모 폴더 확인...",
			})
		}

		cascadeDeletedCount = c.folderAnalysisUseCase.CascadeDeleteEmptyParents(ctx, parentIDsToCheck, deletedIDs, req.ExcludeFolderNames, req.PermanentDelete)

		if cascadeDeletedCount > 0 {
			log.Printf("🔄 연쇄 삭제 완료: 추가 %d개 부모 폴더 삭제됨", cascadeDeletedCount)
			successCount += cascadeDeletedCount
		}
	}

	// Cancel context after all deletions complete
	cancel()

	// Log final result
	log.Printf("✅ 빈 폴더 삭제 작업 완료: %d개 성공 (초기: %d, 연쇄: %d), %d개 실패",
		successCount, successCount-cascadeDeletedCount, cascadeDeletedCount, failedCount)

	// Send completion event only if client still connected
	if !clientDisconnected {
		sendProgress("complete", map[string]interface{}{
			"successCount":        successCount,
			"failedCount":         failedCount,
			"totalCount":          totalCount,
			"cascadeDeletedCount": cascadeDeletedCount,
			"message":             fmt.Sprintf("빈 폴더 삭제 완료 (연쇄 삭제: %d개)", cascadeDeletedCount),
		})
	}
}

// sendSSEEvent sends a Server-Sent Event
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}

// CleanupEmptySubfolders handles cleanup of empty subfolders
func (c *FolderAnalysisController) CleanupEmptySubfolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req usecases.CleanupEmptySubfoldersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderID := req.ParentFolderID

	// Extract folder ID from URL if provided instead of direct ID
	if folderID == "" && req.URL != "" {
		extractedID, err := extractFolderIDFromURL(req.URL)
		if err != nil {
			http.Error(w, "Invalid Google Drive URL", http.StatusBadRequest)
			return
		}
		folderID = extractedID
		req.ParentFolderID = folderID
	}

	if folderID == "" {
		http.Error(w, "Parent folder ID or URL is required", http.StatusBadRequest)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Cleanup empty subfolders
	result, err := c.folderAnalysisUseCase.CleanupEmptySubfolders(ctx, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 버퍼에 먼저 인코딩하여 오류 확인
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(result); err != nil {
		log.Printf("❌ JSON 인코딩 실패: %v (삭제된 폴더: %d개)", err, result.DeletedCount)
		http.Error(w, fmt.Sprintf("JSON 인코딩 실패: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("📤 정리 결과 전송 중: %d 바이트, 삭제 %d개, 건너뜀 %d개",
		buf.Len(), result.DeletedCount, result.SkippedCount)

	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(buf.Bytes()); err != nil {
		log.Printf("❌ HTTP 응답 쓰기 실패: %v", err)
		return
	}
}

// extractFolderIDFromURL extracts folder ID from Google Drive URL
func extractFolderIDFromURL(url string) (string, error) {
	if url == "" {
		return "", fmt.Errorf("URL is empty")
	}

	// Handle different Google Drive URL formats
	patterns := []string{
		`/folders/([a-zA-Z0-9_-]+)`,             // https://drive.google.com/drive/folders/FOLDER_ID
		`[?&]id=([a-zA-Z0-9_-]+)`,               // https://drive.google.com/open?id=FOLDER_ID
		`/drive/u/\d+/folders/([a-zA-Z0-9_-]+)`, // https://drive.google.com/drive/u/0/folders/FOLDER_ID
	}

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		matches := re.FindStringSubmatch(url)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("could not extract folder ID from URL: %s", url)
}

// FindFoldersByNameRequest represents the request to find folders by name
type FindFoldersByNameRequest struct {
	FolderID   string `json:"folderId"`
	URL        string `json:"url,omitempty"`
	FolderName string `json:"folderName"` // 검색할 폴더명 (정확히 일치)
	SkipStats  bool   `json:"skipStats"`  // 통계 계산 생략 (빠른 검색)
}

// FindFoldersByName handles finding folders by name pattern
func (c *FolderAnalysisController) FindFoldersByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FindFoldersByNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderID := req.FolderID

	// Extract folder ID from URL if provided instead of direct ID
	if folderID == "" && req.URL != "" {
		extractedID, err := extractFolderIDFromURL(req.URL)
		if err != nil {
			http.Error(w, "Invalid Google Drive URL", http.StatusBadRequest)
			return
		}
		folderID = extractedID
	}

	if folderID == "" {
		http.Error(w, "Folder ID or URL is required", http.StatusBadRequest)
		return
	}

	if req.FolderName == "" {
		http.Error(w, "Folder name to search is required", http.StatusBadRequest)
		return
	}

	skipStatsText := ""
	if req.SkipStats {
		skipStatsText = ", 통계생략"
	}
	log.Printf("🔍 폴더명 검색 요청 (SSE): 루트=%s, 검색어='%s'%s", folderID, req.FolderName, skipStatsText)

	// SSE 설정
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx 버퍼링 비활성화

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Use background context for search operations - should not be canceled when client disconnects
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// 클라이언트 연결 끊김 감지
	clientGone := r.Context().Done()

	// Keep-alive 고루틴 시작
	done := make(chan struct{})
	defer close(done)

	var sendMu sync.Mutex

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-clientGone:
				return
			case <-ticker.C:
				sendMu.Lock()
				// SSE 주석 형식으로 heartbeat 전송 (클라이언트에서 무시됨)
				fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
				flusher.Flush()
				sendMu.Unlock()
			}
		}
	}()

	// Progress callback for SSE
	progressCallback := func(progress usecases.FolderSearchProgress) {
		sendMu.Lock()
		defer sendMu.Unlock()

		// 클라이언트 연결 확인
		select {
		case <-clientGone:
			return
		default:
		}

		data, err := json.Marshal(progress)
		if err != nil {
			log.Printf("⚠️ SSE 데이터 직렬화 실패: %v", err)
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	// Find folders by name with SSE callback (with panic recovery)
	var searchErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("🔴 폴더명 검색 중 패닉 복구: %v", r)
				searchErr = fmt.Errorf("검색 중 예기치 않은 오류: %v", r)
			}
		}()
		_, searchErr = c.folderAnalysisUseCase.FindFoldersByNameWithCallback(ctx, folderID, req.FolderName, req.SkipStats, progressCallback)
	}()

	if searchErr != nil {
		log.Printf("❌ 폴더명 검색 실패: %v", searchErr)
		sendMu.Lock()
		errorData, _ := json.Marshal(map[string]string{"type": "error", "error": searchErr.Error()})
		fmt.Fprintf(w, "data: %s\n\n", errorData)
		flusher.Flush()
		sendMu.Unlock()
		return
	}

	log.Printf("✅ 폴더명 검색 SSE 스트림 완료")
}

// DeleteFoldersByNameRequest represents the request to delete folders by name
type DeleteFoldersByNameRequest struct {
	FolderIDs       []string `json:"folderIds"`
	PermanentDelete bool     `json:"permanentDelete,omitempty"`
}

// DeleteFoldersByName handles deletion of folders by their IDs (with all contents)
func (c *FolderAnalysisController) DeleteFoldersByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DeleteFoldersByNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if len(req.FolderIDs) == 0 {
		http.Error(w, "Folder IDs are required", http.StatusBadRequest)
		return
	}

	deleteMethod := "휴지통 이동"
	if req.PermanentDelete {
		deleteMethod = "완전 삭제"
	}
	log.Printf("🗑️ 특정 폴더명 삭제 요청: %d개 폴더 (%s)", len(req.FolderIDs), deleteMethod)

	// Check if client wants SSE
	acceptHeader := r.Header.Get("Accept")
	useSSE := acceptHeader == "text/event-stream"

	if useSSE {
		c.deleteFoldersByNameWithSSE(w, r, req)
		return
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Delete folders
	result, err := c.folderAnalysisUseCase.DeleteFoldersByName(ctx, req.FolderIDs, req.PermanentDelete)
	if err != nil {
		log.Printf("❌ 폴더 삭제 실패: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("✅ 폴더 삭제 완료: %d개 성공, %d개 실패", result.SuccessCount, result.FailedCount)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// deleteFoldersByNameWithSSE handles deletion with Server-Sent Events for progress
func (c *FolderAnalysisController) deleteFoldersByNameWithSSE(w http.ResponseWriter, r *http.Request, req DeleteFoldersByNameRequest) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Use background context for deletion operations - should not be canceled when client disconnects
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	// Note: Don't defer cancel() here - we want deletions to complete even if client disconnects

	clientGone := r.Context().Done()
	clientDisconnected := false

	totalCount := len(req.FolderIDs)
	successCount := 0
	failedCount := 0

	var sendMu sync.Mutex
	sendProgress := func(event string, data map[string]interface{}) bool {
		sendMu.Lock()
		defer sendMu.Unlock()

		select {
		case <-clientGone:
			log.Printf("⚠️ 클라이언트 연결 끊김, SSE 전송 중단")
			return false
		default:
			sendSSEEvent(w, flusher, event, data)
			return true
		}
	}

	// Keep-alive goroutine
	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-clientGone:
				return
			case <-ticker.C:
				sendMu.Lock()
				fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
				flusher.Flush()
				sendMu.Unlock()
			}
		}
	}()

	// Send initial progress
	if !sendProgress("progress", map[string]interface{}{
		"completed": 0,
		"total":     totalCount,
		"current":   "삭제 시작...",
	}) {
		return
	}

	// Delete folders one by one with progress updates
	type deleteResult struct {
		folderID string
		success  bool
		err      error
	}
	resultChan := make(chan deleteResult, totalCount)

	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for _, folderID := range req.FolderIDs {
		wg.Add(1)
		go func(fID string) {
			defer wg.Done()

			// Acquire semaphore - don't check clientGone here, continue deletion anyway
			sem <- struct{}{}
			defer func() { <-sem }()

			// Use internal method to delete folder with all contents
			// Continue even if client disconnected - deletion should complete
			_, err := c.folderAnalysisUseCase.DeleteFoldersByName(ctx, []string{fID}, req.PermanentDelete)

			if err != nil {
				log.Printf("❌ 폴더 삭제 실패 (%s): %v", fID, err)
				resultChan <- deleteResult{folderID: fID, success: false, err: fmt.Errorf("%v", err)}
			} else {
				resultChan <- deleteResult{folderID: fID, success: true, err: nil}
			}
		}(folderID)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	completedCount := 0
	for result := range resultChan {
		completedCount++
		if result.success {
			successCount++
		} else {
			failedCount++
		}

		// Check if client disconnected - stop sending SSE but continue deletion
		if clientDisconnected {
			continue
		}

		select {
		case <-clientGone:
			log.Printf("⚠️ 클라이언트 연결 끊김, 진행 상황 업데이트 중단 (%d/%d 완료) - 삭제 작업은 계속 진행됩니다", completedCount, totalCount)
			clientDisconnected = true
			continue
		default:
		}

		if !sendProgress("progress", map[string]interface{}{
			"completed": completedCount,
			"total":     totalCount,
			"success":   successCount,
			"failed":    failedCount,
			"current":   fmt.Sprintf("%d/%d 처리 중...", completedCount, totalCount),
		}) {
			clientDisconnected = true
			continue
		}
	}

	// Cancel context after all deletions complete
	cancel()

	// Log final result
	log.Printf("✅ 폴더 삭제 작업 완료: %d개 성공, %d개 실패 (총 %d개)", successCount, failedCount, totalCount)

	// Send completion event only if client still connected
	if !clientDisconnected {
		sendProgress("complete", map[string]interface{}{
			"successCount": successCount,
			"failedCount":  failedCount,
			"totalCount":   totalCount,
			"message":      "폴더 삭제가 완료되었습니다",
		})
	}
}
