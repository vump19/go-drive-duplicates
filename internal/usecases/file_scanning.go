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
	storageProvider services.StorageProvider,
	fileService services.FileService,
	progressService services.ProgressService,
) *FileScanningUseCase {
	return &FileScanningUseCase{
		fileRepo:        fileRepo,
		progressRepo:    progressRepo,
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
	response := &ScanAllFilesResponse{
		Progress: progress,
		Errors:   make([]string, 0),
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

// performFullScan performs the actual full file scanning
func (uc *FileScanningUseCase) performFullScan(ctx context.Context, progress *entities.Progress, callback func(*entities.Progress), response *ScanAllFilesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 전체 스캔 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 목록 조회 중...")

	// Get all files from storage
	files, err := uc.storageProvider.ListAllFiles(ctx)
	if err != nil {
		log.Printf("❌ 파일 목록 조회 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("파일 목록 조회 실패: %v", err))
		return
	}

	// Update total count
	progress.SetTotal(len(files))
	response.TotalFiles = len(files)

	log.Printf("📊 총 %d개 파일 발견", len(files))

	// Process files in batches
	log.Printf("📦 배치 처리 시작: %d개 파일을 %d개씩 처리", len(files), uc.batchSize)
	uc.processBatches(ctx, files, progress, callback, response)

	// Complete the operation
	log.Printf("🏁 스캔 완료 처리 시작: 진행 상황 ID %d", progress.ID)
	
	// Update progress status to completed directly via repository
	progress.SetTotal(len(files))
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
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "폴더 파일 목록 조회 중...")

	// Get files from folder
	var files []*entities.File
	var err error

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
		response.ProcessedFiles += len(batch)
		response.NewFiles += newFiles
		response.UpdatedFiles += updatedFiles
		response.Errors = append(response.Errors, errors...)

		// Update progress
		progress.UpdateProgress(response.ProcessedFiles, fmt.Sprintf("배치 %d/%d 완료 (%d/%d 파일)", batchCount, (totalFiles+uc.batchSize-1)/uc.batchSize, response.ProcessedFiles, totalFiles))
		uc.progressService.UpdateOperation(ctx, progress.ID, response.ProcessedFiles, progress.CurrentStep)

		// Call progress callback
		if callback != nil {
			callback(progress)
		}

		// Log progress
		log.Printf("📈 진행 상황: %d/%d (%.1f%%) - 새 파일: %d, 업데이트: %d", 
			response.ProcessedFiles, totalFiles, float64(response.ProcessedFiles)/float64(totalFiles)*100,
			response.NewFiles, response.UpdatedFiles)
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

	return nil, fmt.Errorf("활성 파일 스캔 작업을 찾을 수 없습니다")
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
