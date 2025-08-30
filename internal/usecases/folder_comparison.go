package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"regexp"
	"sync"
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
	}
}

// CompareFoldersRequest represents the request for comparing folders
type CompareFoldersRequest struct {
	SourceFolderID      string                   `json:"sourceFolderId"`
	TargetFolderID      string                   `json:"targetFolderId"`
	IncludeSubfolders   bool                     `json:"includeSubfolders"`
	DeepComparison      bool                     `json:"deepComparison"`
	ForceNewComparison  bool                     `json:"forceNewComparison"`  // 기존 결과 무시하고 새로 비교
	MinFileSize         int64                    `json:"minFileSize,omitempty"`
	WorkerCount         int                      `json:"workerCount,omitempty"`
	ResumeProgressID    int                      `json:"resumeProgressId,omitempty"` // 재개할 진행 상황 ID
	ProgressCallback    func(*entities.Progress) `json:"-"`
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
	progress.SetMetadata("currentPhase", "initialized")

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

	// Start comparison in background with a new context (not tied to HTTP request)
	go uc.performFolderComparison(context.Background(), req, progress, comparisonResult, response)

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

	// Reconstruct request
	resumeReq := &CompareFoldersRequest{
		SourceFolderID:    sourceFolderID.(string),
		TargetFolderID:    targetFolderID.(string),
		IncludeSubfolders: includeSubfolders.(bool),
		DeepComparison:    deepComparison.(bool),
		MinFileSize:       int64(minFileSize.(float64)),
		WorkerCount:       int(workerCount.(float64)),
		ResumeProgressID:  req.ProgressID,
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

	// Continue comparison from checkpoint
	go uc.performFolderComparison(context.Background(), req, progress, comparisonResult, response)

	return response, nil
}

// GetPendingComparisons returns all pending/failed comparison operations
func (uc *FolderComparisonUseCase) GetPendingComparisons(ctx context.Context) ([]*entities.Progress, error) {
	// For now, return empty list - this can be implemented later when needed
	log.Printf("📋 중단된 작업 조회 요청 (현재 미구현)")
	return []*entities.Progress{}, nil
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
		if r := recover(); r != nil {
			log.Printf("💥 폴더 비교 중 패닉 발생: %v", r)
			log.Printf("📍 패닉 위치 - Progress ID: %d, 현재 단계: %v", progress.ID, progress.CurrentStep)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
		log.Printf("🔚 백그라운드 폴더 비교 작업 종료 - Progress ID: %d", progress.ID)
	}()

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
		
		log.Printf("📂 기준 폴더 스캔 시작 - Folder ID: %s, 하위폴더 포함: %v", req.SourceFolderID, req.IncludeSubfolders)
		sourceFiles, err = uc.getFilesFromFolder(ctx, req.SourceFolderID, req.IncludeSubfolders)
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
		sourceFiles, err = uc.getFilesFromFolder(ctx, req.SourceFolderID, req.IncludeSubfolders)
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
		targetFiles, err = uc.getFilesFromFolder(ctx, req.TargetFolderID, req.IncludeSubfolders)
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
		sourceFiles, err = uc.getFilesFromFolder(ctx, req.SourceFolderID, req.IncludeSubfolders)
		if err != nil {
			log.Printf("❌ 기준 폴더 파일 재조회 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("기준 폴더 파일 재조회 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("기준 폴더 파일 재조회 실패: %v", err))
			return
		}
		
		targetFiles, err = uc.getFilesFromFolder(ctx, req.TargetFolderID, req.IncludeSubfolders)
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
		
		duplicateFiles := uc.findDuplicatesBetweenFolders(sourceFiles, targetFiles, req.DeepComparison)

		// Add duplicate files to result
		for _, file := range duplicateFiles {
			result.AddDuplicateFile(file)
		}

		log.Printf("📊 중복 파일 %d개 발견 (%.1f%% 중복)",
			len(duplicateFiles), result.DuplicationPercentage)

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
		
		log.Printf("📊 저장할 데이터: 중복 파일 %d개, 기준 폴더 %d개 파일, 대상 폴더 %d개 파일", 
			len(duplicateFiles), len(sourceFiles), len(targetFiles))
		
		err = uc.comparisonRepo.Save(ctx, result)
		if err != nil {
			log.Printf("❌ 비교 결과 저장 실패: %v", err)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("비교 결과 저장 실패: %v", err))
			response.Errors = append(response.Errors, fmt.Sprintf("비교 결과 저장 실패: %v", err))
			return
		} else {
			log.Printf("✅ 비교 결과 저장 완료")
			// Final checkpoint - comparison completed
			progress.SetMetadata("currentPhase", "completed")
			log.Printf("✅ 체크포인트 저장: 폴더 비교 완료")
		}
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
	if includeSubfolders {
		return uc.getFilesRecursive(ctx, folderID)
	}
	return uc.storageProvider.ListFiles(ctx, folderID)
}

