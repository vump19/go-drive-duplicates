package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"sync"
	"time"
)

// FileScanningUseCase handles file scanning operations
type FileScanningUseCase struct {
	fileRepo        repositories.FileRepository
	progressRepo    repositories.ProgressRepository
	duplicateRepo   repositories.DuplicateRepository
	storageProvider services.StorageProvider
	fileService     services.FileService
	progressService services.ProgressService

	// Configuration
	batchSize    int
	workerCount  int
	saveInterval time.Duration
}

// NewFileScanningUseCase creates a new file scanning use case
func NewFileScanningUseCase(
	fileRepo repositories.FileRepository,
	progressRepo repositories.ProgressRepository,
	duplicateRepo repositories.DuplicateRepository,
	storageProvider services.StorageProvider,
	fileService services.FileService,
	progressService services.ProgressService,
) *FileScanningUseCase {
	return &FileScanningUseCase{
		fileRepo:        fileRepo,
		progressRepo:    progressRepo,
		duplicateRepo:   duplicateRepo,
		storageProvider: storageProvider,
		fileService:     fileService,
		progressService: progressService,
		batchSize:       100,
		workerCount:     3,
		saveInterval:    30 * time.Second,
	}
}

// ScanAllFilesRequest represents the request for scanning all files
type ScanAllFilesRequest struct {
	ResumeFromProgress bool                     `json:"resumeFromProgress"`
	BatchSize          int                      `json:"batchSize,omitempty"`
	WorkerCount        int                      `json:"workerCount,omitempty"`
	ProgressCallback   func(*entities.Progress) `json:"-"`
}

// ScanAllFilesResponse represents the response for scanning all files
type ScanAllFilesResponse struct {
	Progress       *entities.Progress `json:"progress"`
	TotalFiles     int                `json:"totalFiles"`
	ProcessedFiles int                `json:"processedFiles"`
	NewFiles       int                `json:"newFiles"`
	UpdatedFiles   int                `json:"updatedFiles"`
	Errors         []string           `json:"errors,omitempty"`
}

