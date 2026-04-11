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

// FileCleanupUseCase handles file cleanup and deletion operations
type FileCleanupUseCase struct {
	fileRepo        repositories.FileRepository
	duplicateRepo   repositories.DuplicateRepository
	comparisonRepo  repositories.ComparisonRepository
	progressRepo    repositories.ProgressRepository
	storageProvider services.StorageProvider
	cleanupService  services.CleanupService
	progressService services.ProgressService

	// Configuration
	batchSize      int
	workerCount    int
	safetyChecks   bool
	cleanupFolders bool
}

// NewFileCleanupUseCase creates a new file cleanup use case
func NewFileCleanupUseCase(
	fileRepo repositories.FileRepository,
	duplicateRepo repositories.DuplicateRepository,
	comparisonRepo repositories.ComparisonRepository,
	progressRepo repositories.ProgressRepository,
	storageProvider services.StorageProvider,
	cleanupService services.CleanupService,
	progressService services.ProgressService,
) *FileCleanupUseCase {
	return &FileCleanupUseCase{
		fileRepo:        fileRepo,
		duplicateRepo:   duplicateRepo,
		comparisonRepo:  comparisonRepo,
		progressRepo:    progressRepo,
		storageProvider: storageProvider,
		cleanupService:  cleanupService,
		progressService: progressService,
		batchSize:       50,
		workerCount:     3,
		safetyChecks:    true,
		cleanupFolders:  true,
	}
}

