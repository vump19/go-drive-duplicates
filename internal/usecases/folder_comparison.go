package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// FolderComparisonUseCase handles folder comparison operations
type FolderComparisonUseCase struct {
	fileRepo          repositories.FileRepository
	comparisonRepo    repositories.ComparisonRepository
	progressRepo      repositories.ProgressRepository
	storageProvider   services.StorageProvider
	hashService       services.HashService
	comparisonService services.ComparisonService
	progressService   services.ProgressService

	// Configuration
	workerCount       int
	includeSubfolders bool
	deepComparison    bool
	minFileSize       int64

	// Cancellation management
	runningOps   map[int]context.CancelFunc
	runningOpsMu sync.RWMutex
}

// NewFolderComparisonUseCase creates a new folder comparison use case
func NewFolderComparisonUseCase(
	fileRepo repositories.FileRepository,
	comparisonRepo repositories.ComparisonRepository,
	progressRepo repositories.ProgressRepository,
	storageProvider services.StorageProvider,
	hashService services.HashService,
	comparisonService services.ComparisonService,
	progressService services.ProgressService,
) *FolderComparisonUseCase {
	return &FolderComparisonUseCase{
		fileRepo:          fileRepo,
		comparisonRepo:    comparisonRepo,
		progressRepo:      progressRepo,
		storageProvider:   storageProvider,
		hashService:       hashService,
		comparisonService: comparisonService,
		progressService:   progressService,
		workerCount:       5,
		includeSubfolders: true,
		deepComparison:    true,
		minFileSize:       0,
		runningOps:        make(map[int]context.CancelFunc),
	}
}

// CompareFoldersRequest represents the request for comparing folders
type CompareFoldersRequest struct {
	SourceFolderID     string                   `json:"sourceFolderId"`
	TargetFolderID     string                   `json:"targetFolderId"`
	IncludeSubfolders  bool                     `json:"includeSubfolders"`
	DeepComparison     bool                     `json:"deepComparison"`
	ForceNewComparison bool                     `json:"forceNewComparison"` // 기존 결과 무시하고 새로 비교
	MinFileSize        int64                    `json:"minFileSize,omitempty"`
	WorkerCount        int                      `json:"workerCount,omitempty"`
	ResumeProgressID   int                      `json:"resumeProgressId,omitempty"`   // 재개할 진행 상황 ID
	ExcludeFolderNames []string                 `json:"excludeFolderNames,omitempty"` // 제외할 폴더명 목록
	ProgressCallback   func(*entities.Progress) `json:"-"`
}

// ResumeComparisonRequest represents the request for resuming folder comparison
type ResumeComparisonRequest struct {
	ProgressID int `json:"progressId"`
}

// CompareFoldersResponse represents the response for comparing folders
type CompareFoldersResponse struct {
	Progress         *entities.Progress         `json:"progress"`
	ComparisonResult *entities.ComparisonResult `json:"comparisonResult"`
	Errors           []string                   `json:"errors,omitempty"`
}

// GetComparisonProgressRequest represents the request for getting comparison progress
type GetComparisonProgressRequest struct {
	ComparisonID int `json:"comparisonId"`
}

// LoadSavedComparisonRequest represents the request for loading a saved comparison
type LoadSavedComparisonRequest struct {
	SourceFolderID string `json:"sourceFolderId"`
	TargetFolderID string `json:"targetFolderId"`
}

// CompareFolders compares two folders and finds duplicate files
func (uc *FolderComparisonUseCase) CompareFolders(ctx context.Context, req *CompareFoldersRequest) (*CompareFoldersResponse, error) {
	log.Printf("📂 폴더 비교 시작: %s vs %s", req.SourceFolderID, req.TargetFolderID)

	// Apply configuration
	if req.WorkerCount > 0 {
		uc.workerCount = req.WorkerCount
	}
	uc.includeSubfolders = req.IncludeSubfolders
	uc.deepComparison = req.DeepComparison
	if req.MinFileSize > 0 {
		uc.minFileSize = req.MinFileSize
	}

	// 재개 요청인 경우 기존 진행 상황 확인
	if req.ResumeProgressID > 0 {
		return uc.resumeComparison(ctx, req)
	}

	// Validate folder access
	if err := uc.validateFolderAccess(ctx, req.SourceFolderID, req.TargetFolderID); err != nil {
		return nil, fmt.Errorf("폴더 접근 권한 확인 실패: %w", err)
	}

	// Check for existing comparison (only if not forced to create new)
	if !req.ForceNewComparison {
		existingComparison, err := uc.comparisonRepo.GetByFolders(ctx, req.SourceFolderID, req.TargetFolderID)
		if err == nil && existingComparison != nil {
			log.Printf("📋 기존 비교 결과 발견: ID %d", existingComparison.ID)
			return &CompareFoldersResponse{
				ComparisonResult: existingComparison,
				Errors:           make([]string, 0),
			}, nil
		}
	} else {
		log.Printf("🔄 새로운 비교 강제 실행 - 기존 결과 무시")
	}

	// Create progress tracker with checkpoint metadata
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFolderComparison, 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// 체크포인트 메타데이터 저장
	progress.SetMetadata("sourceFolderId", req.SourceFolderID)
	progress.SetMetadata("targetFolderId", req.TargetFolderID)
	progress.SetMetadata("includeSubfolders", req.IncludeSubfolders)
	progress.SetMetadata("deepComparison", req.DeepComparison)
	progress.SetMetadata("minFileSize", req.MinFileSize)
	progress.SetMetadata("workerCount", uc.workerCount)
	progress.SetMetadata("excludeFolderNames", req.ExcludeFolderNames)
	progress.SetMetadata("currentPhase", "initialized")

	// 제외 폴더명 로깅
	if len(req.ExcludeFolderNames) > 0 {
		log.Printf("🚫 제외할 폴더명: %v", req.ExcludeFolderNames)
	}

	// Get folder names
	sourceFolderName, targetFolderName := uc.getFolderNames(ctx, req.SourceFolderID, req.TargetFolderID)

	// Initialize comparison result
	comparisonResult := entities.NewComparisonResult(
		req.SourceFolderID,
		req.TargetFolderID,
		sourceFolderName,
		targetFolderName,
	)

	// Initialize response
	response := &CompareFoldersResponse{
		Progress:         progress,
		ComparisonResult: comparisonResult,
		Errors:           make([]string, 0),
	}

	// Start comparison in background with a cancellable context (not tied to HTTP request)
	ctx, cancel := context.WithCancel(context.Background())

	// Store the cancel function for later cancellation
	uc.runningOpsMu.Lock()
	uc.runningOps[progress.ID] = cancel
	uc.runningOpsMu.Unlock()

	go uc.performFolderComparison(ctx, req, progress, comparisonResult, response)

	return response, nil
}

// GetComparisonProgress returns the current comparison progress
func (uc *FolderComparisonUseCase) GetComparisonProgress(ctx context.Context, req *GetComparisonProgressRequest) (*entities.Progress, error) {
	return uc.progressService.GetProgress(ctx, req.ComparisonID)
}

// ResumeComparison resumes a paused or failed comparison
func (uc *FolderComparisonUseCase) ResumeComparison(ctx context.Context, req *ResumeComparisonRequest) (*CompareFoldersResponse, error) {
	log.Printf("🔄 폴더 비교 재개: Progress ID %d", req.ProgressID)

	// Get existing progress
	progress, err := uc.progressService.GetProgress(ctx, req.ProgressID)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 조회 실패: %w", err)
	}

	if progress.Status == entities.StatusCompleted {
		return nil, fmt.Errorf("이미 완료된 작업입니다")
	}

	// Extract metadata to reconstruct request
	sourceFolderID, _ := progress.GetMetadata("sourceFolderId")
	targetFolderID, _ := progress.GetMetadata("targetFolderId")
	includeSubfolders, _ := progress.GetMetadata("includeSubfolders")
	deepComparison, _ := progress.GetMetadata("deepComparison")
	minFileSize, _ := progress.GetMetadata("minFileSize")
	workerCount, _ := progress.GetMetadata("workerCount")
	excludeFolderNamesRaw, _ := progress.GetMetadata("excludeFolderNames")

	// Parse excludeFolderNames from metadata
	var excludeFolderNames []string
	if excludeFolderNamesRaw != nil {
		if names, ok := excludeFolderNamesRaw.([]interface{}); ok {
			for _, name := range names {
				if strName, ok := name.(string); ok {
					excludeFolderNames = append(excludeFolderNames, strName)
				}
			}
		}
	}

	// Reconstruct request
	resumeReq := &CompareFoldersRequest{
		SourceFolderID:     sourceFolderID.(string),
		TargetFolderID:     targetFolderID.(string),
		IncludeSubfolders:  includeSubfolders.(bool),
		DeepComparison:     deepComparison.(bool),
		MinFileSize:        int64(minFileSize.(float64)),
		WorkerCount:        int(workerCount.(float64)),
		ExcludeFolderNames: excludeFolderNames,
		ResumeProgressID:   req.ProgressID,
	}

	return uc.resumeComparison(ctx, resumeReq)
}

// resumeComparison handles the actual resumption logic
func (uc *FolderComparisonUseCase) resumeComparison(ctx context.Context, req *CompareFoldersRequest) (*CompareFoldersResponse, error) {
	// Get existing progress
	progress, err := uc.progressService.GetProgress(ctx, req.ResumeProgressID)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 조회 실패: %w", err)
	}

	// Get folder names
	sourceFolderName, targetFolderName := uc.getFolderNames(ctx, req.SourceFolderID, req.TargetFolderID)

	// Check if comparison result already exists
	comparisonResult, err := uc.comparisonRepo.GetByFolders(ctx, req.SourceFolderID, req.TargetFolderID)
	if err != nil {
		// Create new comparison result
		comparisonResult = entities.NewComparisonResult(
			req.SourceFolderID,
			req.TargetFolderID,
			sourceFolderName,
			targetFolderName,
		)
	}

	// Initialize response
	response := &CompareFoldersResponse{
		Progress:         progress,
		ComparisonResult: comparisonResult,
		Errors:           make([]string, 0),
	}

	// Resume progress
	progress.Resume()
	uc.progressService.UpdateOperation(ctx, progress.ID, progress.ProcessedItems, "작업 재개 중...")

	// Create cancellable context and continue comparison from checkpoint
	bgCtx, cancel := context.WithCancel(context.Background())

	// Store the cancel function for later cancellation
	uc.runningOpsMu.Lock()
	uc.runningOps[progress.ID] = cancel
	uc.runningOpsMu.Unlock()

	go uc.performFolderComparison(bgCtx, req, progress, comparisonResult, response)

	return response, nil
}

// GetPendingComparisons returns all pending/failed comparison operations
func (uc *FolderComparisonUseCase) GetPendingComparisons(ctx context.Context) ([]*entities.Progress, error) {
	// For now, return empty list - this can be implemented later when needed
	log.Printf("📋 중단된 작업 조회 요청 (현재 미구현)")
	return []*entities.Progress{}, nil
}

// GetRecentSingleFolderResults returns recent completed single-folder duplicate search results
func (uc *FolderComparisonUseCase) GetRecentSingleFolderResults(ctx context.Context, limit int) ([]*entities.Progress, error) {
	allOps, err := uc.progressRepo.GetByOperationType(ctx, "single_folder_duplicates")
	if err != nil {
		return nil, err
	}

	var results []*entities.Progress
	for _, op := range allOps {
		if op.Status == entities.StatusCompleted {
			results = append(results, op)
			if len(results) >= limit {
				break
			}
		}
	}
	return results, nil
}