// ScanFolderRequest represents the request for scanning a specific folder
type ScanFolderRequest struct {
	FolderID         string                   `json:"folderId"`
	Recursive        bool                     `json:"recursive"`
	UpdatePaths      bool                     `json:"updatePaths"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// ScanFolderResponse represents the response for scanning a folder
type ScanFolderResponse struct {
	Progress       *entities.Progress `json:"progress"`
	TotalFiles     int                `json:"totalFiles"`
	ProcessedFiles int                `json:"processedFiles"`
	NewFiles       int                `json:"newFiles"`
	UpdatedFiles   int                `json:"updatedFiles"`
	FolderPath     string             `json:"folderPath"`
	Errors         []string           `json:"errors,omitempty"`
}

// ScanAllFiles scans all files from the storage provider
func (uc *FileScanningUseCase) ScanAllFiles(ctx context.Context, req *ScanAllFilesRequest) (*ScanAllFilesResponse, error) {
	log.Println("🔍 전체 파일 스캔 시작")

	// Apply configuration
	if req.BatchSize > 0 {
		uc.batchSize = req.BatchSize
	}
	if req.WorkerCount > 0 {
		uc.workerCount = req.WorkerCount
	}

	// Check for existing progress
	var progress *entities.Progress
	var err error

	if req.ResumeFromProgress {
		activeProgress, err := uc.progressService.GetActiveOperations(ctx)
		if err == nil && len(activeProgress) > 0 {
			for _, p := range activeProgress {
				if p.OperationType == entities.OperationFileScan {
					progress = p
					log.Printf("🔄 기존 진행 상황에서 재개: %d/%d", p.ProcessedItems, p.TotalItems)
					break
				}
			}
		}
	}

	// Create new progress if not resuming
	if progress == nil {
		progress, err = uc.progressService.StartOperation(ctx, entities.OperationFileScan, 0)
		if err != nil {
			return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
		}
	}

	// Initialize response
	// DB에서 현재 저장된 파일 수 조회 (재개 시 정확한 표시용)
	var initialProcessedFiles int
	stats, err := uc.fileRepo.GetStatistics(ctx)
	if err == nil {
		initialProcessedFiles = int(stats.TotalFiles)
		log.Printf("📊 재개 시작: DB에 이미 저장된 파일 %d개", initialProcessedFiles)
	} else {
		initialProcessedFiles = progress.ProcessedItems
		log.Printf("⚠️ 파일 통계 조회 실패, progress 값 사용: %d", initialProcessedFiles)
	}

	response := &ScanAllFilesResponse{
		Progress:       progress,
		ProcessedFiles: initialProcessedFiles, // DB에 저장된 실제 파일 수
		Errors:         make([]string, 0),
	}

	// Start scanning in background with a new context (not tied to HTTP request)
	go uc.performFullScan(context.Background(), progress, req.ProgressCallback, response)

	return response, nil
}

// ScanFolder scans files in a specific folder
func (uc *FileScanningUseCase) ScanFolder(ctx context.Context, req *ScanFolderRequest) (*ScanFolderResponse, error) {
	log.Printf("📁 폴더 스캔 시작: %s", req.FolderID)

	// Validate folder access
	if err := uc.fileService.ValidateFileAccess(ctx, req.FolderID); err != nil {
		return nil, fmt.Errorf("폴더 접근 권한 확인 실패: %w", err)
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileScan, 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Get folder path
	folderPath, err := uc.storageProvider.GetFolderPath(ctx, req.FolderID)
	if err != nil {
		log.Printf("⚠️ 폴더 경로 조회 실패: %v", err)
		folderPath = "알 수 없는 경로"
	}

	// Initialize response
	response := &ScanFolderResponse{
		Progress:   progress,
		FolderPath: folderPath,
		Errors:     make([]string, 0),
	}

	// Start scanning in background with a new context (not tied to HTTP request)
	go uc.performFolderScan(context.Background(), req.FolderID, req.Recursive, req.UpdatePaths, progress, req.ProgressCallback, response)

	return response, nil
}

// performFullScan performs the actual full file scanning with checkpoint support
func (uc *FileScanningUseCase) performFullScan(ctx context.Context, progress *entities.Progress, callback func(*entities.Progress), response *ScanAllFilesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 전체 스캔 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()

	// DB에서 현재 전체 파일 수 조회 (재개 시 정확한 진행 상황 표시용)
	stats, err := uc.fileRepo.GetStatistics(ctx)
	if err == nil {
		totalFiles := stats.TotalFiles
		progress.SetTotal(int(totalFiles))
		response.TotalFiles = int(totalFiles)
		log.Printf("📊 DB에 저장된 전체 파일: %d개", totalFiles)
	} else {
		log.Printf("⚠️ 파일 통계 조회 실패: %v", err)
	}

	// ⚠️ 중요: status를 "running"으로 먼저 DB에 저장해야 함!
	// UpdateOperation은 DB에서 progress를 다시 읽어오므로, 먼저 저장하지 않으면 status가 "pending"으로 남음
	err = uc.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("⚠️ 진행 상황 시작 상태 저장 실패: %v", err)
	}
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 목록 조회 중...")

	// 체크포인트 기반 재귀 스캔 수행
	err = uc.scanFolderRecursiveWithCheckpoint(ctx, "root", "", progress, callback, response)
	if err != nil {
		log.Printf("❌ 파일 스캔 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("파일 스캔 실패: %v", err))
		return
	}

	// Complete the operation
	log.Printf("🏁 스캔 완료 처리 시작: 진행 상황 ID %d", progress.ID)

	// Update progress status to completed directly via repository
	progress.Complete()
	err = uc.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("⚠️ 진행 상황 완료 처리 실패: %v", err)
	} else {
		log.Printf("✅ 진행 상황 완료 처리 성공 - DB 직접 업데이트")
	}
	log.Printf("📊 최종 상태: 처리된 파일=%d, 새 파일=%d, 업데이트된 파일=%d",
		response.ProcessedFiles, response.NewFiles, response.UpdatedFiles)

	if callback != nil {
		callback(progress)
	}

	log.Printf("✅ 전체 파일 스캔 완료: %d개 파일 처리", response.ProcessedFiles)
}

// performFolderScan performs the actual folder scanning
func (uc *FileScanningUseCase) performFolderScan(ctx context.Context, folderID string, recursive, updatePaths bool, progress *entities.Progress, callback func(*entities.Progress), response *ScanFolderResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 폴더 스캔 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	// ⚠️ 중요: status를 "running"으로 먼저 DB에 저장해야 함!
	err := uc.progressRepo.Update(ctx, progress)
	if err != nil {
		log.Printf("⚠️ 진행 상황 시작 상태 저장 실패: %v", err)
	}
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "폴더 파일 목록 조회 중...")

	// Get files from folder
	var files []*entities.File

	if recursive {
		files, err = uc.scanFolderRecursive(ctx, folderID)
	} else {
		files, err = uc.storageProvider.ListFiles(ctx, folderID)
	}

	if err != nil {
		log.Printf("❌ 폴더 파일 목록 조회 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("폴더 파일 목록 조회 실패: %v", err))
		return
	}

	// Update total count
	progress.SetTotal(len(files))
	response.TotalFiles = len(files)

	log.Printf("📊 폴더 내 %d개 파일 발견", len(files))

	// Update paths if requested
	if updatePaths {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 경로 업데이트 중...")
		if err := uc.fileService.UpdateFilePaths(ctx, files); err != nil {
			log.Printf("⚠️ 파일 경로 업데이트 실패: %v", err)
			response.Errors = append(response.Errors, fmt.Sprintf("파일 경로 업데이트 실패: %v", err))
		}
	}

	// Process files in batches
	uc.processBatches(ctx, files, progress, callback, &ScanAllFilesResponse{
		Progress:       response.Progress,
		TotalFiles:     response.TotalFiles,
		ProcessedFiles: response.ProcessedFiles,
		NewFiles:       response.NewFiles,
		UpdatedFiles:   response.UpdatedFiles,
		Errors:         response.Errors,
	})

	// Response is already updated through tempResponse

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	if callback != nil {
		callback(progress)
	}

	log.Printf("✅ 폴더 스캔 완료: %d개 파일 처리", response.ProcessedFiles)
}

// processBatches processes files in batches
func (uc *FileScanningUseCase) processBatches(ctx context.Context, files []*entities.File, progress *entities.Progress, callback func(*entities.Progress), response *ScanAllFilesResponse) {
	totalFiles := len(files)
	log.Printf("📦 배치 처리 세부사항: 총 %d개 파일, 배치 크기 %d, 워커 %d개", totalFiles, uc.batchSize, uc.workerCount)

	// 배치 처리 시작 시 한 번만 DB에서 전체 파일 수 조회 (재개 시 정확한 표시)
	var baseFileCount int
	stats, err := uc.fileRepo.GetStatistics(ctx)
	if err == nil {
		baseFileCount = int(stats.TotalFiles)
		log.Printf("📊 배치 처리 시작 - DB에 저장된 파일: %d개", baseFileCount)
	} else {
		// GetStatistics 실패 시 DB에서 직접 파일 수 조회
		count, countErr := uc.fileRepo.Count(ctx)
		if countErr == nil {
			baseFileCount = int(count)
			log.Printf("📊 배치 처리 시작 - DB 직접 조회: %d개 파일 (GetStatistics 실패: %v)", baseFileCount, err)
		} else {
			// DB 조회도 실패하면 response.ProcessedFiles 사용
			baseFileCount = response.ProcessedFiles
			log.Printf("⚠️ DB 조회 실패, response 값 사용: %d (에러: GetStatistics=%v, Count=%v)", baseFileCount, err, countErr)
		}
	}

	localProcessedFiles := 0 // 이번 processBatches 호출에서 처리한 파일 수
	batchCount := 0
	for i := 0; i < totalFiles; i += uc.batchSize {
		end := i + uc.batchSize
		if end > totalFiles {
			end = totalFiles
		}

		batch := files[i:end]
		batchCount++

		log.Printf("📦 배치 %d 처리 시작: %d-%d번째 파일 (%d개)", batchCount, i+1, end, len(batch))

		// Process batch
		newFiles, updatedFiles, errors := uc.processBatch(ctx, batch)

		log.Printf("📊 배치 %d 결과: 새 파일 %d개, 업데이트 %d개, 에러 %d개", batchCount, newFiles, updatedFiles, len(errors))

		// Update response
		localProcessedFiles += len(batch)
		response.ProcessedFiles = baseFileCount + localProcessedFiles
		response.NewFiles += newFiles
		response.UpdatedFiles += updatedFiles
		response.Errors = append(response.Errors, errors...)

		// Update progress
		progress.UpdateProgress(response.ProcessedFiles, fmt.Sprintf("배치 %d/%d 완료 (현재 폴더: %d/%d, 전체: %d개)", batchCount, (totalFiles+uc.batchSize-1)/uc.batchSize, end, totalFiles, response.ProcessedFiles))
		uc.progressService.UpdateOperation(ctx, progress.ID, response.ProcessedFiles, progress.CurrentStep)

		// Call progress callback
		if callback != nil {
			callback(progress)
		}

		// Log progress
		log.Printf("📈 진행 상황: %d/%d (%.1f%%) - 새 파일: %d, 업데이트: %d",
			response.ProcessedFiles, totalFiles, float64(response.ProcessedFiles)/float64(totalFiles)*100,
			response.NewFiles, response.UpdatedFiles)

		// 🔄 실시간 중복 그룹 업데이트 (10배치마다 또는 마지막 배치)
		isLastBatch := (i + uc.batchSize) >= totalFiles
		if batchCount%10 == 0 || isLastBatch {
			go uc.updateDuplicateGroupsAsync(context.Background(), batch, progress.ID)
		}
	}

	log.Printf("🎯 모든 배치 처리 완료: %d개 배치 처리됨", batchCount)
}

// processBatch processes a batch of files
func (uc *FileScanningUseCase) processBatch(ctx context.Context, batch []*entities.File) (newFiles, updatedFiles int, errors []string) {
	errors = make([]string, 0)

	// Use worker pool for parallel processing
	jobs := make(chan *entities.File, len(batch))
	results := make(chan struct {
		isNew bool
		err   error
	}, len(batch))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < uc.workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				isNew, err := uc.processFile(ctx, file)
				results <- struct {
					isNew bool
					err   error
				}{isNew: isNew, err: err}
			}
		}()
	}

	// Send jobs
	for _, file := range batch {
		jobs <- file
	}
	close(jobs)

	// Wait for workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		if result.err != nil {
			errors = append(errors, result.err.Error())
		} else if result.isNew {
			newFiles++
		} else {
			updatedFiles++
		}
	}

	return newFiles, updatedFiles, errors
}

// processFile processes a single file
func (uc *FileScanningUseCase) processFile(ctx context.Context, file *entities.File) (isNew bool, err error) {
	// Check if file exists
	exists, err := uc.fileRepo.Exists(ctx, file.ID)
	if err != nil {
		return false, fmt.Errorf("파일 존재 확인 실패 [%s]: %w", file.ID, err)
	}

	if exists {
		// Update existing file
		err = uc.fileRepo.Update(ctx, file)
		if err != nil {
			return false, fmt.Errorf("파일 업데이트 실패 [%s]: %w", file.ID, err)
		}
		return false, nil
	} else {
		// Save new file
		err = uc.fileRepo.Save(ctx, file)
		if err != nil {
			return false, fmt.Errorf("파일 저장 실패 [%s]: %w", file.ID, err)
		}
		return true, nil
	}
}

// scanFolderRecursive recursively scans a folder and its subfolders
func (uc *FileScanningUseCase) scanFolderRecursive(ctx context.Context, folderID string) ([]*entities.File, error) {
	var allFiles []*entities.File

	// Get files in current folder
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		return nil, err
	}

	// Separate files and folders
	var actualFiles []*entities.File
	var subfolders []*entities.File

	for _, file := range files {
		if file.GetFileCategory() == "folder" {
			subfolders = append(subfolders, file)
		} else {
			actualFiles = append(actualFiles, file)
		}
	}

	// Add current folder files
	allFiles = append(allFiles, actualFiles...)

	// Recursively scan subfolders
	for _, subfolder := range subfolders {
		subFiles, err := uc.scanFolderRecursive(ctx, subfolder.ID)
		if err != nil {
			log.Printf("⚠️ 하위 폴더 스캔 실패 [%s]: %v", subfolder.ID, err)
			continue
		}
		allFiles = append(allFiles, subFiles...)
	}

	return allFiles, nil
}

// GetScanProgress returns the current scan progress
func (uc *FileScanningUseCase) GetScanProgress(ctx context.Context) (*entities.Progress, error) {
	activeProgress, err := uc.progressService.GetActiveOperations(ctx)
	if err != nil {
		return nil, err
	}

	for _, progress := range activeProgress {
		if progress.OperationType == entities.OperationFileScan {
			return progress, nil
		}
	}

	return nil, nil
}

// GetStatistics returns file statistics including hash calculation progress
func (uc *FileScanningUseCase) GetStatistics(ctx context.Context) (*entities.FileStatistics, error) {
	stats, err := uc.fileRepo.GetStatistics(ctx)
	if err != nil {
		return nil, fmt.Errorf("파일 통계 조회 실패: %w", err)
	}
	return stats, nil
}

// SetConfiguration sets the use case configuration
func (uc *FileScanningUseCase) SetConfiguration(batchSize, workerCount int, saveInterval time.Duration) {
	if batchSize > 0 {
		uc.batchSize = batchSize
	}
	if workerCount > 0 {
		uc.workerCount = workerCount
	}
	if saveInterval > 0 {
		uc.saveInterval = saveInterval
	}
}

// ClearFailedProgress clears all failed or pending progress records
func (uc *FileScanningUseCase) ClearFailedProgress(ctx context.Context) error {
	log.Println("🧹 실패한 진행 상황 정리 시작")

	// Get all progress records
	allProgress, err := uc.progressService.GetActiveOperations(ctx)
	if err != nil {
		return fmt.Errorf("진행 상황 조회 실패: %w", err)
	}

	clearedCount := 0
	for _, progress := range allProgress {
		// Clear records that are in failed, pending, or stuck states
		if progress.Status == "failed" || progress.Status == "pending" {
			// Clear all pending operations (수동 정리이므로 시간 제한 없음)
			// if progress.Status == "pending" && time.Since(progress.StartTime) < 10*time.Minute {
			//	continue // Skip recent pending operations
			// }

			err := uc.progressService.FailOperation(ctx, progress.ID, "수동으로 정리됨")
			if err != nil {
				log.Printf("⚠️ 진행 상황 정리 실패 [%d]: %v", progress.ID, err)
				continue
			}
			clearedCount++
			log.Printf("🗑️ 진행 상황 정리됨: ID=%d, Type=%s, Status=%s",
				progress.ID, progress.OperationType, progress.Status)
		}
	}

	log.Printf("✅ %d개 진행 상황 정리 완료", clearedCount)
	return nil
}

// scanFolderRecursiveWithCheckpoint recursively scans a folder with checkpoint support
func (uc *FileScanningUseCase) scanFolderRecursiveWithCheckpoint(
	ctx context.Context,
	folderID string,
	currentPath string,
	progress *entities.Progress,
	callback func(*entities.Progress),
	response *ScanAllFilesResponse,
) error {
	// DB에서 최신 progress 다시 읽어오기 (다른 재귀 호출의 체크포인트 반영)
	latestProgress, err := uc.progressRepo.GetByID(ctx, progress.ID)
	if err != nil {
		log.Printf("⚠️ 진행 상황 조회 실패, 메모리 상태 사용: %v", err)
		latestProgress = progress
	} else {
		// 최신 progress로 교체
		progress = latestProgress
	}

	// 메타데이터에서 스캔 완료된 폴더 목록 가져오기
	scannedFolders := make(map[string]bool)
	if scannedData, ok := progress.GetMetadata("scannedFolders"); ok {
		if scannedList, ok := scannedData.([]interface{}); ok {
			for _, folder := range scannedList {
				if folderIDStr, ok := folder.(string); ok {
					scannedFolders[folderIDStr] = true
				}
			}
		}
	}

	// 이미 스캔한 폴더인지 확인
	if scannedFolders[folderID] {
		log.Printf("⏭️ 폴더 건너뛰기 (이미 스캔됨): %s [%s]", currentPath, folderID)
		return nil
	}

	log.Printf("📁 폴더 스캔 시작: %s [%s] - 이미 스캔된 폴더: %d개", currentPath, folderID, len(scannedFolders))

	// 현재 폴더의 파일 및 하위 폴더 목록 조회
	files, err := uc.storageProvider.ListFiles(ctx, folderID)
	if err != nil {
		log.Printf("❌ 폴더 목록 조회 실패 [%s]: %v", folderID, err)
		return fmt.Errorf("폴더 목록 조회 실패 [%s]: %w", folderID, err)
	}

	// 파일과 하위 폴더 분리
	var actualFiles []*entities.File
	var subfolders []*entities.File

	for _, file := range files {
		// 경로 설정
		if currentPath != "" {
			file.Path = currentPath + "/" + file.Name
		} else {
			file.Path = file.Name
		}

		if file.GetFileCategory() == "folder" {
			subfolders = append(subfolders, file)
		} else {
			actualFiles = append(actualFiles, file)
		}
	}

	// 현재 폴더의 파일들을 배치로 처리
	if len(actualFiles) > 0 {
		log.Printf("📦 폴더 [%s] 내 파일 처리 시작: %d개", folderID, len(actualFiles))
		uc.processBatches(ctx, actualFiles, progress, callback, response)
	}

	// 하위 폴더 재귀 처리
	for _, subfolder := range subfolders {
		subPath := currentPath
		if subPath != "" {
			subPath = subPath + "/" + subfolder.Name
		} else {
			subPath = subfolder.Name
		}

		err := uc.scanFolderRecursiveWithCheckpoint(ctx, subfolder.ID, subPath, progress, callback, response)
		if err != nil {
			log.Printf("⚠️ 하위 폴더 스캔 실패 [%s]: %v", subfolder.Name, err)
			// 에러가 발생해도 계속 진행 (일부 폴더 스캔 실패 허용)
		}
	}

	// 폴더의 모든 항목 스캔 완료 -> 메타데이터에 저장
	log.Printf("✅ 폴더 스캔 완료: %s [%s] - %d개 파일, %d개 하위 폴더", currentPath, folderID, len(actualFiles), len(subfolders))

	// 체크포인트 저장 직전에 DB에서 최신 progress 다시 읽기 (하위 폴더들의 체크포인트 반영)
	latestProgressForSave, err := uc.progressRepo.GetByID(ctx, progress.ID)
	if err != nil {
		log.Printf("⚠️ 체크포인트 저장 전 진행 상황 조회 실패, 현재 상태로 저장: %v", err)
		latestProgressForSave = progress
	}

	// 최신 progress에서 스캔 완료된 폴더 목록 가져오기
	latestScannedFolders := make(map[string]bool)
	if scannedData, ok := latestProgressForSave.GetMetadata("scannedFolders"); ok {
		if scannedList, ok := scannedData.([]interface{}); ok {
			for _, folder := range scannedList {
				if folderIDStr, ok := folder.(string); ok {
					latestScannedFolders[folderIDStr] = true
				}
			}
		}
	}

	// 현재 폴더 추가
	latestScannedFolders[folderID] = true

	// Map을 List로 변환
	scannedFoldersList := make([]string, 0, len(latestScannedFolders))
	for fid := range latestScannedFolders {
		scannedFoldersList = append(scannedFoldersList, fid)
	}

	latestProgressForSave.SetMetadata("scannedFolders", scannedFoldersList)
	latestProgressForSave.SetMetadata("lastScannedFolder", folderID)
	latestProgressForSave.SetMetadata("lastScannedFolderPath", currentPath)

	// 진행 상황 DB 업데이트
	err = uc.progressRepo.Update(ctx, latestProgressForSave)
	if err != nil {
		log.Printf("⚠️ 체크포인트 저장 실패: %v", err)
	} else {
		log.Printf("💾 체크포인트 저장: %s [%s] - 총 %d개 폴더 스캔 완료", currentPath, folderID, len(scannedFoldersList))
	}

	// 메모리 progress도 업데이트 (다음 재귀 호출을 위해)
	progress = latestProgressForSave

	return nil
}

// updateDuplicateGroupsAsync updates duplicate groups asynchronously for the given batch of files
func (uc *FileScanningUseCase) updateDuplicateGroupsAsync(ctx context.Context, batch []*entities.File, progressID int) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️ 중복 그룹 업데이트 중 패닉 발생: %v", r)
		}
	}()

	// Collect unique hashes from batch
	hashSet := make(map[string]bool)
	for _, file := range batch {
		if file.Hash != "" {
			hashSet[file.Hash] = true
		}
	}

	if len(hashSet) == 0 {
		return // No hashes to process
	}

	log.Printf("🔄 중복 그룹 실시간 업데이트: %d개 해시 처리 중...", len(hashSet))

	groupsCreated := 0
	groupsUpdated := 0

	for hash := range hashSet {
		// Get all files with this hash from DB
		files, err := uc.fileRepo.GetByHash(ctx, hash)
		if err != nil {
			log.Printf("⚠️ 해시 조회 실패 [%s]: %v", hash[:8], err)
			continue
		}

		// Only create/update group if there are 2+ files with same hash
		if len(files) < 2 {
			continue
		}

		// Check if group already exists
		existingGroup, err := uc.duplicateRepo.GetByHash(ctx, hash)
		if err != nil {
			// Error occurred, but it might just be "not found"
			log.Printf("⚠️ 중복 그룹 조회 오류 [%s]: %v", hash[:8], err)
		}

		if existingGroup == nil {
			// Create new duplicate group
			group := entities.NewDuplicateGroup(hash)
			for _, file := range files {
				group.AddFile(file)
			}

			if err := uc.duplicateRepo.Save(ctx, group); err != nil {
				log.Printf("⚠️ 중복 그룹 생성 실패 [%s]: %v", hash[:8], err)
			} else {
				groupsCreated++
			}
		} else {
			// Update existing group if file count changed
			if existingGroup.Count != len(files) {
				existingGroup.Count = len(files)
				existingGroup.TotalSize = int64(len(files)) * files[0].Size
				existingGroup.Files = files

				if err := uc.duplicateRepo.Update(ctx, existingGroup); err != nil {
					log.Printf("⚠️ 중복 그룹 업데이트 실패 [%s]: %v", hash[:8], err)
				} else {
					groupsUpdated++
				}
			}
		}
	}

	if groupsCreated > 0 || groupsUpdated > 0 {
		log.Printf("✅ 중복 그룹 업데이트 완료: %d개 생성, %d개 업데이트", groupsCreated, groupsUpdated)

		// 📊 progress 메타데이터에 발견된 중복 그룹 수 업데이트
		totalGroups, err := uc.duplicateRepo.Count(ctx)
		if err != nil {
			log.Printf("⚠️ 중복 그룹 수 조회 실패: %v", err)
			return
		}

		// progress 객체 조회 및 메타데이터 업데이트
		progress, err := uc.progressRepo.GetByID(ctx, progressID)
		if err != nil {
			log.Printf("⚠️ progress 조회 실패 [ID=%d]: %v", progressID, err)
			return
		}

		progress.SetMetadata("foundDuplicateGroups", totalGroups)
		if err := uc.progressRepo.Update(ctx, progress); err != nil {
			log.Printf("⚠️ progress 메타데이터 업데이트 실패: %v", err)
		} else {
			log.Printf("📊 발견된 중복 그룹: %d개 (progress 메타데이터 업데이트됨)", totalGroups)
		}
	}
}