// getFilesRecursive recursively gets files from folder and subfolders
func (uc *FolderComparisonUseCase) getFilesRecursive(ctx context.Context, folderID string) ([]*entities.File, error) {
	var allFiles []*entities.File

	// Get files in current folder
	log.Printf("📂 폴더 스캔 시작: %s", folderID)
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
			log.Printf("📁 하위 폴더 발견: %s (%s)", file.Name, file.ID)
			subfolders = append(subfolders, file)
		} else if file.Size >= uc.minFileSize {
			log.Printf("📄 파일 발견: %s (크기: %d bytes)", file.Name, file.Size)
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
		log.Printf("🔍 하위 폴더 재귀 스캔 시작: %s (%s)", subfolder.Name, subfolder.ID)
		subFiles, err := uc.getFilesRecursive(ctx, subfolder.ID)
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

// findDuplicatesBetweenFolders finds duplicate files between source and target folders
func (uc *FolderComparisonUseCase) findDuplicatesBetweenFolders(sourceFiles, targetFiles []*entities.File, deepComparison bool) []*entities.File {
	// Create hash map of source files
	sourceHashes := make(map[string]*entities.File)
	sourceNameSizes := make(map[string]*entities.File) // fallback for non-hashed files

	for _, file := range sourceFiles {
		if deepComparison && file.IsHashCalculated() {
			sourceHashes[file.Hash] = file
		} else {
			// Use name + size as key for basic comparison
			key := fmt.Sprintf("%s_%d", file.Name, file.Size)
			sourceNameSizes[key] = file
		}
	}

	// Find duplicates in target files
	var duplicates []*entities.File

	for _, targetFile := range targetFiles {
		isDuplicate := false

		if deepComparison && targetFile.IsHashCalculated() {
			// Hash-based comparison (more accurate)
			if _, exists := sourceHashes[targetFile.Hash]; exists {
				isDuplicate = true
			}
		} else {
			// Name + size comparison (fallback)
			key := fmt.Sprintf("%s_%d", targetFile.Name, targetFile.Size)
			if _, exists := sourceNameSizes[key]; exists {
				isDuplicate = true
			}
		}

		if isDuplicate {
			duplicates = append(duplicates, targetFile)
		}
	}

	return duplicates
}

// DeleteTargetFolderRequest represents the request for deleting target folder
type DeleteTargetFolderRequest struct {
	ComparisonID         int                      `json:"comparisonId"`
	TargetFolderID       string                   `json:"targetFolderId"`
	DeleteEmptyFolders   bool                     `json:"deleteEmptyFolders"`
	ProgressCallback     func(*entities.Progress) `json:"-"`
	DeletionCallback     func(string, string)     `json:"-"` // fileId, status
}

// DeleteDuplicateFilesRequest represents the request for deleting duplicate files
type DeleteDuplicateFilesRequest struct {
	ComparisonID         int                      `json:"comparisonId"`
	FileIDs              []string                 `json:"fileIds"`
	DeleteEmptyFolders   bool                     `json:"deleteEmptyFolders"`
	ProgressCallback     func(*entities.Progress) `json:"-"`
	DeletionCallback     func(string, string)     `json:"-"` // fileId, status
}

// DeleteTargetFolderResponse represents the response for deleting target folder
type DeleteTargetFolderResponse struct {
	Progress         *entities.Progress `json:"progress"`
	DeletedFiles     []string           `json:"deletedFiles"`
	DeletedFolders   []string           `json:"deletedFolders"`
	FailedFiles      []string           `json:"failedFiles"`
	TotalDeleted     int                `json:"totalDeleted"`
	Errors           []string           `json:"errors,omitempty"`
}

// DeleteDuplicateFilesResponse represents the response for deleting duplicate files
type DeleteDuplicateFilesResponse struct {
	Progress         *entities.Progress `json:"progress"`
	DeletedFiles     []string           `json:"deletedFiles"`
	DeletedFolders   []string           `json:"deletedFolders"`
	FailedFiles      []string           `json:"failedFiles"`
	TotalDeleted     int                `json:"totalDeleted"`
	Errors           []string           `json:"errors,omitempty"`
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
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileCleanup, 0)
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
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "대상 폴더 삭제 시작...")

	// Delete the target folder directly
	err := uc.storageProvider.DeleteFolder(ctx, req.TargetFolderID)
	if err != nil {
		log.Printf("❌ 대상 폴더 삭제 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("대상 폴더 삭제 실패: %v", err))
		response.Errors = append(response.Errors, fmt.Sprintf("대상 폴더 삭제 실패: %v", err))
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

	// Pre-collect parent folders for empty folder cleanup
	if req.DeleteEmptyFolders {
		log.Printf("📁 빈 폴더 정리를 위한 부모 폴더 정보 수집 중...")
		uc.collectParentFoldersFromComparison(ctx, comparison, req.FileIDs, affectedFolders)
	}

	// Use batch deletion with parallel processing (configurable)
	batchSize := 10 // Default batch size
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

		deletedFolders := uc.cleanupEmptyFolders(ctx, affectedFolders)
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

// cleanupEmptyFolders removes empty folders
func (uc *FolderComparisonUseCase) cleanupEmptyFolders(ctx context.Context, folderIDs map[string]bool) []string {
	var deletedFolders []string

	for folderID := range folderIDs {
		// Check if folder is empty
		files, err := uc.storageProvider.ListFiles(ctx, folderID)
		if err != nil {
			log.Printf("⚠️ 폴더 내용 확인 실패 [%s]: %v", folderID, err)
			continue
		}

		if len(files) == 0 {
			// Folder is empty, delete it
			log.Printf("🗑️ 빈 폴더 삭제: %s", folderID)
			err := uc.storageProvider.DeleteFolder(ctx, folderID)
			if err != nil {
				log.Printf("❌ 빈 폴더 삭제 실패 [%s]: %v", folderID, err)
			} else {
				log.Printf("✅ 빈 폴더 삭제 완료: %s", folderID)
				deletedFolders = append(deletedFolders, folderID)
			}
		} else {
			log.Printf("📁 폴더가 비어있지 않음 [%s]: %d개 파일", folderID, len(files))
		}
	}

	return deletedFolders
}

// collectParentFoldersFromComparison collects parent folder information from comparison result
func (uc *FolderComparisonUseCase) collectParentFoldersFromComparison(ctx context.Context, comparison *entities.ComparisonResult, fileIDs []string, affectedFolders map[string]bool) {
	// Create a map of file IDs to delete for quick lookup
	deleteFileMap := make(map[string]bool)
	for _, fileID := range fileIDs {
		deleteFileMap[fileID] = true
	}
	
	// Extract parent folder information from duplicate files in comparison result
	for _, file := range comparison.DuplicateFiles {
		if deleteFileMap[file.ID] && len(file.Parents) > 0 {
			affectedFolders[file.Parents[0]] = true
		}
	}
	
	log.Printf("📁 수집된 부모 폴더 %d개 (파일 메타데이터에서)", len(affectedFolders))
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
				result := uc.deleteFileWithCallback(ctx, fileID, req.DeletionCallback)
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
func (uc *FolderComparisonUseCase) deleteFileWithCallback(ctx context.Context, fileID string, callback func(string, string)) deleteResult {
	// Call deletion callback - mark as deleting
	if callback != nil {
		callback(fileID, "deleting")
	}
	
	// Delete file
	err := uc.storageProvider.DeleteFile(ctx, fileID)
	
	// Call deletion callback with result
	if callback != nil {
		if err != nil {
			callback(fileID, "failed")
		} else {
			callback(fileID, "deleted")
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
		`/folders/([a-zA-Z0-9_-]+)(?:[/?#]|$)`,  // More precise pattern with end boundary
		`[?&]id=([a-zA-Z0-9_-]+)(?:[&]|$)`,      // More precise pattern for query parameter
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

	// Set metadata for checkpoint
	progress.SetMetadata("folderId", req.FolderID)
	progress.SetMetadata("includeSubfolders", req.IncludeSubfolders)
	progress.SetMetadata("minFileSize", req.MinFileSize)
	progress.SetMetadata("currentPhase", "initialized")

	// Initialize response
	response := &FindDuplicatesInSingleFolderResponse{
		Progress: progress,
		Errors:   make([]string, 0),
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

	files, err := uc.getFilesRecursive(ctx, req.FolderID, req.IncludeSubfolders)
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

	uc.progressService.CompleteOperation(ctx, progress.ID)
}

// findDuplicatesWithHashes finds duplicate files by calculating hashes
func (uc *FolderComparisonUseCase) findDuplicatesWithHashes(ctx context.Context, files []*entities.File, progress *entities.Progress) ([]*entities.DuplicateGroup, error) {
	hashToFiles := make(map[string][]*entities.File)
	
	for i, file := range files {
		// Calculate hash if not already calculated
		if file.Hash == "" {
			hash, err := uc.hashService.CalculateFileHash(ctx, file.ID)
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