// CancelComparison cancels a running comparison operation
func (uc *FolderComparisonUseCase) CancelComparison(ctx context.Context, progressID int) error {
	log.Printf("🛑 폴더 비교 작업 취소 요청: Progress ID %d", progressID)

	// Get the cancel function
	uc.runningOpsMu.RLock()
	cancel, exists := uc.runningOps[progressID]
	uc.runningOpsMu.RUnlock()

	if !exists {
		// Check if the operation exists in the database
		progress, err := uc.progressService.GetProgress(ctx, progressID)
		if err != nil {
			return fmt.Errorf("작업을 찾을 수 없습니다: %w", err)
		}

		// If the operation is already completed or failed, return error
		if progress.Status == entities.StatusCompleted {
			return fmt.Errorf("이미 완료된 작업입니다")
		}
		if progress.Status == entities.StatusFailed {
			return fmt.Errorf("이미 실패한 작업입니다")
		}
		if progress.Status == entities.StatusCancelled {
			return fmt.Errorf("이미 취소된 작업입니다")
		}

		// The operation might have been started in a previous server session
		// Update the status to cancelled
		uc.progressService.CancelOperation(ctx, progressID)
		log.Printf("⏹️ 이전 세션의 작업 상태를 취소로 변경: Progress ID %d", progressID)
		return nil
	}

	// Call the cancel function
	cancel()

	// Remove from running operations
	uc.runningOpsMu.Lock()
	delete(uc.runningOps, progressID)
	uc.runningOpsMu.Unlock()

	log.Printf("✅ 폴더 비교 작업 취소 완료: Progress ID %d", progressID)
	return nil
}

// LoadSavedComparison loads a previously saved comparison result
func (uc *FolderComparisonUseCase) LoadSavedComparison(ctx context.Context, req *LoadSavedComparisonRequest) (*entities.ComparisonResult, error) {
	comparison, err := uc.comparisonRepo.GetByFolders(ctx, req.SourceFolderID, req.TargetFolderID)
	if err != nil {
		return nil, fmt.Errorf("저장된 비교 결과 조회 실패: %w", err)
	}

	if comparison == nil {
		return nil, fmt.Errorf("저장된 비교 결과를 찾을 수 없습니다")
	}

	return comparison, nil
}

// DeleteComparisonResult deletes a comparison result
func (uc *FolderComparisonUseCase) DeleteComparisonResult(ctx context.Context, comparisonID int) error {
	return uc.comparisonRepo.Delete(ctx, comparisonID)
}

// GetRecentComparisons returns recent comparison results
func (uc *FolderComparisonUseCase) GetRecentComparisons(ctx context.Context, limit int) ([]*entities.ComparisonResult, error) {
	return uc.comparisonRepo.GetRecentComparisons(ctx, limit)
}

// performFolderComparison performs the actual folder comparison
func (uc *FolderComparisonUseCase) performFolderComparison(ctx context.Context, req *CompareFoldersRequest, progress *entities.Progress, result *entities.ComparisonResult, response *CompareFoldersResponse) {
	log.Printf("🚀 백그라운드 폴더 비교 작업 시작 - Progress ID: %d", progress.ID)

	defer func() {
		// Clean up the cancel function from the map
		uc.runningOpsMu.Lock()
		delete(uc.runningOps, progress.ID)
		uc.runningOpsMu.Unlock()

		if r := recover(); r != nil {
			log.Printf("💥 폴더 비교 중 패닉 발생: %v", r)
			log.Printf("📍 패닉 위치 - Progress ID: %d, 현재 단계: %v", progress.ID, progress.CurrentStep)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
		log.Printf("🔚 백그라운드 폴더 비교 작업 종료 - Progress ID: %d", progress.ID)
	}()

	// Check for cancellation
	if ctx.Err() != nil {
		log.Printf("🛑 작업이 취소되었습니다: Progress ID %d", progress.ID)
		uc.progressService.CancelOperation(ctx, progress.ID)
		return
	}

	// Update progress to running
	log.Printf("▶️ 진행 상태를 실행 중으로 변경...")
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "폴더 비교 시작...")
	log.Printf("✅ 진행 상태 업데이트 완료: %s", progress.Status)

	// Check current phase and resume from checkpoint
	currentPhase, _ := progress.GetMetadata("currentPhase")
	var sourceFiles, targetFiles []*entities.File
	var err error

	// Step 1: Get files from source folder (skip if already done)
	if currentPhase == "initialized" || currentPhase == "" {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "기준 폴더 파일 조회 중...")
		progress.SetMetadata("currentPhase", "scanning_source")
		progress.SetMetadata("scanStartTime", time.Now().UnixMilli())

		log.Printf("📂 기준 폴더 스캔 시작 - Folder ID: %s, 하위폴더 포함: %v", req.SourceFolderID, req.IncludeSubfolders)
		sourceFiles, err = uc.getFilesFromFolderWithProgress(ctx, req.SourceFolderID, req.IncludeSubfolders, req.ExcludeFolderNames, progress)
		if err != nil {
			log.Printf("❌ 기준 폴더 파일 조회 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("기준 폴더 파일 조회 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("기준 폴더 파일 조회 실패: %v", err))
			return
		}

		// Save checkpoint - source files scanned
		progress.SetMetadata("sourceFileCount", len(sourceFiles))
		progress.SetMetadata("currentPhase", "source_completed")
		log.Printf("📂 기준 폴더 스캔 완료: %d개 파일 발견", len(sourceFiles))

		// Important: Save progress immediately to persist the checkpoint
		err = uc.progressRepo.Update(ctx, progress)
		if err != nil {
			log.Printf("⚠️ 진행 상태 저장 실패: %v", err)
		}

	} else if currentPhase == "source_completed" || currentPhase == "scanning_target" {
		log.Printf("🔄 체크포인트에서 재개: %s", currentPhase)
		// Reload source files if resuming
		sourceFiles, err = uc.getFilesFromFolderWithExclusion(ctx, req.SourceFolderID, req.IncludeSubfolders, req.ExcludeFolderNames)
		if err != nil {
			log.Printf("❌ 기준 폴더 파일 재조회 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("기준 폴더 파일 재조회 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("기준 폴더 파일 재조회 실패: %v", err))
			return
		}
	}

	// Step 2: Get files from target folder (skip if already done)
	log.Printf("🔍 대상 폴더 스캔 조건 확인 - currentPhase: '%s'", currentPhase)
	// If we just completed source scanning or are resuming target scanning, proceed with target folder
	if currentPhase == "source_completed" || currentPhase == "scanning_target" || (currentPhase == "initialized" && len(sourceFiles) > 0) {
		log.Printf("✅ 대상 폴더 스캔 조건 통과")
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "대상 폴더 파일 조회 중...")
		progress.SetMetadata("currentPhase", "scanning_target")

		// Save progress immediately to persist the phase change
		err = uc.progressRepo.Update(ctx, progress)
		if err != nil {
			log.Printf("⚠️ 스캔 상태 저장 실패: %v", err)
		}

		log.Printf("🎯 대상 폴더 스캔 시작 - Folder ID: %s, 하위폴더 포함: %v", req.TargetFolderID, req.IncludeSubfolders)
		progress.SetMetadata("scanStartTime", time.Now().UnixMilli())
		progress.SetMetadata("scannedFileCount", 0)
		progress.SetMetadata("scannedFolderCount", 0)
		targetFiles, err = uc.getFilesFromFolderWithProgress(ctx, req.TargetFolderID, req.IncludeSubfolders, req.ExcludeFolderNames, progress)
		if err != nil {
			log.Printf("❌ 대상 폴더 파일 조회 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("대상 폴더 파일 조회 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("대상 폴더 파일 조회 실패: %v", err))
			return
		}

		// Save checkpoint - target files scanned
		progress.SetMetadata("targetFileCount", len(targetFiles))
		progress.SetMetadata("currentPhase", "target_completed")
		log.Printf("🎯 대상 폴더 스캔 완료: %d개 파일 발견", len(targetFiles))

		// Important: Save progress immediately to persist the checkpoint
		err = uc.progressRepo.Update(ctx, progress)
		if err != nil {
			log.Printf("⚠️ 대상 폴더 스캔 완료 상태 저장 실패: %v", err)
		}
	} else {
		log.Printf("⚠️ 대상 폴더 스캔 조건 불일치 - currentPhase: '%s'", currentPhase)
	}

	// Handle other phases or continue processing
	if currentPhase != "initialized" && currentPhase != "scanning_source" && currentPhase != "source_completed" && currentPhase != "scanning_target" && currentPhase != "target_completed" {
		// Load both source and target files if resuming from later phase
		sourceFiles, err = uc.getFilesFromFolderWithExclusion(ctx, req.SourceFolderID, req.IncludeSubfolders, req.ExcludeFolderNames)
		if err != nil {
			log.Printf("❌ 기준 폴더 파일 재조회 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("기준 폴더 파일 재조회 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("기준 폴더 파일 재조회 실패: %v", err))
			return
		}

		targetFiles, err = uc.getFilesFromFolderWithExclusion(ctx, req.TargetFolderID, req.IncludeSubfolders, req.ExcludeFolderNames)
		if err != nil {
			log.Printf("❌ 대상 폴더 파일 재조회 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("대상 폴더 파일 재조회 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("대상 폴더 파일 재조회 실패: %v", err))
			return
		}
	}

	log.Printf("📊 기준 폴더: %d개 파일, 대상 폴더: %d개 파일", len(sourceFiles), len(targetFiles))

	// Update folder statistics
	sourceTotalSize := uc.calculateTotalSize(sourceFiles)
	targetTotalSize := uc.calculateTotalSize(targetFiles)
	result.SetSourceStats(len(sourceFiles), sourceTotalSize)
	result.SetTargetStats(len(targetFiles), targetTotalSize)

	// Step 3: Calculate hashes if deep comparison is enabled (skip if already done)
	totalFiles := len(sourceFiles) + len(targetFiles)
	progress.SetTotal(totalFiles)

	if req.DeepComparison && (currentPhase == "target_completed" || currentPhase == "calculating_hashes") {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 해시 계산 중...")
		progress.SetMetadata("currentPhase", "calculating_hashes")

		allFiles := append(sourceFiles, targetFiles...)
		err := uc.calculateHashesForFiles(ctx, allFiles, progress, req.ProgressCallback)
		if err != nil {
			log.Printf("❌ 해시 계산 실패: %v", err)
			response.Errors = append(response.Errors, fmt.Sprintf("해시 계산 실패: %v", err))
		} else {
			// Save checkpoint - hash calculation completed
			progress.SetMetadata("currentPhase", "hashes_completed")
			log.Printf("🔐 체크포인트 저장: 해시 계산 완료")
		}
	} else if currentPhase == "hashes_completed" || currentPhase == "comparing_files" {
		log.Printf("🔄 해시 계산 단계 건너뛰기 (이미 완료됨)")
	}

	// Step 4: Compare files and find duplicates (final phase)
	if currentPhase != "completed" {
		uc.progressService.UpdateOperation(ctx, progress.ID, totalFiles, "중복 파일 검색 중...")
		progress.SetMetadata("currentPhase", "comparing_files")

		// Use the new comparison function that tracks unique files too
		comparisonFilesResult := uc.compareFilesWithUniqueTracking(sourceFiles, targetFiles, req.DeepComparison)

		// Add duplicate files to result
		for _, file := range comparisonFilesResult.Duplicates {
			result.AddDuplicateFile(file)
		}

		// Add unique files to result
		for _, file := range comparisonFilesResult.UniqueInSource {
			result.AddUniqueInSource(file)
		}
		for _, file := range comparisonFilesResult.UniqueInTarget {
			result.AddUniqueInTarget(file)
		}

		log.Printf("📊 중복 파일 %d개 발견 (%.1f%% 중복), 비중복 파일: 기준폴더 %d개, 대상폴더 %d개",
			len(comparisonFilesResult.Duplicates), result.DuplicationPercentage,
			len(comparisonFilesResult.UniqueInSource), len(comparisonFilesResult.UniqueInTarget))

		// Step 5a: Save files to database first to avoid foreign key constraint errors
		log.Printf("💾 파일 메타데이터 저장 시작...")
		uc.progressService.UpdateOperation(ctx, progress.ID, totalFiles, "파일 메타데이터 저장 중...")

		allFiles := append(sourceFiles, targetFiles...)
		err = uc.saveFilesToDatabase(ctx, allFiles)
		if err != nil {
			log.Printf("❌ 파일 저장 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("파일 저장 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("파일 저장 실패: %v", err))
			return
		}
		log.Printf("✅ 파일 메타데이터 저장 완료")

		// Step 5b: Save comparison result
		log.Printf("💾 비교 결과 저장 시작...")
		uc.progressService.UpdateOperation(ctx, progress.ID, totalFiles, "비교 결과 저장 중...")
		progress.SetMetadata("currentPhase", "saving_results")

		log.Printf("📊 저장할 데이터: 중복 파일 %d개, 기준폴더만 %d개, 대상폴더만 %d개",
			result.DuplicateCount, len(result.UniqueInSource), len(result.UniqueInTarget))

		err = uc.comparisonRepo.Save(ctx, result)
		if err != nil {
			log.Printf("❌ 비교 결과 저장 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("비교 결과 저장 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("비교 결과 저장 실패: %v", err))
			return
		}
		log.Printf("✅ 비교 결과 저장 완료")

		// Step 5c: Save unique files to database
		if len(result.UniqueInTarget) > 0 {
			log.Printf("💾 비중복 파일 저장 시작 (대상 폴더: %d개)...", len(result.UniqueInTarget))
			uc.progressService.UpdateOperation(ctx, progress.ID, totalFiles, "비중복 파일 저장 중...")

			// Use type assertion to access SaveUniqueFiles method
			if repo, ok := uc.comparisonRepo.(interface {
				SaveUniqueFiles(ctx context.Context, comparisonID int, files []*entities.File, folderType string) error
			}); ok {
				err = repo.SaveUniqueFiles(ctx, result.ID, result.UniqueInTarget, "target")
				if err != nil {
					log.Printf("⚠️ 비중복 파일 저장 실패: %v", err)
					response.Errors = append(response.Errors, fmt.Sprintf("비중복 파일 저장 실패: %v", err))
				} else {
					log.Printf("✅ 대상 폴더 비중복 파일 %d개 저장 완료", len(result.UniqueInTarget))
				}
			}
		}

		if len(result.UniqueInSource) > 0 {
			log.Printf("💾 비중복 파일 저장 시작 (기준 폴더: %d개)...", len(result.UniqueInSource))

			if repo, ok := uc.comparisonRepo.(interface {
				SaveUniqueFiles(ctx context.Context, comparisonID int, files []*entities.File, folderType string) error
			}); ok {
				err = repo.SaveUniqueFiles(ctx, result.ID, result.UniqueInSource, "source")
				if err != nil {
					log.Printf("⚠️ 비중복 파일 저장 실패: %v", err)
					response.Errors = append(response.Errors, fmt.Sprintf("비중복 파일 저장 실패: %v", err))
				} else {
					log.Printf("✅ 기준 폴더 비중복 파일 %d개 저장 완료", len(result.UniqueInSource))
				}
			}
		}

		// Final checkpoint - comparison completed
		progress.SetMetadata("currentPhase", "completed")
		log.Printf("✅ 체크포인트 저장: 폴더 비교 완료")
	}

	// Complete the operation
	log.Printf("🏁 작업 완료 처리 시작...")
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()
	log.Printf("✅ 진행 상황 완료 처리됨")

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
		log.Printf("📞 진행 상황 콜백 호출됨")
	}

	log.Printf("🎉 폴더 비교 최종 완료: %s 절약 가능, 폴더 삭제 권장: %v",
		formatFileSize(result.GetWastedSpace()), result.CanDeleteTargetFolder)
	log.Printf("📈 최종 통계: 기준 폴더 %d개 파일, 대상 폴더 %d개 파일, 중복률 %.1f%%",
		len(sourceFiles), len(targetFiles), result.DuplicationPercentage)
}