// DeleteFilesRequest represents the request for deleting files
type DeleteFilesRequest struct {
	FileIDs          []string                 `json:"fileIds"`
	CleanupFolders   bool                     `json:"cleanupFolders"`
	SafetyChecks     bool                     `json:"safetyChecks"`
	BatchSize        int                      `json:"batchSize,omitempty"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// DeleteFilesResponse represents the response for deleting files
type DeleteFilesResponse struct {
	Progress       *entities.Progress `json:"progress"`
	TotalFiles     int                `json:"totalFiles"`
	DeletedFiles   int                `json:"deletedFiles"`
	FailedFiles    int                `json:"failedFiles"`
	DeletedFolders int                `json:"deletedFolders,omitempty"`
	SpaceSaved     int64              `json:"spaceSaved"`
	Errors         []string           `json:"errors,omitempty"`
}

// DeleteDuplicatesRequest represents the request for deleting duplicates from a group
type DeleteDuplicatesRequest struct {
	GroupID          int                      `json:"groupId"`
	KeepFileID       string                   `json:"keepFileId"`
	CleanupFolders   bool                     `json:"cleanupFolders"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// BulkDeleteByPatternRequest represents the request for bulk deletion by pattern
type BulkDeleteByPatternRequest struct {
	FolderID         string                   `json:"folderId"`
	Pattern          string                   `json:"pattern"`
	Recursive        bool                     `json:"recursive"`
	DryRun           bool                     `json:"dryRun"`
	CleanupFolders   bool                     `json:"cleanupFolders"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// CleanupEmptyFoldersRequest represents the request for cleaning up empty folders
type CleanupEmptyFoldersRequest struct {
	RootFolderID     string                   `json:"rootFolderId,omitempty"`
	Recursive        bool                     `json:"recursive"`
	ProgressCallback func(*entities.Progress) `json:"-"`
}

// CleanupEmptyFoldersResponse represents the response for cleaning up empty folders
type CleanupEmptyFoldersResponse struct {
	Progress       *entities.Progress `json:"progress"`
	DeletedFolders int                `json:"deletedFolders"`
	Errors         []string           `json:"errors,omitempty"`
}

// DeleteFiles deletes specified files
func (uc *FileCleanupUseCase) DeleteFiles(ctx context.Context, req *DeleteFilesRequest) (*DeleteFilesResponse, error) {
	log.Printf("🗑️ 파일 휴지통 이동 시작: %d개 파일", len(req.FileIDs))

	// Apply configuration
	if req.BatchSize > 0 {
		uc.batchSize = req.BatchSize
	}
	uc.safetyChecks = req.SafetyChecks
	uc.cleanupFolders = req.CleanupFolders

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileCleanup, len(req.FileIDs))
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &DeleteFilesResponse{
		Progress:   progress,
		TotalFiles: len(req.FileIDs),
		Errors:     make([]string, 0),
	}

	// Start deletion in background (HTTP 컨텍스트 대신 독립 컨텍스트 사용)
	go uc.performFileDeletion(context.Background(), req, progress, response)

	return response, nil
}

// DeleteDuplicatesFromGroup deletes duplicate files from a group, keeping one file
func (uc *FileCleanupUseCase) DeleteDuplicatesFromGroup(ctx context.Context, req *DeleteDuplicatesRequest) (*DeleteFilesResponse, error) {
	log.Printf("🗑️ 중복 그룹에서 파일 삭제 시작: 그룹 %d", req.GroupID)

	// Get duplicate group
	group, err := uc.duplicateRepo.GetByID(ctx, req.GroupID)
	if err != nil {
		return nil, fmt.Errorf("중복 그룹 조회 실패: %w", err)
	}

	if group == nil {
		return nil, fmt.Errorf("중복 그룹을 찾을 수 없습니다: %d", req.GroupID)
	}

	// Get files to delete (all except the one to keep)
	filesToDelete := group.GetFilesExcept(req.KeepFileID)
	if len(filesToDelete) == 0 {
		return nil, fmt.Errorf("삭제할 파일이 없습니다")
	}

	// Convert to file IDs
	fileIDs := make([]string, len(filesToDelete))
	for i, file := range filesToDelete {
		fileIDs[i] = file.ID
	}

	// Create file deletion request
	deleteReq := &DeleteFilesRequest{
		FileIDs:          fileIDs,
		CleanupFolders:   req.CleanupFolders,
		SafetyChecks:     true,
		ProgressCallback: req.ProgressCallback,
	}

	return uc.DeleteFiles(ctx, deleteReq)
}

// BulkDeleteByPattern deletes files matching a pattern
func (uc *FileCleanupUseCase) BulkDeleteByPattern(ctx context.Context, req *BulkDeleteByPatternRequest) (*DeleteFilesResponse, error) {
	log.Printf("🗑️ 패턴 기반 일괄 삭제 시작: %s", req.Pattern)

	// Validate pattern
	_, err := regexp.Compile(req.Pattern)
	if err != nil {
		return nil, fmt.Errorf("잘못된 정규표현식 패턴: %w", err)
	}

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileCleanup, 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &DeleteFilesResponse{
		Progress: progress,
		Errors:   make([]string, 0),
	}

	// Start pattern-based deletion in background
	go uc.performPatternBasedDeletion(ctx, req, progress, response)

	return response, nil
}

// CleanupEmptyFolders cleans up empty folders
func (uc *FileCleanupUseCase) CleanupEmptyFolders(ctx context.Context, req *CleanupEmptyFoldersRequest) (*CleanupEmptyFoldersResponse, error) {
	log.Println("📂 빈 폴더 정리 시작")

	// Create progress tracker
	progress, err := uc.progressService.StartOperation(ctx, entities.OperationFileCleanup, 0)
	if err != nil {
		return nil, fmt.Errorf("진행 상황 생성 실패: %w", err)
	}

	// Initialize response
	response := &CleanupEmptyFoldersResponse{
		Progress: progress,
		Errors:   make([]string, 0),
	}

	// Start empty folder cleanup in background
	go uc.performEmptyFolderCleanup(ctx, req, progress, response)

	return response, nil
}

// performFileDeletion performs the actual file deletion
func (uc *FileCleanupUseCase) performFileDeletion(ctx context.Context, req *DeleteFilesRequest, progress *entities.Progress, response *DeleteFilesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 파일 삭제 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 삭제 준비 중...")

	// Validate files if safety checks are enabled
	validFileIDs := req.FileIDs
	if req.SafetyChecks {
		uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 검증 중...")
		validFileIDs = uc.validateFilesForDeletion(ctx, req.FileIDs, response)
	}

	if len(validFileIDs) == 0 {
		log.Println("⚠️ 삭제할 유효한 파일이 없습니다")
		uc.progressService.CompleteOperation(ctx, progress.ID)
		progress.Complete()
		return
	}

	// Calculate space that will be saved
	totalSize := uc.calculateFilesSize(ctx, validFileIDs)
	response.SpaceSaved = totalSize

	// Delete files in batches
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "파일 휴지통 이동 중...")
	uc.deleteFilesInBatches(ctx, validFileIDs, progress, req.ProgressCallback, response)

	// Cleanup empty folders if requested
	if req.CleanupFolders && response.DeletedFiles > 0 {
		uc.progressService.UpdateOperation(ctx, progress.ID, response.DeletedFiles, "빈 폴더 정리 중...")
		deletedFolders := uc.cleanupEmptyFoldersForDeletedFiles(ctx, validFileIDs)
		response.DeletedFolders = deletedFolders
	}

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
	}

	log.Printf("✅ 파일 휴지통 이동 완료: %d개 성공, %d개 실패, %s 절약",
		response.DeletedFiles, response.FailedFiles, formatFileSize(response.SpaceSaved))
}

// performPatternBasedDeletion performs pattern-based file deletion
func (uc *FileCleanupUseCase) performPatternBasedDeletion(ctx context.Context, req *BulkDeleteByPatternRequest, progress *entities.Progress, response *DeleteFilesResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 패턴 기반 삭제 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "패턴 매칭 파일 검색 중...")

	// Get files from folder
	var files []*entities.File
	var err error

	if req.Recursive {
		files, err = uc.getFilesRecursive(ctx, req.FolderID)
	} else {
		files, err = uc.storageProvider.ListFiles(ctx, req.FolderID)
	}

	if err != nil {
		log.Printf("❌ 폴더 파일 조회 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("폴더 파일 조회 실패: %v", err))
		return
	}

	// Filter files by pattern
	pattern, _ := regexp.Compile(req.Pattern)
	matchingFiles := make([]*entities.File, 0)

	for _, file := range files {
		if pattern.MatchString(file.Name) {
			matchingFiles = append(matchingFiles, file)
		}
	}

	log.Printf("📊 패턴 매칭 파일: %d개", len(matchingFiles))

	if len(matchingFiles) == 0 {
		log.Println("⚠️ 패턴에 매칭되는 파일이 없습니다")
		uc.progressService.CompleteOperation(ctx, progress.ID)
		progress.Complete()
		return
	}

	// Update progress total
	progress.SetTotal(len(matchingFiles))
	response.TotalFiles = len(matchingFiles)

	// If dry run, just return the matching files
	if req.DryRun {
		response.DeletedFiles = 0
		response.SpaceSaved = uc.calculateTotalSizeFromFiles(matchingFiles)
		uc.progressService.CompleteOperation(ctx, progress.ID)
		progress.Complete()
		log.Printf("🔍 Dry Run 완료: %d개 파일이 삭제될 예정, %s 절약 예상",
			len(matchingFiles), formatFileSize(response.SpaceSaved))
		return
	}

	// Convert to file IDs and delete
	fileIDs := make([]string, len(matchingFiles))
	for i, file := range matchingFiles {
		fileIDs[i] = file.ID
	}

	// Calculate space that will be saved
	response.SpaceSaved = uc.calculateTotalSizeFromFiles(matchingFiles)

	// Delete files
	uc.deleteFilesInBatches(ctx, fileIDs, progress, nil, response)

	// Cleanup empty folders if requested
	if req.CleanupFolders && response.DeletedFiles > 0 {
		uc.progressService.UpdateOperation(ctx, progress.ID, response.DeletedFiles, "빈 폴더 정리 중...")
		deletedFolders := uc.cleanupEmptyFoldersForDeletedFiles(ctx, fileIDs)
		response.DeletedFolders = deletedFolders
	}

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	log.Printf("✅ 패턴 기반 삭제 완료: %d개 파일 삭제", response.DeletedFiles)
}

// performEmptyFolderCleanup performs empty folder cleanup
func (uc *FileCleanupUseCase) performEmptyFolderCleanup(ctx context.Context, req *CleanupEmptyFoldersRequest, progress *entities.Progress, response *CleanupEmptyFoldersResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ 빈 폴더 정리 중 패닉 발생: %v", r)
			uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("패닉 발생: %v", r))
		}
	}()

	// Update progress to running
	progress.Start()
	uc.progressService.UpdateOperation(ctx, progress.ID, 0, "빈 폴더 검색 중...")

	var deletedCount int
	var err error

	if req.RootFolderID != "" {
		// Clean up specific folder
		if req.Recursive {
			deletedCount, err = uc.cleanupService.CleanupEmptyFoldersInPath(ctx, req.RootFolderID)
		} else {
			deletedCount, err = uc.cleanupService.DeleteEmptyFoldersRecursive(ctx, req.RootFolderID)
		}
	} else {
		// Clean up all empty folders
		deletedCount, err = uc.cleanupService.CleanupEmptyFolders(ctx)
	}

	if err != nil {
		log.Printf("❌ 빈 폴더 정리 실패: %v", err)
		uc.progressService.FailOperation(ctx, progress.ID, fmt.Sprintf("빈 폴더 정리 실패: %v", err))
		response.Errors = append(response.Errors, fmt.Sprintf("빈 폴더 정리 실패: %v", err))
		return
	}

	response.DeletedFolders = deletedCount

	// Complete the operation
	uc.progressService.CompleteOperation(ctx, progress.ID)
	progress.Complete()

	if req.ProgressCallback != nil {
		req.ProgressCallback(progress)
	}

	log.Printf("✅ 빈 폴더 정리 완료: %d개 폴더 삭제", deletedCount)
}

// validateFilesForDeletion validates files before deletion
func (uc *FileCleanupUseCase) validateFilesForDeletion(ctx context.Context, fileIDs []string, response *DeleteFilesResponse) []string {
	validIDs := make([]string, 0, len(fileIDs))

	for _, fileID := range fileIDs {
		// Check if file exists
		exists, err := uc.fileRepo.Exists(ctx, fileID)
		if err != nil {
			log.Printf("⚠️ 파일 존재 확인 실패 [%s]: %v", fileID, err)
			response.Errors = append(response.Errors, fmt.Sprintf("파일 존재 확인 실패 [%s]: %v", fileID, err))
			continue
		}

		if !exists {
			log.Printf("⚠️ 파일이 존재하지 않습니다 [%s]", fileID)
			response.Errors = append(response.Errors, fmt.Sprintf("파일이 존재하지 않습니다 [%s]", fileID))
			continue
		}

		validIDs = append(validIDs, fileID)
	}

	log.Printf("✅ 파일 검증 완료: %d개 유효, %d개 무효", len(validIDs), len(fileIDs)-len(validIDs))
	return validIDs
}

// calculateFilesSize calculates total size of files
func (uc *FileCleanupUseCase) calculateFilesSize(ctx context.Context, fileIDs []string) int64 {
	var totalSize int64

	for _, fileID := range fileIDs {
		file, err := uc.fileRepo.GetByID(ctx, fileID)
		if err == nil && file != nil {
			totalSize += file.Size
		}
	}

	return totalSize
}

// calculateTotalSizeFromFiles calculates total size from file entities
func (uc *FileCleanupUseCase) calculateTotalSizeFromFiles(files []*entities.File) int64 {
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
	}
	return totalSize
}

// deleteFilesInBatches deletes files in batches
func (uc *FileCleanupUseCase) deleteFilesInBatches(ctx context.Context, fileIDs []string, progress *entities.Progress, callback func(*entities.Progress), response *DeleteFilesResponse) {
	totalFiles := len(fileIDs)

	for i := 0; i < totalFiles; i += uc.batchSize {
		end := i + uc.batchSize
		if end > totalFiles {
			end = totalFiles
		}

		batch := fileIDs[i:end]

		// Delete batch
		successCount, errors := uc.deleteBatch(ctx, batch)

		// Update response
		response.DeletedFiles += successCount
		response.FailedFiles += len(batch) - successCount
		response.Errors = append(response.Errors, errors...)

		// Update progress
		processed := i + len(batch)
		progress.UpdateProgress(processed, fmt.Sprintf("파일 휴지통 이동 중... (%d/%d)", processed, totalFiles))
		uc.progressService.UpdateOperation(ctx, progress.ID, processed, progress.CurrentStep)

		// Call progress callback
		if callback != nil {
			callback(progress)
		}

		// Log progress
		if processed%100 == 0 || processed == totalFiles {
			log.Printf("📈 삭제 진행: %d/%d (%.1f%%)", processed, totalFiles, float64(processed)/float64(totalFiles)*100)
		}
	}
}

// deleteBatch deletes a batch of files
func (uc *FileCleanupUseCase) deleteBatch(ctx context.Context, fileIDs []string) (successCount int, errors []string) {
	errors = make([]string, 0)

	// Use worker pool for parallel deletion
	jobs := make(chan string, len(fileIDs))
	results := make(chan error, len(fileIDs))

	// Start workers
	var wg sync.WaitGroup
	for w := 0; w < uc.workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fileID := range jobs {
				err := uc.deleteFile(ctx, fileID)
				results <- err
			}
		}()
	}

	// Send jobs
	for _, fileID := range fileIDs {
		jobs <- fileID
	}
	close(jobs)

	// Wait for workers to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for err := range results {
		if err != nil {
			errors = append(errors, err.Error())
		} else {
			successCount++
		}
	}

	return successCount, errors
}

// deleteFile deletes a single file
func (uc *FileCleanupUseCase) deleteFile(ctx context.Context, fileID string) error {
	// Delete from storage provider (휴지통 이동 방식 사용)
	err := uc.storageProvider.TrashFile(ctx, fileID)
	if err != nil {
		log.Printf("❌ 파일 삭제 실패 [%s]: %v", fileID, err)
		return fmt.Errorf("스토리지에서 파일 삭제 실패 [%s]: %w", fileID, err)
	}
	log.Printf("✅ 파일 휴지통 이동 완료: %s", fileID)

	// Delete from database
	err = uc.fileRepo.Delete(ctx, fileID)
	if err != nil {
		log.Printf("⚠️ 데이터베이스에서 파일 삭제 실패 [%s]: %v", fileID, err)
		// Continue even if DB deletion fails
	}

	return nil
}

// cleanupEmptyFoldersForDeletedFiles cleans up empty folders after file deletion
func (uc *FileCleanupUseCase) cleanupEmptyFoldersForDeletedFiles(ctx context.Context, deletedFileIDs []string) int {
	// Get parent folders of deleted files
	parentFolders := make(map[string]bool)

	for _, fileID := range deletedFileIDs {
		file, err := uc.fileRepo.GetByID(ctx, fileID)
		if err == nil && file != nil {
			for _, parentID := range file.Parents {
				parentFolders[parentID] = true
			}
		}
	}

	// Clean up each parent folder
	deletedCount := 0
	for folderID := range parentFolders {
		count, err := uc.cleanupService.DeleteEmptyFoldersRecursive(ctx, folderID)
		if err != nil {
			log.Printf("⚠️ 폴더 정리 실패 [%s]: %v", folderID, err)
		} else {
			deletedCount += count
		}
	}

	return deletedCount
}

// getFilesRecursive recursively gets files from folder and subfolders
func (uc *FileCleanupUseCase) getFilesRecursive(ctx context.Context, folderID string) ([]*entities.File, error) {
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

	// Recursively get files from subfolders
	for _, subfolder := range subfolders {
		subFiles, err := uc.getFilesRecursive(ctx, subfolder.ID)
		if err != nil {
			log.Printf("⚠️ 하위 폴더 파일 조회 실패 [%s]: %v", subfolder.ID, err)
			continue
		}
		allFiles = append(allFiles, subFiles...)
	}

	return allFiles, nil
}

// GetCleanupProgress returns the current cleanup progress
func (uc *FileCleanupUseCase) GetCleanupProgress(ctx context.Context) (*entities.Progress, error) {
	activeProgress, err := uc.progressService.GetActiveOperations(ctx)
	if err != nil {
		return nil, err
	}

	for _, progress := range activeProgress {
		if progress.OperationType == entities.OperationFileCleanup {
			return progress, nil
		}
	}

	return nil, fmt.Errorf("활성 파일 정리 작업을 찾을 수 없습니다")
}

// SetConfiguration sets the use case configuration
func (uc *FileCleanupUseCase) SetConfiguration(batchSize, workerCount int, safetyChecks, cleanupFolders bool) {
	if batchSize > 0 {
		uc.batchSize = batchSize
	}
	if workerCount > 0 {
		uc.workerCount = workerCount
	}
	uc.safetyChecks = safetyChecks
	uc.cleanupFolders = cleanupFolders
}
