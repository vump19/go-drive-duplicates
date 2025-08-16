package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"sync"
)

var (
	globalDriveService *DriveService
	serviceOnce        sync.Once
	serviceMutex       sync.RWMutex
	maxWorkers         int = 5    // 기본값 5개 워커
	workersMutex       sync.RWMutex
)

func main() {
	log.Println("=== Google Drive Duplicates Finder ===")
	log.Println("서버 초기화 중...")
	
	// 전역 DriveService 초기화
	initGlobalDriveService()
	
	// 정적 파일 서빙
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	
	// 핸들러 등록
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/scan", scanHandler)
	http.HandleFunc("/progress", progressHandler)
	http.HandleFunc("/reset", resetHandler)
	http.HandleFunc("/resume", resumeHandler)
	http.HandleFunc("/duplicates", duplicatesHandler)
	http.HandleFunc("/file-path", filePathHandler)
	http.HandleFunc("/update-parents", updateParentsHandler)
	http.HandleFunc("/search-files-to-delete", searchFilesToDeleteHandler)
	http.HandleFunc("/bulk-delete", bulkDeleteHandler)
	http.HandleFunc("/compare-folders", compareFoldersHandler)
	http.HandleFunc("/comparison-progress", comparisonProgressHandler)
	http.HandleFunc("/comparison-result", comparisonResultHandler)
	http.HandleFunc("/saved-comparison-tasks", savedComparisonTasksHandler)
	http.HandleFunc("/clear-saved-comparison", clearSavedComparisonHandler)
	http.HandleFunc("/delete-duplicates-from-target", deleteDuplicatesFromTargetHandler)
	http.HandleFunc("/delete-target-folder", deleteTargetFolderHandler)
	http.HandleFunc("/delete", deleteFileHandler)
	http.HandleFunc("/remove-duplicate-group", removeDuplicateGroupHandler)
	http.HandleFunc("/cleanup-deleted-files", cleanupDeletedFilesHandler)
	http.HandleFunc("/cleanup-empty-folders", cleanupEmptyFoldersHandler)
	http.HandleFunc("/settings", settingsHandler)
	http.HandleFunc("/set-workers", setWorkersHandler)
	
	log.Println("HTTP 핸들러 등록 완료")
	log.Println("서버 시작 중... 포트: 8080")
	log.Println("브라우저에서 http://localhost:8080 에 접속하세요")
	log.Println("서버를 중지하려면 Ctrl+C를 누르세요")
	
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func initGlobalDriveService() {
	serviceOnce.Do(func() {
		ctx := context.Background()
		service, err := NewDriveService(ctx)
		if err != nil {
			log.Printf("❌ 전역 DriveService 초기화 실패: %v", err)
			return
		}
		serviceMutex.Lock()
		globalDriveService = service
		serviceMutex.Unlock()
		log.Println("✅ 전역 DriveService 초기화 완료")
	})
}

func getGlobalDriveService() *DriveService {
	serviceMutex.RLock()
	defer serviceMutex.RUnlock()
	return globalDriveService
}

func getMaxWorkers() int {
	workersMutex.RLock()
	defer workersMutex.RUnlock()
	return maxWorkers
}

func setMaxWorkers(count int) {
	workersMutex.Lock()
	defer workersMutex.Unlock()
	if count >= 1 && count <= 20 {
		maxWorkers = count
		log.Printf("⚙️ 해시 계산 워커 수 설정: %d개", count)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("홈페이지 요청: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
	
	tmpl, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Printf("템플릿 파싱 오류: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	
	err = tmpl.Execute(w, nil)
	if err != nil {
		log.Printf("템플릿 실행 오류: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func scanHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	go func() {
		driveService := getGlobalDriveService()
		if driveService == nil {
			log.Printf("❌ DriveService가 초기화되지 않았습니다")
			return
		}
		
		_, err := driveService.startNewFileScanning()
		if err != nil {
			log.Printf("❌ 파일 스캔 실패: %v", err)
			return
		}
	}()
	
	response := map[string]string{"status": "스캔 시작됨"}
	json.NewEncoder(w).Encode(response)
}

func progressHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	progress, err := driveService.loadProgress()
	if err != nil {
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	var duplicates [][]*DriveFile
	if progress.Status == "completed" {
		duplicates, _ = driveService.loadDuplicateGroups()
	}
	
	response := map[string]interface{}{
		"progress":   progress,
		"duplicates": duplicates,
	}
	
	json.NewEncoder(w).Encode(response)
}

func resetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	err := driveService.clearAllData()
	if err != nil {
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := map[string]string{"status": "데이터 삭제 완료"}
	json.NewEncoder(w).Encode(response)
}

func resumeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	go func() {
		driveService := getGlobalDriveService()
		if driveService == nil {
			log.Printf("❌ DriveService가 초기화되지 않았습니다")
			return
		}
		
		progress, err := driveService.loadProgress()
		if err != nil {
			log.Printf("❌ 진행 상태 로드 실패: %v", err)
			return
		}
		
		if progress.Status == "running" {
			_, err = driveService.resumeFileScanning(progress)
			if err != nil {
				log.Printf("❌ 파일 스캔 재개 실패: %v", err)
				return
			}
		}
	}()
	
	response := map[string]string{"status": "작업 재개됨"}
	json.NewEncoder(w).Encode(response)
}

func settingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	settings := map[string]interface{}{
		"maxWorkers":     runtime.NumCPU(),
		"currentWorkers": getMaxWorkers(),
		"platform":       runtime.GOOS,
	}
	
	json.NewEncoder(w).Encode(settings)
}

func setWorkersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	workersStr := r.FormValue("workers")
	workers, err := strconv.Atoi(workersStr)
	if err != nil || workers < 1 || workers > 20 {
		response := map[string]string{"error": "작업자 수는 1-20 사이여야 합니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 워커 수 설정 저장
	setMaxWorkers(workers)
	
	response := map[string]interface{}{
		"status":  "설정 완료",
		"workers": workers,
		"message": fmt.Sprintf("해시 계산 워커 수가 %d개로 설정되었습니다", workers),
	}
	
	json.NewEncoder(w).Encode(response)
}

func updateParentsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	// 백그라운드에서 실행
	go func() {
		driveService := getGlobalDriveService()
		if driveService == nil {
			log.Printf("❌ DriveService가 초기화되지 않았습니다")
			return
		}
		
		err := driveService.updateAllFileParents()
		if err != nil {
			log.Printf("❌ parents 업데이트 실패: %v", err)
		} else {
			log.Printf("✅ parents 정보 업데이트 완료")
		}
	}()
	
	response := map[string]string{"status": "parents 정보 업데이트 시작됨"}
	json.NewEncoder(w).Encode(response)
}

func searchFilesToDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	var request struct {
		FolderID     string `json:"folderId"`
		RegexPattern string `json:"regexPattern"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response := map[string]string{"error": "잘못된 요청 형식"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	files, err := driveService.searchFilesInFolder(request.FolderID, request.RegexPattern)
	if err != nil {
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := map[string]interface{}{
		"files": files,
		"count": len(files),
	}
	
	json.NewEncoder(w).Encode(response)
}

func bulkDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	var request struct {
		FileIDs []string `json:"fileIds"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response := map[string]string{"error": "잘못된 요청 형식"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	deletedCount, err := driveService.bulkDeleteFiles(request.FileIDs)
	if err != nil {
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := map[string]interface{}{
		"deletedCount": deletedCount,
		"message":      fmt.Sprintf("%d개 파일이 성공적으로 삭제되었습니다", deletedCount),
	}
	
	json.NewEncoder(w).Encode(response)
}

func filePathHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		http.Error(w, "파일 ID가 필요합니다", http.StatusBadRequest)
		return
	}
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// API에서 파일 정보 조회 (삭제된 파일도 확인)
	fileInfo, err := driveService.service.Files.Get(fileID).Fields("id, name, parents, trashed").Do()
	if err != nil {
		// 파일이 존재하지 않는 경우
		log.Printf("⚠️ 파일 정보 조회 실패 (%s): %v", fileID, err)
		
		// 데이터베이스에서도 해당 파일 제거
		deleteErr := driveService.deleteFileFromDB(fileID)
		if deleteErr != nil {
			log.Printf("⚠️ DB에서 파일 삭제 실패: %v", deleteErr)
		}
		
		response := map[string]interface{}{
			"fileId": fileID,
			"status": "deleted",
			"message": "파일이 삭제되었거나 존재하지 않습니다",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 휴지통에 있는 파일인지 확인
	if fileInfo.Trashed {
		log.Printf("🗑️ 휴지통에 있는 파일: %s (%s)", fileInfo.Name, fileID)
		
		// 데이터베이스에서 해당 파일 제거
		deleteErr := driveService.deleteFileFromDB(fileID)
		if deleteErr != nil {
			log.Printf("⚠️ DB에서 파일 삭제 실패: %v", deleteErr)
		}
		
		response := map[string]interface{}{
			"fileId": fileID,
			"status": "trashed",
			"message": "파일이 휴지통에 있습니다",
			"name": fileInfo.Name,
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	var path string
	if len(fileInfo.Parents) > 0 {
		path = driveService.buildFullPath(fileInfo.Parents[0])
	} else {
		path = "/"
	}
	
	response := map[string]interface{}{
		"fileId": fileID,
		"status": "exists",
		"path":   path,
		"name":   fileInfo.Name,
	}
	
	json.NewEncoder(w).Encode(response)
}

func duplicatesHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("🔍 중복 그룹 요청: %s from %s", r.Method, r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	// 페이지네이션 파라미터
	page := 1
	limit := 20 // 한 번에 20개 그룹만 표시
	
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
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		log.Printf("❌ DriveService가 초기화되지 않았습니다")
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 페이지네이션과 함께 중복 그룹 로드
	duplicates, totalCount, err := driveService.loadDuplicateGroupsPaginated(page, limit)
	if err != nil {
		log.Printf("❌ 중복 그룹 로드 실패: %v", err)
		response := map[string]string{"error": "중복 그룹 조회 실패: " + err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	log.Printf("📊 중복 그룹 응답: %d개 그룹 (페이지 %d, 총 %d개)", len(duplicates), page, totalCount)
	
	if len(duplicates) > 0 {
		// 경로 정보 설정 (간단히)
		duplicates = driveService.enrichDuplicatesWithPaths(duplicates)
	}
	
	response := map[string]interface{}{
		"duplicates": duplicates,
		"page":       page,
		"limit":      limit,
		"total":      totalCount,
		"totalPages": (totalCount + limit - 1) / limit,
	}
	
	json.NewEncoder(w).Encode(response)
}

func deleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	fileID := r.FormValue("fileId")
	if fileID == "" {
		http.Error(w, "파일 ID가 필요합니다", http.StatusBadRequest)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 파일 삭제 전에 상위 폴더 정보 저장
	var parentFolders []string
	fileInfo, err := driveService.service.Files.Get(fileID).Fields("id,name,parents").Do()
	if err != nil {
		log.Printf("⚠️ 파일 정보 조회 실패 (%s): %v", fileID, err)
	} else {
		parentFolders = fileInfo.Parents
		log.Printf("🗑️ 파일 삭제: %s (%s)", fileInfo.Name, fileID)
	}
	
	// 파일 삭제
	err = driveService.service.Files.Delete(fileID).Do()
	if err != nil {
		response := map[string]string{"error": "파일 삭제 실패: " + err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	err = driveService.deleteFileFromDB(fileID)
	if err != nil {
		log.Printf("⚠️ DB에서 파일 삭제 실패: %v", err)
	}
	
	// 파일 삭제 후 상위 폴더들의 빈 상태 확인 및 삭제 (백그라운드에서 실행)
	if len(parentFolders) > 0 {
		go func() {
			for _, parentID := range parentFolders {
				err := driveService.checkAndDeleteEmptyFolder(parentID)
				if err != nil {
					log.Printf("⚠️ 빈 폴더 정리 실패: %v", err)
				}
			}
		}()
	}
	
	response := map[string]string{"status": "파일이 성공적으로 삭제되었습니다"}
	json.NewEncoder(w).Encode(response)
}

func compareFoldersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	var request struct {
		SourceFolderURL string `json:"sourceFolderUrl"`
		TargetFolderURL string `json:"targetFolderUrl"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response := map[string]string{"error": "잘못된 요청 형식"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// URL에서 폴더 ID 추출
	sourceFolderID := extractFolderIDFromURL(request.SourceFolderURL)
	targetFolderID := extractFolderIDFromURL(request.TargetFolderURL)
	
	if sourceFolderID == "" || targetFolderID == "" {
		response := map[string]string{"error": "올바른 Google Drive 폴더 URL을 입력해주세요"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	log.Printf("📂 폴더 비교 요청: 기준=%s, 대상=%s", sourceFolderID, targetFolderID)
	
	// 백그라운드에서 폴더 비교 실행
	go func() {
		result, err := driveService.compareFolders(sourceFolderID, targetFolderID)
		if err != nil {
			log.Printf("❌ 폴더 비교 실패: %v", err)
			// 에러 상태 업데이트
			updateComparisonProgress(func(p *FolderComparisonProgress) {
				p.Status = "error"
				p.ErrorMessage = err.Error()
			})
			return
		}
		
		log.Printf("✅ 폴더 비교 완료: %d개 중복 파일 발견", result.TotalDuplicates)
	}()
	
	// 즉시 응답 반환 (백그라운드 작업 시작됨을 알림)
	response := map[string]interface{}{
		"status": "started",
		"message": "폴더 비교가 시작되었습니다. 진행 상황을 확인하세요.",
	}
	json.NewEncoder(w).Encode(response)
}

func comparisonProgressHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	progress := getComparisonProgress()
	if progress == nil {
		response := map[string]interface{}{
			"status": "idle",
			"message": "비교 작업이 진행 중이지 않습니다",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	json.NewEncoder(w).Encode(progress)
}

func comparisonResultHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	result := getComparisonResult()
	if result == nil {
		response := map[string]interface{}{
			"error": "비교 결과가 없습니다",
		}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	json.NewEncoder(w).Encode(result)
}

func deleteDuplicatesFromTargetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	var request struct {
		FileIDs              []string `json:"fileIds"`
		CleanupEmptyFolders  bool     `json:"cleanupEmptyFolders"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response := map[string]string{"error": "잘못된 요청 형식"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	log.Printf("🗑️ 대상 폴더 중복 파일 삭제 시작: %d개 파일 (빈 폴더 정리: %v)", len(request.FileIDs), request.CleanupEmptyFolders)
	
	deletedCount, err := driveService.bulkDeleteFilesWithCleanup(request.FileIDs, request.CleanupEmptyFolders)
	if err != nil {
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	var message string
	if request.CleanupEmptyFolders {
		message = fmt.Sprintf("대상 폴더에서 %d개 중복 파일이 성공적으로 삭제되었습니다 (빈 폴더 정리 포함)", deletedCount)
	} else {
		message = fmt.Sprintf("대상 폴더에서 %d개 중복 파일이 성공적으로 삭제되었습니다", deletedCount)
	}
	
	response := map[string]interface{}{
		"deletedCount": deletedCount,
		"message":      message,
	}
	
	json.NewEncoder(w).Encode(response)
}

func savedComparisonTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 저장된 비교 작업 조회
	tasks, err := driveService.getSavedComparisonTasks()
	if err != nil {
		response := map[string]string{"error": "저장된 작업 조회 실패: " + err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := map[string]interface{}{
		"tasks": tasks,
		"count": len(tasks),
	}
	
	json.NewEncoder(w).Encode(response)
}

func clearSavedComparisonHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}
	
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	
	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	// 저장된 비교 작업 삭제
	err := driveService.clearSavedComparisonTasks()
	if err != nil {
		response := map[string]string{"error": "저장된 작업 삭제 실패: " + err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}
	
	response := map[string]string{
		"status": "success",
		"message": "저장된 폴더 비교 작업이 삭제되었습니다",
	}
	
	json.NewEncoder(w).Encode(response)
}

func removeDuplicateGroupHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var request struct {
		GroupHash string `json:"groupHash"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response := map[string]string{"error": "잘못된 요청 형식"}
		json.NewEncoder(w).Encode(response)
		return
	}

	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}

	err := driveService.removeDuplicateGroup(request.GroupHash)
	if err != nil {
		response := map[string]string{"error": err.Error()}
		json.NewEncoder(w).Encode(response)
		return
	}

	response := map[string]string{
		"status":  "success",
		"message": "중복 그룹이 목록에서 제거되었습니다",
	}

	json.NewEncoder(w).Encode(response)
}

func cleanupDeletedFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}

	// 백그라운드에서 삭제된 파일 정리 실행
	go func() {
		cleanedCount, err := driveService.cleanupDeletedFiles()
		if err != nil {
			log.Printf("❌ 삭제된 파일 정리 실패: %v", err)
		} else {
			log.Printf("✅ 삭제된 파일 정리 완료: %d개 파일 제거됨", cleanedCount)
		}
	}()

	response := map[string]string{
		"status":  "started",
		"message": "삭제된 파일 정리가 백그라운드에서 시작되었습니다",
	}

	json.NewEncoder(w).Encode(response)
}

func cleanupEmptyFoldersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}

	// 백그라운드에서 빈 폴더 정리 실행
	go func() {
		cleanedCount, err := driveService.cleanupAllEmptyFolders()
		if err != nil {
			log.Printf("❌ 빈 폴더 정리 실패: %v", err)
		} else {
			log.Printf("✅ 빈 폴더 정리 완료: %d개 폴더 삭제됨", cleanedCount)
		}
	}()

	response := map[string]string{
		"status":  "started",
		"message": "빈 폴더 정리가 백그라운드에서 시작되었습니다",
	}

	json.NewEncoder(w).Encode(response)
}

func deleteTargetFolderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST method required", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var request struct {
		FolderID   string `json:"folderId"`
		FolderName string `json:"folderName"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		response := map[string]string{"error": "잘못된 요청 형식"}
		json.NewEncoder(w).Encode(response)
		return
	}

	driveService := getGlobalDriveService()
	if driveService == nil {
		response := map[string]string{"error": "DriveService가 초기화되지 않았습니다"}
		json.NewEncoder(w).Encode(response)
		return
	}

	log.Printf("🗑️ 대상 폴더 전체 삭제 시작: %s (%s)", request.FolderName, request.FolderID)

	// 폴더 삭제 (백그라운드에서 실행)
	go func() {
		err := driveService.deleteTargetFolder(request.FolderID, request.FolderName)
		if err != nil {
			log.Printf("❌ 대상 폴더 삭제 실패: %v", err)
		} else {
			log.Printf("✅ 대상 폴더 삭제 완료: %s", request.FolderName)
		}
	}()

	response := map[string]string{
		"status":  "started",
		"message": fmt.Sprintf("'%s' 폴더 삭제가 백그라운드에서 시작되었습니다", request.FolderName),
	}

	json.NewEncoder(w).Encode(response)
}