// validateFolderAccess validates access to both folders
func (uc *FolderComparisonUseCase) validateFolderAccess(ctx context.Context, sourceFolderID, targetFolderID string) error {
	// Check source folder
	_, err := uc.storageProvider.GetFolder(ctx, sourceFolderID)
	if err != nil {
		return fmt.Errorf("기준 폴더 접근 실패 [%s]: %w", sourceFolderID, err)
	}

	// Check target folder
	_, err = uc.storageProvider.GetFolder(ctx, targetFolderID)
	if err != nil {
		return fmt.Errorf("대상 폴더 접근 실패 [%s]: %w", targetFolderID, err)
	}

	return nil
}

// getFolderNames retrieves folder names for display
func (uc *FolderComparisonUseCase) getFolderNames(ctx context.Context, sourceFolderID, targetFolderID string) (string, string) {
	sourceName := "알 수 없는 폴더"
	targetName := "알 수 없는 폴더"

	if sourceFolder, err := uc.storageProvider.GetFolder(ctx, sourceFolderID); err == nil {
		sourceName = sourceFolder.Name
	}

	if targetFolder, err := uc.storageProvider.GetFolder(ctx, targetFolderID); err == nil {
		targetName = targetFolder.Name
	}

	return sourceName, targetName
}

// getFilesFromFolder retrieves files from a folder
func (uc *FolderComparisonUseCase) getFilesFromFolder(ctx context.Context, folderID string, includeSubfolders bool) ([]*entities.File, error) {
	return uc.getFilesFromFolderWithExclusion(ctx, folderID, includeSubfolders, nil)
}

// getFilesFromFolderWithExclusion retrieves files from a folder with folder name exclusions
func (uc *FolderComparisonUseCase) getFilesFromFolderWithExclusion(ctx context.Context, folderID string, includeSubfolders bool, excludeFolderNames []string) ([]*entities.File, error) {
	return uc.getFilesFromFolderWithProgress(ctx, folderID, includeSubfolders, excludeFolderNames, nil)
}

// getFilesFromFolderWithProgress scans folder with real-time progress updates
func (uc *FolderComparisonUseCase) getFilesFromFolderWithProgress(ctx context.Context, folderID string, includeSubfolders bool, excludeFolderNames []string, progress *entities.Progress) ([]*entities.File, error) {
	if includeSubfolders {
		return uc.getFilesRecursiveWithProgress(ctx, folderID, excludeFolderNames, progress)
	}
	// Non-recursive: still need to set file paths
	folderInfo, err := uc.storageProvider.GetFolder(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 정보 조회 실패 [%s]: %v", folderID, err)
		return uc.storageProvider.ListFiles(ctx, folderID)
	}

	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return nil, err
	}

	// Set file paths
	for _, file := range files {
		if file.GetFileCategory() != "folder" {
			file.Path = folderInfo.Name + "/" + file.Name
		}
	}
	return files, nil
}

// getFilesRecursive recursively gets files from folder and subfolders
func (uc *FolderComparisonUseCase) getFilesRecursive(ctx context.Context, folderID string) ([]*entities.File, error) {
	return uc.getFilesRecursiveWithExclusion(ctx, folderID, nil)
}

// getFilesRecursiveWithProgress recursively gets files with real-time progress updates
func (uc *FolderComparisonUseCase) getFilesRecursiveWithProgress(ctx context.Context, folderID string, excludeFolderNames []string, progress *entities.Progress) ([]*entities.File, error) {
	// Get folder name for path building
	folderInfo, err := uc.storageProvider.GetFolder(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 정보 조회 실패 [%s]: %v - 경로 없이 스캔 진행", folderID, err)
		return uc.getFilesRecursiveWithPathAndProgress(ctx, folderID, "", excludeFolderNames, progress, &scanStats{})
	}

	log.Printf("📂 폴더 정보 조회 성공: %s (ID: %s)", folderInfo.Name, folderID)
	return uc.getFilesRecursiveWithPathAndProgress(ctx, folderID, folderInfo.Name, excludeFolderNames, progress, &scanStats{})
}

// scanStats tracks scanning statistics for real-time progress updates
type scanStats struct {
	fileCount      int
	folderCount    int
	lastUpdateTime int64
}

// getFilesRecursiveWithPathAndProgress recursively gets files with path tracking and progress updates
func (uc *FolderComparisonUseCase) getFilesRecursiveWithPathAndProgress(ctx context.Context, folderID, currentPath string, excludeFolderNames []string, progress *entities.Progress, stats *scanStats) ([]*entities.File, error) {
	var allFiles []*entities.File

	// Get files in current folder
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		log.Printf("❌ 폴더 파일 조회 실패 [%s]: %v", folderID, err)
		return nil, err
	}

	// Update folder count
	stats.folderCount++

	// Separate files and folders
	var actualFiles []*entities.File
	var subfolders []*entities.File

	for _, file := range files {
		if file.GetFileCategory() == "folder" {
			if uc.shouldExcludeFolder(file.Name, excludeFolderNames) {
				continue
			}
			subfolders = append(subfolders, file)
		} else if file.Size >= uc.minFileSize {
			if currentPath != "" {
				file.Path = currentPath + "/" + file.Name
			} else {
				file.Path = file.Name
			}
			actualFiles = append(actualFiles, file)
			stats.fileCount++
		}
	}

	// Update progress metadata periodically (every 500ms or every 100 files)
	if progress != nil {
		now := nowUnixMilli()
		if now-stats.lastUpdateTime > 500 || stats.fileCount%100 == 0 {
			stats.lastUpdateTime = now
			progress.SetMetadata("scannedFileCount", stats.fileCount)
			progress.SetMetadata("scannedFolderCount", stats.folderCount)
			progress.SetMetadata("lastScannedPath", currentPath)
			// Save to database for frontend polling
			uc.progressRepo.Update(ctx, progress)
		}
	}

	allFiles = append(allFiles, actualFiles...)

	// Recursively get files from subfolders
	for _, subfolder := range subfolders {
		subfolderPath := currentPath
		if subfolderPath != "" {
			subfolderPath = subfolderPath + "/" + subfolder.Name
		} else {
			subfolderPath = subfolder.Name
		}

		subFiles, err := uc.getFilesRecursiveWithPathAndProgress(ctx, subfolder.ID, subfolderPath, excludeFolderNames, progress, stats)
		if err != nil {
			log.Printf("⚠️ 하위 폴더 파일 조회 실패 [%s]: %v", subfolder.ID, err)
			continue
		}
		allFiles = append(allFiles, subFiles...)
	}

	return allFiles, nil
}

// nowUnixMilli returns current time in milliseconds
func nowUnixMilli() int64 {
	return time.Now().UnixMilli()
}

// getFilesRecursiveWithExclusion recursively gets files from folder and subfolders, excluding specified folder names
func (uc *FolderComparisonUseCase) getFilesRecursiveWithExclusion(ctx context.Context, folderID string, excludeFolderNames []string) ([]*entities.File, error) {
	// Get folder name for path building
	folderInfo, err := uc.storageProvider.GetFolder(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 정보 조회 실패 [%s]: %v - 경로 없이 스캔 진행", folderID, err)
		// Fall back to scanning without paths
		return uc.getFilesRecursiveWithPath(ctx, folderID, "", excludeFolderNames)
	}

	log.Printf("📂 폴더 정보 조회 성공: %s (ID: %s)", folderInfo.Name, folderID)
	return uc.getFilesRecursiveWithPath(ctx, folderID, folderInfo.Name, excludeFolderNames)
}

// getFilesRecursiveWithPath recursively gets files with path tracking
func (uc *FolderComparisonUseCase) getFilesRecursiveWithPath(ctx context.Context, folderID, currentPath string, excludeFolderNames []string) ([]*entities.File, error) {
	var allFiles []*entities.File

	// Get files in current folder
	log.Printf("📂 폴더 스캔 시작: %s (경로: %s)", folderID, currentPath)
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		log.Printf("❌ 폴더 파일 조회 실패 [%s]: %v", folderID, err)
		return nil, err
	}
	log.Printf("📋 폴더 [%s]에서 발견된 항목: %d개", folderID, len(files))

	// Debug: Log each item to see what we're getting
	for i, file := range files {
		log.Printf("  항목 %d: %s (타입: %s, 크기: %d)", i+1, file.Name, file.MimeType, file.Size)
	}

	// Separate files and folders
	var actualFiles []*entities.File
	var subfolders []*entities.File

	for _, file := range files {
		if file.GetFileCategory() == "folder" {
			// Check if this folder should be excluded
			if uc.shouldExcludeFolder(file.Name, excludeFolderNames) {
				log.Printf("🚫 폴더 제외: %s (%s) - 제외 목록에 포함됨", file.Name, file.ID)
				continue
			}
			log.Printf("📁 하위 폴더 발견: %s (%s)", file.Name, file.ID)
			subfolders = append(subfolders, file)
		} else if file.Size >= uc.minFileSize {
			// Set file path
			if currentPath != "" {
				file.Path = currentPath + "/" + file.Name
			} else {
				file.Path = file.Name
			}
			log.Printf("📄 파일 발견: %s (크기: %d bytes, 경로: %s)", file.Name, file.Size, file.Path)
			actualFiles = append(actualFiles, file)
		} else {
			log.Printf("⏭️ 파일 건너뛰기 (최소 크기 미만): %s (크기: %d bytes)", file.Name, file.Size)
		}
	}

	log.Printf("📊 현재 폴더 [%s]: 파일 %d개, 하위 폴더 %d개", folderID, len(actualFiles), len(subfolders))

	// Add current folder files
	allFiles = append(allFiles, actualFiles...)

	// Recursively get files from subfolders
	for _, subfolder := range subfolders {
		// Build subfolder path
		subfolderPath := currentPath
		if subfolderPath != "" {
			subfolderPath = subfolderPath + "/" + subfolder.Name
		} else {
			subfolderPath = subfolder.Name
		}

		log.Printf("🔍 하위 폴더 재귀 스캔 시작: %s (%s, 경로: %s)", subfolder.Name, subfolder.ID, subfolderPath)
		subFiles, err := uc.getFilesRecursiveWithPath(ctx, subfolder.ID, subfolderPath, excludeFolderNames)
		if err != nil {
			log.Printf("⚠️ 하위 폴더 파일 조회 실패 [%s]: %v", subfolder.ID, err)
			continue
		}
		log.Printf("✅ 하위 폴더 [%s]에서 %d개 파일 발견", subfolder.Name, len(subFiles))
		allFiles = append(allFiles, subFiles...)
	}

	log.Printf("🎯 폴더 [%s] 최종 결과: 총 %d개 파일", folderID, len(allFiles))
	return allFiles, nil
}

// shouldExcludeFolder checks if a folder name matches any of the exclusion patterns
func (uc *FolderComparisonUseCase) shouldExcludeFolder(folderName string, excludeNames []string) bool {
	if len(excludeNames) == 0 {
		return false
	}

	// Convert folder name to lowercase for case-insensitive comparison
	lowerFolderName := strings.ToLower(folderName)

	for _, excludeName := range excludeNames {
		// Case-insensitive comparison
		if strings.ToLower(excludeName) == lowerFolderName {
			return true
		}
	}
	return false
}

// calculateTotalSize calculates total size of files
func (uc *FolderComparisonUseCase) calculateTotalSize(files []*entities.File) int64 {
	var total int64
	for _, file := range files {
		total += file.Size
	}
	return total
}

// calculateHashesForFiles calculates hashes for files that don't have them
func (uc *FolderComparisonUseCase) calculateHashesForFiles(ctx context.Context, files []*entities.File, progress *entities.Progress, callback func(*entities.Progress)) error {
	// Find files that need hash calculation
	filesNeedingHash := make([]*entities.File, 0)
	for _, file := range files {
		if !file.IsHashCalculated() {
			filesNeedingHash = append(filesNeedingHash, file)
		}
	}

	if len(filesNeedingHash) == 0 {
		log.Println("✅ 모든 파일의 해시가 이미 계산되어 있습니다")
		return nil
	}

	log.Printf("🔐 %d개 파일의 해시 계산 시작", len(filesNeedingHash))

	// Use worker pool for parallel hash calculation
	jobs := make(chan *entities.File, len(filesNeedingHash))
	results := make(chan error, len(filesNeedingHash))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < uc.workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				err := uc.calculateFileHash(ctx, file)
				results <- err
			}
		}()
	}

	// Send jobs
	for _, file := range filesNeedingHash {
		jobs <- file
	}
	close(jobs)

	// Wait for workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	processed := 0
	errors := make([]string, 0)

	for err := range results {
		processed++

		if err != nil {
			errors = append(errors, err.Error())
		}

		// Update progress
		currentProgress := len(files) - len(filesNeedingHash) + processed
		progress.UpdateProgress(currentProgress, fmt.Sprintf("해시 계산 중... (%d/%d)", currentProgress, len(files)))
		uc.progressService.UpdateOperation(ctx, progress.ID, currentProgress, progress.CurrentStep)

		// Call progress callback
		if callback != nil {
			callback(progress)
		}

		// Log progress
		if processed%100 == 0 || processed == len(filesNeedingHash) {
			log.Printf("📈 해시 계산 진행: %d/%d", processed, len(filesNeedingHash))
		}
	}

	if len(errors) > 0 {
		log.Printf("⚠️ %d개 파일의 해시 계산 실패", len(errors))
		return fmt.Errorf("%d개 파일의 해시 계산 실패", len(errors))
	}

	log.Printf("✅ 해시 계산 완료: %d개 파일", len(filesNeedingHash))
	return nil
}

// calculateFileHash calculates hash for a single file
func (uc *FolderComparisonUseCase) calculateFileHash(ctx context.Context, file *entities.File) error {
	hash, err := uc.hashService.CalculateFileHash(ctx, file)
	if err != nil {
		return fmt.Errorf("파일 해시 계산 실패 [%s]: %w", file.ID, err)
	}

	// Update file with hash
	file.SetHash(hash)

	// Save to database
	return uc.fileRepo.UpdateHash(ctx, file.ID, hash)
}

// ComparisonFilesResult holds the result of comparing two sets of files
type ComparisonFilesResult struct {
	Duplicates     []*entities.File // Files in target that exist in source
	UniqueInSource []*entities.File // Files only in source
	UniqueInTarget []*entities.File // Files only in target (can be moved to source)
}

// findDuplicatesBetweenFolders finds duplicate files between source and target folders
func (uc *FolderComparisonUseCase) findDuplicatesBetweenFolders(sourceFiles, targetFiles []*entities.File, deepComparison bool) []*entities.File {
	result := uc.compareFilesWithUniqueTracking(sourceFiles, targetFiles, deepComparison)
	return result.Duplicates
}

// compareFilesWithUniqueTracking compares files and tracks both duplicates and unique files
func (uc *FolderComparisonUseCase) compareFilesWithUniqueTracking(sourceFiles, targetFiles []*entities.File, deepComparison bool) *ComparisonFilesResult {
	result := &ComparisonFilesResult{
		Duplicates:     make([]*entities.File, 0),
		UniqueInSource: make([]*entities.File, 0),
		UniqueInTarget: make([]*entities.File, 0),
	}

	// Create hash map and name+size map of source files
	sourceHashes := make(map[string]*entities.File)
	sourceNameSizes := make(map[string]*entities.File)
	matchedSourceFiles := make(map[string]bool) // Track which source files have matches

	for _, file := range sourceFiles {
		if deepComparison && file.IsHashCalculated() {
			sourceHashes[file.Hash] = file
		}
		// Always build name+size map for fallback
		key := fmt.Sprintf("%s_%d", file.Name, file.Size)
		sourceNameSizes[key] = file
	}

	// Create hash map of target files for tracking matched targets
	targetHashes := make(map[string]*entities.File)
	targetNameSizes := make(map[string]*entities.File)

	for _, file := range targetFiles {
		if deepComparison && file.IsHashCalculated() {
			targetHashes[file.Hash] = file
		}
		key := fmt.Sprintf("%s_%d", file.Name, file.Size)
		targetNameSizes[key] = file
	}

	// Find duplicates and unique files in target
	for _, targetFile := range targetFiles {
		isDuplicate := false
		var matchKey string

		if deepComparison && targetFile.IsHashCalculated() {
			// Hash-based comparison (more accurate)
			if _, exists := sourceHashes[targetFile.Hash]; exists {
				isDuplicate = true
				matchKey = "hash:" + targetFile.Hash
			}
		}

		if !isDuplicate {
			// Name + size comparison (fallback)
			key := fmt.Sprintf("%s_%d", targetFile.Name, targetFile.Size)
			if _, exists := sourceNameSizes[key]; exists {
				isDuplicate = true
				matchKey = "namesize:" + key
			}
		}

		if isDuplicate {
			result.Duplicates = append(result.Duplicates, targetFile)
			matchedSourceFiles[matchKey] = true
		} else {
			result.UniqueInTarget = append(result.UniqueInTarget, targetFile)
		}
	}

	// Find unique files in source (not matched with any target file)
	for _, sourceFile := range sourceFiles {
		var matchKey string
		matched := false

		if deepComparison && sourceFile.IsHashCalculated() {
			matchKey = "hash:" + sourceFile.Hash
			if matchedSourceFiles[matchKey] {
				matched = true
			}
		}

		if !matched {
			key := fmt.Sprintf("%s_%d", sourceFile.Name, sourceFile.Size)
			matchKey = "namesize:" + key
			if matchedSourceFiles[matchKey] {
				matched = true
			}
		}

		if !matched {
			result.UniqueInSource = append(result.UniqueInSource, sourceFile)
		}
	}

	log.Printf("📊 비교 결과: 중복 %d개, 기준폴더만 %d개, 대상폴더만 %d개",
		len(result.Duplicates), len(result.UniqueInSource), len(result.UniqueInTarget))

	return result
}

// DeleteTargetFolderRequest represents the request for deleting target folder
type DeleteTargetFolderRequest struct {
	ComparisonID       int                      `json:"comparisonId"`
	TargetFolderID     string                   `json:"targetFolderId"`
	DeleteEmptyFolders bool                     `json:"deleteEmptyFolders"`
	PermanentDelete    bool                     `json:"permanentDelete"` // true: 완전 삭제, false: 휴지통 이동
	ProgressCallback   func(*entities.Progress) `json:"-"`
	DeletionCallback   func(string, string)     `json:"-"` // fileId, status
}

// DeleteDuplicateFilesRequest represents the request for deleting duplicate files
type DeleteDuplicateFilesRequest struct {
	ComparisonID       int                      `json:"comparisonId"`
	FileIDs            []string                 `json:"fileIds"`
	DeleteEmptyFolders bool                     `json:"deleteEmptyFolders"`
	PermanentDelete    bool                     `json:"permanentDelete"` // true: 완전 삭제, false: 휴지통 이동
	ProgressCallback   func(*entities.Progress) `json:"-"`
	DeletionCallback   func(string, string)     `json:"-"` // fileId, status
}

// DeleteTargetFolderResponse represents the response for deleting target folder
type DeleteTargetFolderResponse struct {
	Progress       *entities.Progress `json:"progress"`
	DeletedFiles   []string           `json:"deletedFiles"`
	DeletedFolders []string           `json:"deletedFolders"`
	FailedFiles    []string           `json:"failedFiles"`
	TotalDeleted   int                `json:"totalDeleted"`
	Errors         []string           `json:"errors,omitempty"`
}

// DeleteDuplicateFilesResponse represents the response for deleting duplicate files
type DeleteDuplicateFilesResponse struct {
	Progress       *entities.Progress `json:"progress"`
	DeletedFiles   []string           `json:"deletedFiles"`
	DeletedFolders []string           `json:"deletedFolders"`
	FailedFiles    []string           `json:"failedFiles"`
	TotalDeleted   int                `json:"totalDeleted"`
	Errors         []string           `json:"errors,omitempty"`
}

// DeleteTargetFolder deletes the entire target folder (when 100% duplicated)
func (uc *FolderComparisonUseCase) DeleteTargetFolder(ctx context.Context, req *DeleteTargetFolderRequest) (*DeleteTargetFolderResponse, error) {
	log.Printf("🗑️ 대상 폴더 전체 삭제 시작: 폴더 ID %s", req.TargetFolderID)

	// Get comparison result to verify 100% duplication
	comparison, err := uc.comparisonRepo.GetByID(ctx, req.ComparisonID)
	if err != nil {
		return nil, fmt.Errorf("비교 결과 조회 실패: %w", err)
	}

	if comparison == nil {
		return nil, fmt.Errorf("비교 결과를 찾을 수 없습니다: %d", req.ComparisonID)
	}

	// Verify 100% duplication
	if comparison.DuplicationPercentage < 100.0 {
		return nil, fmt.Errorf("대상 폴더가 100%% 중복이 아닙니다 (%.1f%% 중복)", comparison.DuplicationPercentage)
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileCleanup, 1)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &DeleteTargetFolderResponse{
		Progress:       progress,
		DeletedFiles:   make([]string, 0),
		DeletedFolders: make([]string, 0),
		FailedFiles:    make([]string, 0),
		Errors:         make([]string, 0),
	}

	// Start deletion in background
	go uc.performTargetFolderDeletion(context.Background(), req, progress, comparison, response)

	return response, nil
}

// DeleteDuplicateFiles deletes specific duplicate files from target folder
func (uc *FolderComparisonUseCase) DeleteDuplicateFiles(ctx context.Context, req *DeleteDuplicateFilesRequest) (*DeleteDuplicateFilesResponse, error) {
	log.Printf("🗑️ 중복 파일 삭제 시작: %d개 파일", len(req.FileIDs))

	// Get comparison result
	comparison, err := uc.comparisonRepo.GetByID(ctx, req.ComparisonID)
	if err != nil {
		return nil, fmt.Errorf("비교 결과 조회 실패: %w", err)
	}

	if comparison == nil {
		return nil, fmt.Errorf("비교 결과를 찾을 수 없습니다: %d", req.ComparisonID)
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileCleanup, len(req.FileIDs))
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &DeleteDuplicateFilesResponse{
		Progress:       progress,
		DeletedFiles:   make([]string, 0),
		DeletedFolders: make([]string, 0),
		FailedFiles:    make([]string, 0),
		Errors:         make([]string, 0),
	}

	// Start deletion in background
	go uc.performDuplicateFilesDeletion(context.Background(), req, progress, comparison, response)

	return response, nil
}

// performTargetFolderDeletion performs the actual target folder deletion
func (uc *FolderComparisonUseCase) performTargetFolderDeletion(ctx context.Context, req *DeleteTargetFolderRequest, progress *entities.Progress, comparison *entities.ComparisonResult, response *DeleteTargetFolderResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 대상 폴더 삭제 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	actionType := "삭제"
	if !req.PermanentDelete {
		actionType = "휴지통 이동"
	}
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, fmt.Sprintf("대상 폴더 %s 시작...", actionType))

	// Delete or trash the target folder based on permanentDelete flag
	var err error
	if req.PermanentDelete {
		log.Printf("🗑️ 대상 폴더 완전 삭제: %s", req.TargetFolderID)
		err = uc.storageProvider.DeleteFolder(ctx, req.TargetFolderID)
	} else {
		log.Printf("📥 대상 폴더 휴지통 이동: %s", req.TargetFolderID)
		err = uc.storageProvider.TrashFolder(ctx, req.TargetFolderID)
	}
	if err != nil {
		log.Printf("❌ 대상 폴더 %s 실패: %v", actionType, err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("대상 폴더 %s 실패: %v", actionType, err))
		response.Errors = append(response.Errors, fmt.Sprintf("대상 폴더 %s 실패: %v", actionType, err))
		return
	}

	// Update response
	response.DeletedFolders = append(response.DeletedFolders, req.TargetFolderID)
	response.TotalDeleted = 1

	// Call deletion callback
	if req.DeletionCallback != nil {
		req.DeletionCallback(req.TargetFolderID, "deleted")
	}

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
	}

	log.Printf("✅ 대상 폴더 삭제 완료: %s", req.TargetFolderID)
}

// performDuplicateFilesDeletion performs the actual duplicate files deletion
func (uc *FolderComparisonUseCase) performDuplicateFilesDeletion(ctx context.Context, req *DeleteDuplicateFilesRequest, progress *entities.Progress, comparison *entities.ComparisonResult, response *DeleteDuplicateFilesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 중복 파일 삭제 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "중복 파일 삭제 시작...")

	// Track folders that might become empty
	affectedFolders := make(map[string]bool)
	// Cache folder parent/name info collected during ancestor traversal
	folderParentCache := make(map[string]string) // folderID -> parentID
	folderNameCache := make(map[string]string)   // folderID -> name

	// Pre-collect parent folders for empty folder cleanup
	if req.DeleteEmptyFolders {
		log.Printf("📁 빈 폴더 정리를 위한 부모 폴더 정보 수집 중...")
		uc.collectParentFoldersFromComparisonWithCache(ctx, comparison, req.FileIDs, affectedFolders, folderParentCache, folderNameCache)
	}

	// Use batch deletion with parallel processing (configurable)
	batchSize := 10             // Default batch size
	progressUpdateInterval := 5 // Default progress update interval

	// Use configuration if available (you'll need to inject config into UseCase)
	// For now, use defaults but make them configurable later
	totalFiles := len(req.FileIDs)

	log.Printf("🚀 병렬 파일 삭제 시작: %d개 파일, 배치 크기: %d", totalFiles, batchSize)

	for i := 0; i < totalFiles; i += batchSize {
		end := i + batchSize
		if end > totalFiles {
			end = totalFiles
		}

		batch := req.FileIDs[i:end]
		uc.deleteBatchFiles(ctx, batch, req, response)

		// Update progress less frequently (per batch instead of per file)
		progress.UpdateProgress(end, fmt.Sprintf("파일 삭제 중... (%d/%d)", end, totalFiles))

		// Only update database every N batches or at end (configurable)
		if (i/batchSize)%progressUpdateInterval == 0 || end == totalFiles {
			uc.progressService.UpdateOperation(ctx, progress.ID, end, progress.CurrentStep)
		}

		// Call progress callback
		if req.ProgressCallback != nil {
			req.ProgressCallback(progress)
		}
	}

	// Delete empty folders if requested
	if req.DeleteEmptyFolders && len(affectedFolders) > 0 {
		uc.progressService.UpdateOperation(ctx, progress.ID, len(req.FileIDs), "빈 폴더 정리 중...")
		log.Printf("🧹 빈 폴더 정리 시작: %d개 폴더 확인", len(affectedFolders))

		deletedFolders := uc.cleanupEmptyFolders(ctx, affectedFolders, req.PermanentDelete, progress, folderParentCache, folderNameCache)
		response.DeletedFolders = append(response.DeletedFolders, deletedFolders...)

		if len(deletedFolders) > 0 {
			log.Printf("✅ %d개 빈 폴더 정리 완료", len(deletedFolders))
		}
	}

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
	}

	log.Printf("✅ 중복 파일 삭제 완료: %d개 성공, %d개 실패", len(response.DeletedFiles), len(response.FailedFiles))
}

// folderDepthInfo stores folder ID and its depth from root
type folderDepthInfo struct {
	ID    string
	Depth int
}

// cleanupEmptyFolders removes empty folders using depth-first approach
// Uses cached parent/name data to avoid redundant API calls for depth calculation
func (uc *FolderComparisonUseCase) cleanupEmptyFolders(ctx context.Context, folderIDs map[string]bool, permanentDelete bool, progress *entities.Progress, parentCache map[string]string, nameCache map[string]string) []string {
	var deletedFolders []string

	actionType := "삭제"
	if !permanentDelete {
		actionType = "휴지통 이동"
	}

	// Step 1: Calculate depth for each folder using cached parent data (no API calls)
	foldersWithDepth := make([]folderDepthInfo, 0, len(folderIDs))
	for folderID := range folderIDs {
		depth := uc.getFolderDepthFromCache(folderID, parentCache)
		foldersWithDepth = append(foldersWithDepth, folderDepthInfo{ID: folderID, Depth: depth})
	}

	// Step 2: Sort by depth (deepest first)
	sort.Slice(foldersWithDepth, func(i, j int) bool {
		return foldersWithDepth[i].Depth > foldersWithDepth[j].Depth
	})

	log.Printf("🧹 빈 폴더 정리 시작: %d개 폴더 (깊이순 정렬됨, 캐시 사용)", len(foldersWithDepth))

	// Initialize cleanup metadata for real-time progress tracking
	cleanupFolders := make([]map[string]string, 0)
	totalFolders := len(foldersWithDepth)
	processedCount := 0

	if progress != nil {
		progress.SetMetadata("cleanupTotal", totalFolders)
		progress.SetMetadata("cleanupProcessed", 0)
		progress.SetMetadata("cleanupFolders", cleanupFolders)
		uc.progressService.SetOperationMetadata(ctx, progress.ID, progress.Metadata)
	}

	// Step 3: Pre-check which folders are empty using parallel API calls
	type emptyCheckResult struct {
		folderID string
		isEmpty  bool
		err      error
	}

	emptyCache := make(map[string]bool)     // folderID -> isEmpty
	emptyCheckErr := make(map[string]error)  // folderID -> error
	deletedSet := make(map[string]bool)

	// Batch empty checks with bounded concurrency
	const checkWorkers = 10
	sem := make(chan struct{}, checkWorkers)
	resultsCh := make(chan emptyCheckResult, len(foldersWithDepth))

	log.Printf("🔍 빈 폴더 확인 시작: %d개 폴더 (병렬 %d)", len(foldersWithDepth), checkWorkers)

	var wg sync.WaitGroup
	for _, fi := range foldersWithDepth {
		wg.Add(1)
		go func(folderID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			isEmpty, err := uc.isFolderEmptySimple(ctx, folderID)
			resultsCh <- emptyCheckResult{folderID: folderID, isEmpty: isEmpty, err: err}
		}(fi.ID)
	}

	// Collect results in background
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	checkedCount := 0
	for result := range resultsCh {
		if result.err != nil {
			emptyCheckErr[result.folderID] = result.err
		} else {
			emptyCache[result.folderID] = result.isEmpty
		}
		checkedCount++
		if checkedCount%500 == 0 {
			log.Printf("🔍 빈 폴더 확인 중: %d/%d", checkedCount, len(foldersWithDepth))
		}
	}

	log.Printf("🔍 빈 폴더 확인 완료: %d개 중 빈 폴더 %d개", len(foldersWithDepth), func() int {
		count := 0
		for _, v := range emptyCache {
			if v {
				count++
			}
		}
		return count
	}())

	// Step 4: Process folders from deepest to shallowest
	for _, folderInfo := range foldersWithDepth {
		folderID := folderInfo.ID

		// Skip if already deleted
		if deletedSet[folderID] {
			continue
		}

		// Get folder name from cache (no API call)
		folderName := folderID
		if name, ok := nameCache[folderID]; ok {
			folderName = name
		}

		// Use pre-checked empty status
		if checkErr, hasErr := emptyCheckErr[folderID]; hasErr {
			log.Printf("⚠️ 폴더 내용 확인 실패 [%s]: %v", folderID, checkErr)
			processedCount++
			cleanupFolders = append(cleanupFolders, map[string]string{"name": folderName, "status": "failed"})
			if progress != nil {
				progress.SetMetadata("cleanupProcessed", processedCount)
				progress.SetMetadata("cleanupFolders", cleanupFolders)
				uc.progressService.SetOperationMetadata(ctx, progress.ID, progress.Metadata)
			}
			continue
		}

		isEmpty := emptyCache[folderID]

		// Double-check: if a parent of a non-empty folder was marked empty in simple check,
		// it might actually have non-empty subfolders. For folders marked empty, verify
		// subfolders are also empty or already deleted.
		if isEmpty {
			isEmpty = uc.isFolderEffectivelyEmpty(folderID, emptyCache, deletedSet, parentCache, folderIDs)
		}

		if isEmpty {
			log.Printf("🗑️ 빈 폴더 %s: %s (깊이: %d)", actionType, folderName, folderInfo.Depth)
			var deleteErr error
			if permanentDelete {
				deleteErr = uc.storageProvider.DeleteFolder(ctx, folderID)
			} else {
				deleteErr = uc.storageProvider.TrashFolder(ctx, folderID)
			}
			if deleteErr != nil {
				log.Printf("❌ 빈 폴더 %s 실패 [%s]: %v", actionType, folderName, deleteErr)
				cleanupFolders = append(cleanupFolders, map[string]string{"name": folderName, "status": "failed"})
			} else {
				log.Printf("✅ 빈 폴더 %s 완료: %s", actionType, folderName)
				deletedFolders = append(deletedFolders, folderID)
				deletedSet[folderID] = true
				cleanupFolders = append(cleanupFolders, map[string]string{"name": folderName, "status": "deleted"})
			}
		} else {
			cleanupFolders = append(cleanupFolders, map[string]string{"name": folderName, "status": "skipped"})
		}

		processedCount++
		if progress != nil && (processedCount%10 == 0 || processedCount == totalFolders) {
			progress.SetMetadata("cleanupProcessed", processedCount)
			progress.SetMetadata("cleanupFolders", cleanupFolders)
			uc.progressService.SetOperationMetadata(ctx, progress.ID, progress.Metadata)
		}
	}

	return deletedFolders
}

// getFolderDepthFromCache calculates folder depth using cached parent relationships (zero API calls)
func (uc *FolderComparisonUseCase) getFolderDepthFromCache(folderID string, parentCache map[string]string) int {
	depth := 0
	currentID := folderID

	visited := make(map[string]bool) // prevent infinite loops
	for depth < 50 {
		if currentID == "" || currentID == "root" {
			break
		}
		if visited[currentID] {
			break
		}
		visited[currentID] = true
		depth++

		parentID, ok := parentCache[currentID]
		if !ok || parentID == "" || parentID == "root" {
			break
		}
		currentID = parentID
	}

	return depth
}

// isFolderEmptySimple checks if a folder directly contains no files (non-recursive, single API call)
func (uc *FolderComparisonUseCase) isFolderEmptySimple(ctx context.Context, folderID string) (bool, error) {
	items, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return false, err
	}

	for _, item := range items {
		isFolder := item.MimeType == "application/vnd.google-apps.folder"
		if !isFolder {
			// Has a file → not empty
			return false, nil
		}
	}

	// Only contains folders (or nothing)
	return true, nil
}

// isFolderEffectivelyEmpty checks if a folder and all its child folders in the affected set are empty
// Uses only cached data, no API calls
func (uc *FolderComparisonUseCase) isFolderEffectivelyEmpty(folderID string, emptyCache map[string]bool, deletedSet map[string]bool, parentCache map[string]string, affectedFolders map[string]bool) bool {
	// Find child folders of this folder from the affected set
	for childID := range affectedFolders {
		if parentCache[childID] == folderID && childID != folderID {
			// This child is under our folder
			if deletedSet[childID] {
				continue // already deleted, fine
			}
			if !emptyCache[childID] {
				return false // child has files, so parent is not effectively empty
			}
		}
	}
	return true
}

// isFolderEmptyOfFiles checks if a folder contains no files (only empty subfolders are OK)
func (uc *FolderComparisonUseCase) isFolderEmptyOfFiles(ctx context.Context, folderID string, deletedFolders map[string]bool) (bool, error) {
	// Get all items in the folder
	items, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return false, err
	}

	for _, item := range items {
		// Check if it's a folder
		isFolder := item.MimeType == "application/vnd.google-apps.folder"

		if isFolder {
			// If it's a folder that was already deleted, skip it
			if deletedFolders[item.ID] {
				continue
			}

			// Check if this subfolder is also empty of files (recursive)
			subfolderEmpty, err := uc.isFolderEmptyOfFiles(ctx, item.ID, deletedFolders)
			if err != nil {
				return false, err
			}
			if !subfolderEmpty {
				// Subfolder has files, so this folder is not empty
				return false, nil
			}
		} else {
			// It's a file, so folder is not empty
			return false, nil
		}
	}

	// All items are either deleted folders or empty subfolders
	return true, nil
}

// collectParentFoldersFromComparisonWithCache collects ALL ancestor folders and caches parent/name info
// Uses parallel API calls for initial parent folders, then sequential for ancestor chains
func (uc *FolderComparisonUseCase) collectParentFoldersFromComparisonWithCache(ctx context.Context, comparison *entities.ComparisonResult, fileIDs []string, affectedFolders map[string]bool, parentCache map[string]string, nameCache map[string]string) {
	// Create a map of file IDs to delete for quick lookup
	deleteFileMap := make(map[string]bool)
	for _, fileID := range fileIDs {
		deleteFileMap[fileID] = true
	}

	// Extract immediate parent folders from duplicate files
	immediateParents := make(map[string]bool)
	for _, file := range comparison.DuplicateFiles {
		if deleteFileMap[file.ID] && len(file.Parents) > 0 {
			immediateParents[file.Parents[0]] = true
		}
	}

	log.Printf("📁 직접 부모 폴더 %d개 수집됨, 병렬 조상 탐색 시작...", len(immediateParents))

	// Parallel fetch folder info for all unique folders using BFS approach
	// Start with immediate parents, then expand to their parents in waves
	var mu sync.Mutex
	toFetch := make([]string, 0, len(immediateParents))
	for id := range immediateParents {
		if id != "" && id != "root" {
			toFetch = append(toFetch, id)
		}
	}

	const fetchWorkers = 10
	wave := 0

	for len(toFetch) > 0 {
		wave++
		currentBatch := toFetch
		toFetch = nil // reset for next wave

		if wave <= 3 || wave%5 == 0 {
			log.Printf("📁 조상 폴더 탐색 wave %d: %d개 폴더 조회 중 (병렬 %d)", wave, len(currentBatch), fetchWorkers)
		}

		type folderResult struct {
			id       string
			parentID string
			name     string
			err      error
		}

		resultsCh := make(chan folderResult, len(currentBatch))
		sem := make(chan struct{}, fetchWorkers)
		var wg sync.WaitGroup

		for _, folderID := range currentBatch {
			wg.Add(1)
			go func(fid string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				folderInfo, err := uc.storageProvider.GetFolder(ctx, fid)
				if err != nil {
					resultsCh <- folderResult{id: fid, err: err}
					return
				}
				parentID := ""
				if len(folderInfo.Parents) > 0 {
					parentID = folderInfo.Parents[0]
				}
				resultsCh <- folderResult{id: fid, parentID: parentID, name: folderInfo.Name}
			}(folderID)
		}

		go func() {
			wg.Wait()
			close(resultsCh)
		}()

		// Collect results and find next wave of parents to fetch
		for r := range resultsCh {
			if r.err != nil {
				log.Printf("⚠️ 폴더 정보 조회 실패 [%s]: %v", r.id, r.err)
				continue
			}

			mu.Lock()
			affectedFolders[r.id] = true
			nameCache[r.id] = r.name
			if r.parentID != "" {
				parentCache[r.id] = r.parentID
			}

			// Queue parent for next wave if not yet visited
			if r.parentID != "" && r.parentID != "root" && !affectedFolders[r.parentID] {
				affectedFolders[r.parentID] = true // mark visited to avoid duplicates
				toFetch = append(toFetch, r.parentID)
			}
			mu.Unlock()
		}
	}

	log.Printf("📁 전체 상위 폴더 %d개 수집됨 (wave %d회, 캐시 %d개)", len(affectedFolders), wave, len(parentCache))
}

// deleteBatchFiles deletes a batch of files concurrently
func (uc *FolderComparisonUseCase) deleteBatchFiles(ctx context.Context, fileIDs []string, req *DeleteDuplicateFilesRequest, response *DeleteDuplicateFilesResponse) {
	// Use goroutines for concurrent deletion
	jobs := make(chan string, len(fileIDs))
	results := make(chan deleteResult, len(fileIDs))

	// Worker pool for parallel deletion
	const numWorkers = 5 // Limit concurrent deletions to avoid rate limits
	for w := 0; w < numWorkers; w++ {
		go func() {
			for fileID := range jobs {
				result := uc.deleteFileWithCallback(ctx, fileID, req.PermanentDelete, req.DeletionCallback)
				results <- result
			}
		}()
	}

	// Send jobs
	for _, fileID := range fileIDs {
		jobs <- fileID
	}
	close(jobs)

	// Collect results
	for range fileIDs {
		result := <-results

		if result.err != nil {
			log.Printf("❌ 파일 삭제 실패 [%s]: %v", result.fileID, result.err)
			response.FailedFiles = append(response.FailedFiles, result.fileID)
			response.Errors = append(response.Errors, fmt.Sprintf("파일 삭제 실패 [%s]: %v", result.fileID, result.err))
		} else {
			log.Printf("✅ 파일 삭제 완료: %s", result.fileID)
			response.DeletedFiles = append(response.DeletedFiles, result.fileID)
			response.TotalDeleted++
		}
	}
}

// deleteResult represents the result of a single file deletion
type deleteResult struct {
	fileID string
	err    error
}

// deleteFileWithCallback deletes a single file with callback notifications
func (uc *FolderComparisonUseCase) deleteFileWithCallback(ctx context.Context, fileID string, permanentDelete bool, callback func(string, string)) deleteResult {
	// Call deletion callback - mark as processing
	if callback != nil {
		if permanentDelete {
			callback(fileID, "deleting")
		} else {
			callback(fileID, "trashing")
		}
	}

	// Delete or trash file based on permanentDelete flag
	var err error
	if permanentDelete {
		err = uc.storageProvider.DeleteFile(ctx, fileID)
	} else {
		err = uc.storageProvider.TrashFile(ctx, fileID)
	}

	// Call deletion callback with result
	if callback != nil {
		if err != nil {
			callback(fileID, "failed")
		} else {
			if permanentDelete {
				callback(fileID, "deleted")
			} else {
				callback(fileID, "trashed")
			}
		}
	}

	return deleteResult{
		fileID: fileID,
		err:    err,
	}
}

// saveFilesToDatabase saves file metadata to database to satisfy foreign key constraints
func (uc *FolderComparisonUseCase) saveFilesToDatabase(ctx context.Context, files []*entities.File) error {
	if len(files) == 0 {
		return nil
	}

	log.Printf("💾 데이터베이스에 %d개 파일 메타데이터 저장", len(files))

	// Save files in batches to avoid overwhelming the database
	const batchSize = 100
	for i := 0; i < len(files); i += batchSize {
		end := i + batchSize
		if end > len(files) {
			end = len(files)
		}

		batch := files[i:end]
		for _, file := range batch {
			// Use upsert to handle duplicates gracefully
			err := uc.fileRepo.Save(ctx, file)
			if err != nil {
				// Log error but continue with other files
				log.Printf("⚠️ 파일 저장 실패 [%s]: %v", file.ID, err)
				continue
			}
		}

		log.Printf("📁 배치 저장 완료: %d/%d", end, len(files))
	}

	log.Printf("✅ 모든 파일 메타데이터 저장 완료")
	return nil
}

// ExtractFolderIdFromUrl extracts Google Drive folder ID from URL
func (uc *FolderComparisonUseCase) ExtractFolderIdFromUrl(url string) (string, error) {
	// Google Drive folder URL patterns:
	// https://drive.google.com/drive/folders/FOLDER_ID
	// https://drive.google.com/drive/u/0/folders/FOLDER_ID
	// https://drive.google.com/open?id=FOLDER_ID

	log.Printf("🔍 Extracting folder ID from URL: %s", url)

	// Try different patterns - Google Drive IDs can contain letters, numbers, underscores, hyphens
	patterns := []string{
		`/folders/([a-zA-Z0-9_-]+)(?:[/?#]|$)`, // More precise pattern with end boundary
		`[?&]id=([a-zA-Z0-9_-]+)(?:[&]|$)`,     // More precise pattern for query parameter
	}

	for _, pattern := range patterns {
		if matches := regexp.MustCompile(pattern).FindStringSubmatch(url); len(matches) > 1 {
			folderID := matches[1]
			log.Printf("✅ Extracted folder ID: %s", folderID)
			return folderID, nil
		}
	}

	// If it's already just an ID, return as is (Google Drive IDs are typically 28-44 characters)
	if regexp.MustCompile(`^[a-zA-Z0-9_-]{10,}$`).MatchString(url) {
		log.Printf("✅ Input is already a folder ID: %s", url)
		return url, nil
	}

	log.Printf("❌ Failed to extract folder ID from URL: %s", url)
	return "", fmt.Errorf("Google Drive 폴더 URL에서 ID를 추출할 수 없습니다: %s", url)
}

// FindDuplicatesInSingleFolderRequest represents the request for finding duplicates in a single folder
type FindDuplicatesInSingleFolderRequest struct {
	FolderID          string `json:"folderId"`
	IncludeSubfolders bool   `json:"includeSubfolders"`
	MinFileSize       int64  `json:"minFileSize"`
	ForceNewScan      bool   `json:"forceNewScan"`
}

// FindDuplicatesInSingleFolderResponse represents the response for single folder duplicate finding
type FindDuplicatesInSingleFolderResponse struct {
	Progress        *entities.Progress         `json:"progress"`
	ProgressId      int                        `json:"progressId"` // For easier frontend access
	DuplicateGroups []*entities.DuplicateGroup `json:"duplicateGroups,omitempty"`
	TotalFiles      int                        `json:"totalFiles"`
	DuplicateFiles  int                        `json:"duplicateFiles"`
	WastedSpace     int64                      `json:"wastedSpace"`
	Errors          []string                   `json:"errors,omitempty"`
}

// FindDuplicatesInSingleFolder finds duplicate files within a single folder
func (uc *FolderComparisonUseCase) FindDuplicatesInSingleFolder(ctx context.Context, req *FindDuplicatesInSingleFolderRequest) (*FindDuplicatesInSingleFolderResponse, error) {
	log.Printf("📁 단일 폴더 내 중복 파일 검색 시작: %s", req.FolderID)

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, "single_folder_duplicates", 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Set metadata for checkpoint (DB에 저장)
	uc.progressService.SetOperationMetadata(ctx, progress.ID, map[string]interface{}{
		"folderId":          req.FolderID,
		"includeSubfolders": req.IncludeSubfolders,
		"minFileSize":       req.MinFileSize,
		"currentPhase":      "initialized",
	})

	// Initialize response
	response := &FindDuplicatesInSingleFolderResponse{
		Progress:   progress,
		ProgressId: progress.ID,
		Errors:     make([]string, 0),
	}

	// Start scanning in background
	go uc.performSingleFolderDuplicateScan(context.Background(), req, progress, response)

	return response, nil
}

// performSingleFolderDuplicateScan performs the actual duplicate scanning in background
func (uc *FolderComparisonUseCase) performSingleFolderDuplicateScan(ctx context.Context, req *FindDuplicatesInSingleFolderRequest, progress *entities.Progress, response *FindDuplicatesInSingleFolderResponse) {
	defer func() {
		log.Printf("🔚 백그라운드 단일 폴더 중복 검색 작업 종료 - Progress ID: %d", progress.ID)
	}()

	// Phase 1: Scan folder for files
	progress.SetMetadata("currentPhase", "scanning_files")
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "폴더 파일 스캔 중...")

	files, err := uc.getFilesRecursive(ctx, req.FolderID)
	if err != nil {
		log.Printf("❌ 폴더 파일 스캔 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("폴더 파일 스캔 실패: %v", err))
		return
	}

	log.Printf("📊 스캔 완료: %d개 파일 발견", len(files))
	response.TotalFiles = len(files)

	// Filter files by size if specified
	if req.MinFileSize > 0 {
		filteredFiles := make([]*entities.File, 0)
		for _, file := range files {
			if file.Size >= req.MinFileSize {
				filteredFiles = append(filteredFiles, file)
			}
		}
		files = filteredFiles
		log.Printf("📏 크기 필터 적용: %d개 파일 (최소 %d bytes)", len(files), req.MinFileSize)
	}

	if len(files) == 0 {
		log.Printf("⚠️ 스캔할 파일이 없습니다")
		uc.progressService.CompleteOperation(ctx, progress.ID)
		return
	}

	// Phase 2: Save files to database for hash calculation
	progress.SetMetadata("currentPhase", "saving_files")
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 메타데이터 저장 중...")

	err = uc.saveFilesToDatabase(ctx, files)
	if err != nil {
		log.Printf("❌ 파일 메타데이터 저장 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("파일 메타데이터 저장 실패: %v", err))
		return
	}

	// Phase 3: Calculate hashes and find duplicates
	progress.SetMetadata("currentPhase", "calculating_hashes")
	progress.TotalItems = len(files)
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 해시 계산 및 중복 검색 중...")

	duplicateGroups, err := uc.findDuplicatesWithHashes(ctx, files, progress)
	if err != nil {
		log.Printf("❌ 중복 파일 검색 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("중복 파일 검색 실패: %v", err))
		return
	}

	// Calculate statistics
	totalDuplicateFiles := 0
	wastedSpace := int64(0)
	for _, group := range duplicateGroups {
		if group.Count > 1 {
			totalDuplicateFiles += group.Count
			wastedSpace += int64(group.Count-1) * group.Files[0].Size
		}
	}

	response.DuplicateGroups = duplicateGroups
	response.DuplicateFiles = totalDuplicateFiles
	response.WastedSpace = wastedSpace

	log.Printf("✅ 단일 폴더 중복 검색 완료: %d개 중복 그룹, %d개 중복 파일, %d bytes 절약 가능",
		len(duplicateGroups), totalDuplicateFiles, wastedSpace)

	// 결과를 progress 메타데이터에 저장하여 프론트엔드에서 조회 가능하도록 함
	resultData := map[string]interface{}{
		"duplicateGroups": duplicateGroups,
		"totalFiles":      response.TotalFiles,
		"duplicateFiles":  totalDuplicateFiles,
		"wastedSpace":     wastedSpace,
	}
	uc.progressService.SetOperationMetadata(ctx, progress.ID, map[string]interface{}{
		"result": resultData,
	})

	uc.progressService.CompleteOperation(ctx, progress.ID)
}

// findDuplicatesWithHashes finds duplicate files by calculating hashes
func (uc *FolderComparisonUseCase) findDuplicatesWithHashes(ctx context.Context, files []*entities.File, progress *entities.Progress) ([]*entities.DuplicateGroup, error) {
	hashToFiles := make(map[string][]*entities.File)

	for i, file := range files {
		// Calculate hash if not already calculated
		if file.Hash == "" {
			hash, err := uc.hashService.CalculateFileHash(ctx, file)
			if err != nil {
				log.Printf("⚠️ 파일 해시 계산 실패 (건너뜀): %s - %v", file.Name, err)
				continue
			}
			file.Hash = hash

			// Update file in database
			uc.fileRepo.Update(ctx, file)
		}

		// Group files by hash
		hashToFiles[file.Hash] = append(hashToFiles[file.Hash], file)

		// Update progress
		uc.progressService.UpdateOperation(ctx, progress.ID, i+1, fmt.Sprintf("해시 계산 중... (%d/%d)", i+1, len(files)))
	}

	// Create duplicate groups from files with same hash
	duplicateGroups := make([]*entities.DuplicateGroup, 0)
	for hash, groupFiles := range hashToFiles {
		if len(groupFiles) > 1 {
			group := entities.NewDuplicateGroup(hash)
			for _, file := range groupFiles {
				group.AddFile(file)
			}
			duplicateGroups = append(duplicateGroups, group)
		}
	}

	return duplicateGroups, nil
}

// SetConfiguration sets the use case configuration
func (uc *FolderComparisonUseCase) SetConfiguration(workerCount int, includeSubfolders, deepComparison bool, minFileSize int64) {
	if workerCount > 0 {
		uc.workerCount = workerCount
	}
	uc.includeSubfolders = includeSubfolders
	uc.deepComparison = deepComparison
	if minFileSize >= 0 {
		uc.minFileSize = minFileSize
	}
}

// MoveUniqueFilesRequest represents the request for moving unique files
type MoveUniqueFilesRequest struct {
	ComparisonID     int                      `json:"comparisonId"`
	PreservePath     bool                     `json:"preservePath"` // Preserve folder structure when moving
	OnConflict       string                   `json:"onConflict"`   // "rename", "skip", "overwrite"
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// MoveUniqueFilesResponse represents the response for moving unique files
type MoveUniqueFilesResponse struct {
	Progress       *entities.Progress `json:"progress"`
	MovedCount     int                `json:"movedCount"`
	FailedCount    int                `json:"failedCount"`
	SkippedCount   int                `json:"skippedCount"`
	MovedFiles     []string           `json:"movedFiles"`
	FailedFiles    []string           `json:"failedFiles"`
	CreatedFolders []string           `json:"createdFolders"`
	Errors         []string           `json:"errors,omitempty"`
}

// MoveUniqueFilesToSource moves unique files from target folder to source folder
func (uc *FolderComparisonUseCase) MoveUniqueFilesToSource(ctx context.Context, req *MoveUniqueFilesRequest) (*MoveUniqueFilesResponse, error) {
	log.Printf("📦 비중복 파일 이동 시작 - ComparisonID: %d, 경로유지: %v, 충돌처리: %s",
		req.ComparisonID, req.PreservePath, req.OnConflict)

	// Get comparison result
	comparison, err := uc.comparisonRepo.GetByID(ctx, req.ComparisonID)
	if err != nil {
		return nil, fmt.Errorf("비교 결과 조회 실패: %w", err)
	}
	if comparison == nil {
		return nil, fmt.Errorf("비교 결과를 찾을 수 없습니다: %d", req.ComparisonID)
	}

	// Load unique files from database
	var uniqueFiles []*entities.File
	if repo, ok := uc.comparisonRepo.(interface {
		GetUnmovedUniqueFiles(ctx context.Context, comparisonID int) ([]*entities.File, error)
	}); ok {
		uniqueFiles, err = repo.GetUnmovedUniqueFiles(ctx, req.ComparisonID)
		if err != nil {
			return nil, fmt.Errorf("비중복 파일 조회 실패: %w", err)
		}
	} else {
		// Fallback: use files from comparison result
		uniqueFiles = comparison.UniqueInTarget
	}

	if len(uniqueFiles) == 0 {
		return &MoveUniqueFilesResponse{
			MovedCount:     0,
			MovedFiles:     make([]string, 0),
			FailedFiles:    make([]string, 0),
			CreatedFolders: make([]string, 0),
		}, nil
	}

	log.Printf("📋 이동 대상 파일: %d개", len(uniqueFiles))

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, "move_unique_files", len(uniqueFiles))
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Store comparisonId in metadata for recovery after page refresh
	progress.SetMetadata("comparisonId", req.ComparisonID)
	uc.progressService.SetOperationMetadata(ctx, progress.ID, progress.Metadata)

	// Initialize response
	response := &MoveUniqueFilesResponse{
		Progress:       progress,
		MovedFiles:     make([]string, 0),
		FailedFiles:    make([]string, 0),
		CreatedFolders: make([]string, 0),
		Errors:         make([]string, 0),
	}

	// Start moving files in background with cancellable context
	bgCtx, cancel := context.WithCancel(context.Background())

	// Store the cancel function for later cancellation
	uc.runningOpsMu.Lock()
	uc.runningOps[progress.ID] = cancel
	uc.runningOpsMu.Unlock()

	go uc.performUniqueFilesMove(bgCtx, req, comparison, uniqueFiles, progress, response)

	return response, nil
}

// CancelMoveUniqueFiles cancels a running move unique files operation
func (uc *FolderComparisonUseCase) CancelMoveUniqueFiles(ctx context.Context, progressID int) error {
	log.Printf("🛑 비중복 파일 이동 취소 요청: Progress ID %d", progressID)

	uc.runningOpsMu.RLock()
	cancel, exists := uc.runningOps[progressID]
	uc.runningOpsMu.RUnlock()

	if !exists {
		return fmt.Errorf("실행 중인 이동 작업을 찾을 수 없습니다: %d", progressID)
	}

	// Call the cancel function
	cancel()

	// Update progress status
	uc.progressService.FailOperation(ctx, progressID, "사용자에 의해 취소됨")

	// Remove from running operations
	uc.runningOpsMu.Lock()
	delete(uc.runningOps, progressID)
	uc.runningOpsMu.Unlock()

	log.Printf("✅ 비중복 파일 이동 취소 완료: Progress ID %d", progressID)
	return nil
}

// ActiveMoveOperation represents an active move operation with its associated comparisonId
type ActiveMoveOperation struct {
	ProgressID   int    `json:"progressId"`
	ComparisonID int    `json:"comparisonId,omitempty"`
	Status       string `json:"status"`
	CurrentStep  string `json:"currentStep,omitempty"`
}

// GetActiveMoveOperations returns all active (running/in_progress) move_unique_files operations
func (uc *FolderComparisonUseCase) GetActiveMoveOperations(ctx context.Context) ([]ActiveMoveOperation, error) {
	ops, err := uc.progressService.GetOperationsByType(ctx, "move_unique_files")
	if err != nil {
		return nil, fmt.Errorf("이동 작업 조회 실패: %w", err)
	}

	var active []ActiveMoveOperation
	for _, op := range ops {
		if op.Status != "running" && op.Status != "in_progress" {
			continue
		}
		amo := ActiveMoveOperation{
			ProgressID:  op.ID,
			Status:      op.Status,
			CurrentStep: op.CurrentStep,
		}
		if op.Metadata != nil {
			if cid, ok := op.Metadata["comparisonId"]; ok {
				switch v := cid.(type) {
				case float64:
					amo.ComparisonID = int(v)
				case int:
					amo.ComparisonID = v
				}
			}
		}
		active = append(active, amo)
	}
	return active, nil
}

// performUniqueFilesMove performs the actual file moving operation
func (uc *FolderComparisonUseCase) performUniqueFilesMove(ctx context.Context, req *MoveUniqueFilesRequest, comparison *entities.ComparisonResult, files []*entities.File, progress *entities.Progress, response *MoveUniqueFilesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 파일 이동 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 이동 준비 중...")

	// Cache for created folders (path -> folder ID)
	createdFoldersCache := make(map[string]string)

	for i, file := range files {
		// Check for cancellation
		select {
		case <-ctx.Done():
			log.Printf("🛑 파일 이동 취소됨 (%d/%d 완료)", i, len(files))
			response.Errors = append(response.Errors, "사용자에 의해 취소됨")
			// Remove from running operations
			uc.runningOpsMu.Lock()
			delete(uc.runningOps, progress.ID)
			uc.runningOpsMu.Unlock()
			return
		default:
		}

		// Update progress
		uc.progressService.UpdateOperation(ctx, progress.ID, i, fmt.Sprintf("파일 이동 중... (%d/%d) - %s", i+1, len(files), file.Name))

		// Determine target folder
		var targetFolderID string
		var err error

		if req.PreservePath && file.Path != "" {
			// Preserve path structure: create folder hierarchy in source folder
			targetFolderID, err = uc.getOrCreateTargetFolder(ctx, comparison.SourceFolderID, comparison.TargetFolderID, file, createdFoldersCache)
			if err != nil {
				log.Printf("❌ 대상 폴더 생성 실패 [%s]: %v", file.Path, err)
				response.FailedFiles = append(response.FailedFiles, file.ID)
				response.FailedCount++
				response.Errors = append(response.Errors, fmt.Sprintf("폴더 생성 실패 [%s]: %v", file.Path, err))
				continue
			}
		} else {
			// Move directly to source folder root
			targetFolderID = comparison.SourceFolderID
		}

		// Check for name conflict and resolve
		finalFileName := file.Name
		if req.OnConflict == "rename" {
			finalFileName, err = uc.resolveFileNameConflict(ctx, targetFolderID, file.Name)
			if err != nil {
				log.Printf("⚠️ 파일명 충돌 해결 실패 [%s]: %v", file.Name, err)
			}
		} else if req.OnConflict == "skip" {
			exists, _ := uc.checkFileExists(ctx, targetFolderID, file.Name)
			if exists {
				log.Printf("⏭️ 파일 건너뛰기 (이미 존재): %s", file.Name)
				response.SkippedCount++
				continue
			}
		}

		// Move the file
		err = uc.moveFileToFolder(ctx, file.ID, targetFolderID, finalFileName)
		if err != nil {
			log.Printf("❌ 파일 이동 실패 [%s]: %v", file.Name, err)
			response.FailedFiles = append(response.FailedFiles, file.ID)
			response.FailedCount++
			response.Errors = append(response.Errors, fmt.Sprintf("파일 이동 실패 [%s]: %v", file.Name, err))
			continue
		}

		log.Printf("✅ 파일 이동 완료: %s -> %s", file.Name, targetFolderID)
		response.MovedFiles = append(response.MovedFiles, file.ID)
		response.MovedCount++

		// Mark file as moved in database
		if repo, ok := uc.comparisonRepo.(interface {
			MarkFileAsMoved(ctx context.Context, comparisonID int, fileID string, newFileID string) error
		}); ok {
			repo.MarkFileAsMoved(ctx, req.ComparisonID, file.ID, file.ID)
		}

		// Call progress callback
		if req.ProgressCallback != nil {
			req.ProgressCallback(progress)
		}
	}

	// Add created folders to response
	for path := range createdFoldersCache {
		response.CreatedFolders = append(response.CreatedFolders, path)
	}

	// Remove from running operations
	uc.runningOpsMu.Lock()
	delete(uc.runningOps, progress.ID)
	uc.runningOpsMu.Unlock()

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	log.Printf("🎉 파일 이동 완료: %d개 성공, %d개 실패, %d개 건너뜀",
		response.MovedCount, response.FailedCount, response.SkippedCount)
}

// getOrCreateTargetFolder creates the necessary folder structure and returns the target folder ID
func (uc *FolderComparisonUseCase) getOrCreateTargetFolder(ctx context.Context, sourceFolderID, targetFolderID string, file *entities.File, cache map[string]string) (string, error) {
	// Get the relative path from target folder
	relativePath := uc.getRelativePath(ctx, targetFolderID, file)
	if relativePath == "" {
		return sourceFolderID, nil
	}

	// Check cache first
	cacheKey := sourceFolderID + ":" + relativePath
	if cachedID, ok := cache[cacheKey]; ok {
		return cachedID, nil
	}

	// Create folder hierarchy
	folderID, err := uc.createFolderHierarchy(ctx, sourceFolderID, relativePath)
	if err != nil {
		return "", err
	}

	// Cache the result
	cache[cacheKey] = folderID
	return folderID, nil
}

// getRelativePath extracts the relative path of a file from the target folder
// Uses file.Path field which is stored in the database
func (uc *FolderComparisonUseCase) getRelativePath(ctx context.Context, targetFolderID string, file *entities.File) string {
	// Use file.Path to extract relative path (file.Parents may be empty when loaded from DB)
	if file.Path == "" {
		log.Printf("⚠️ 파일 경로 없음: %s (ID: %s)", file.Name, file.ID)
		return ""
	}

	// file.Path format: "target_folder_name/subfolder1/subfolder2/filename.txt"
	// We need to extract: "subfolder1/subfolder2" (without target folder and filename)

	// Get directory portion (remove filename)
	dir := filepath.Dir(file.Path)
	if dir == "." || dir == "" {
		// File is in root of target folder
		return ""
	}

	// Split path into parts
	parts := strings.Split(dir, string(filepath.Separator))
	// Handle Unix-style paths as well
	if len(parts) == 1 && strings.Contains(dir, "/") {
		parts = strings.Split(dir, "/")
	}

	if len(parts) <= 1 {
		// Only target folder name, file is at root level
		return ""
	}

	// Skip first element (target folder name) and join the rest
	relativeParts := parts[1:]
	result := strings.Join(relativeParts, "/")

	log.Printf("📂 경로 추출: %s -> %s (대상폴더: %s)", file.Path, result, parts[0])

	return result
}

// createFolderHierarchy creates nested folders and returns the leaf folder ID
func (uc *FolderComparisonUseCase) createFolderHierarchy(ctx context.Context, parentFolderID, path string) (string, error) {
	if path == "" {
		return parentFolderID, nil
	}

	// Split path into parts
	parts := splitPath(path)
	currentParentID := parentFolderID

	for _, folderName := range parts {
		// Check if folder already exists
		existingFolderID, err := uc.findFolderByName(ctx, currentParentID, folderName)
		if err == nil && existingFolderID != "" {
			currentParentID = existingFolderID
			continue
		}

		// Create new folder
		newFolderID, err := uc.storageProvider.CreateFolder(ctx, currentParentID, folderName)
		if err != nil {
			return "", fmt.Errorf("폴더 생성 실패 [%s]: %w", folderName, err)
		}
		log.Printf("📁 새 폴더 생성: %s (ID: %s)", folderName, newFolderID)
		currentParentID = newFolderID
	}

	return currentParentID, nil
}

// splitPath splits a path string into folder names
func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// findFolderByName finds a folder by name within a parent folder
func (uc *FolderComparisonUseCase) findFolderByName(ctx context.Context, parentFolderID, folderName string) (string, error) {
	items, err := uc.storageProvider.ListFiles(ctx, parentFolderID)
	if err != nil {
		return "", err
	}

	for _, item := range items {
		if item.MimeType == "application/vnd.google-apps.folder" && item.Name == folderName {
			return item.ID, nil
		}
	}

	return "", fmt.Errorf("폴더를 찾을 수 없습니다: %s", folderName)
}

// resolveFileNameConflict generates a unique filename to avoid conflict
func (uc *FolderComparisonUseCase) resolveFileNameConflict(ctx context.Context, folderID, fileName string) (string, error) {
	// Check if file with same name exists
	exists, err := uc.checkFileExists(ctx, folderID, fileName)
	if err != nil {
		return fileName, err
	}

	if !exists {
		return fileName, nil
	}

	// Generate new name with suffix
	baseName, ext := splitFileName(fileName)
	for i := 1; i <= 100; i++ {
		newName := fmt.Sprintf("%s_%d%s", baseName, i, ext)
		exists, _ = uc.checkFileExists(ctx, folderID, newName)
		if !exists {
			log.Printf("📝 파일명 변경: %s -> %s", fileName, newName)
			return newName, nil
		}
	}

	return fileName, fmt.Errorf("고유한 파일명을 생성할 수 없습니다: %s", fileName)
}

// splitFileName splits filename into base name and extension
func splitFileName(fileName string) (baseName, ext string) {
	for i := len(fileName) - 1; i >= 0; i-- {
		if fileName[i] == '.' {
			return fileName[:i], fileName[i:]
		}
	}
	return fileName, ""
}

// checkFileExists checks if a file with the given name exists in the folder
func (uc *FolderComparisonUseCase) checkFileExists(ctx context.Context, folderID, fileName string) (bool, error) {
	items, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return false, err
	}

	for _, item := range items {
		if item.Name == fileName {
			return true, nil
		}
	}

	return false, nil
}

// moveFileToFolder moves a file to a new parent folder
func (uc *FolderComparisonUseCase) moveFileToFolder(ctx context.Context, fileID, newParentID, newName string) error {
	return uc.storageProvider.MoveFile(ctx, fileID, newParentID)
}

// GetUniqueFilesForComparison returns unique files that can be moved for a comparison
func (uc *FolderComparisonUseCase) GetUniqueFilesForComparison(ctx context.Context, comparisonID int) ([]*entities.File, error) {
	// Try to load from database first
	if repo, ok := uc.comparisonRepo.(interface {
		GetUnmovedUniqueFiles(ctx context.Context, comparisonID int) ([]*entities.File, error)
	}); ok {
		return repo.GetUnmovedUniqueFiles(ctx, comparisonID)
	}

	// Fallback: load from comparison result
	comparison, err := uc.comparisonRepo.GetByID(ctx, comparisonID)
	if err != nil {
		return nil, err
	}
	if comparison == nil {
		return nil, fmt.Errorf("비교 결과를 찾을 수 없습니다: %d", comparisonID)
	}

	return comparison.UniqueInTarget, nil
}
