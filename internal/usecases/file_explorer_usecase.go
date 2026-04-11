package usecases

import (
	"context"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"go-drive-duplicates/internal/domain/services"
	"log"
	"sync"
)

// FileExplorerUseCase handles file explorer business logic
// 파일 탐색기 비즈니스 로직 처리
type FileExplorerUseCase struct {
	storageProvider services.StorageProvider
	operationRepo   repositories.FileOperationRepository
	driveAdapter    interface{} // Google Drive specific adapter
	mu              sync.Mutex  // For thread-safe operation tracking
}

// NewFileExplorerUseCase creates a new file explorer use case
func NewFileExplorerUseCase(
	storageProvider services.StorageProvider,
	operationRepo repositories.FileOperationRepository,
) *FileExplorerUseCase {
	return &FileExplorerUseCase{
		storageProvider: storageProvider,
		operationRepo:   operationRepo,
		driveAdapter:    storageProvider, // Cast to specific adapter if needed
	}
}

// GetFolderTree retrieves folder tree structure
// 폴더 트리 구조 조회
func (uc *FileExplorerUseCase) GetFolderTree(
	ctx context.Context,
	rootFolderID string,
	options *entities.FolderTreeOptions,
) (*entities.FolderNode, error) {
	log.Printf("🌲 폴더 트리 조회 시작: %s", rootFolderID)

	if options == nil {
		options = entities.DefaultFolderTreeOptions()
	}

	// Type assert to access Google Drive specific methods
	driveAdapter, ok := uc.driveAdapter.(interface {
		ListFoldersRecursive(ctx context.Context, folderID string, maxDepth int) ([]*entities.FolderNode, error)
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support folder tree operations")
	}

	// Get folder tree from Google Drive
	folders, err := driveAdapter.ListFoldersRecursive(ctx, rootFolderID, options.MaxDepth)
	if err != nil {
		return nil, fmt.Errorf("폴더 트리 조회 실패: %w", err)
	}

	if len(folders) == 0 {
		return nil, fmt.Errorf("폴더를 찾을 수 없습니다")
	}

	// The first folder is the root
	rootNode := folders[0]

	// Apply expand all if requested
	if options.ExpandAll {
		rootNode.ExpandAll()
	}

	log.Printf("✅ 폴더 트리 조회 완료: %d개 폴더", len(folders))
	return rootNode, nil
}

// ListFolderContents lists files and folders in a folder
// 폴더 내 파일 및 폴더 목록 조회
func (uc *FileExplorerUseCase) ListFolderContents(
	ctx context.Context,
	folderID string,
	options *entities.ListOptions,
) (*entities.FileListResponse, error) {
	log.Printf("📂 폴더 내용 조회: %s", folderID)

	if options == nil {
		options = entities.DefaultListOptions()
	}

	// Validate options
	if err := options.Validate(); err != nil {
		return nil, fmt.Errorf("유효하지 않은 옵션: %w", err)
	}

	// Type assert to access Google Drive specific methods
	driveAdapter, ok := uc.driveAdapter.(interface {
		ListFolderContents(ctx context.Context, folderID string, options *entities.ListOptions) (*entities.FileListResponse, error)
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support folder listing")
	}

	// Get folder contents from Google Drive
	response, err := driveAdapter.ListFolderContents(ctx, folderID, options)
	if err != nil {
		return nil, fmt.Errorf("폴더 내용 조회 실패: %w", err)
	}

	log.Printf("✅ 폴더 내용 조회 완료: %d개 항목", len(response.Items))
	return response, nil
}

// CopyFiles copies files to target folder
// 파일을 대상 폴더로 복사
func (uc *FileExplorerUseCase) CopyFiles(
	ctx context.Context,
	request *entities.FileOperationRequest,
) (*CopyFilesResponse, error) {
	log.Printf("📋 파일 복사 시작: %d개 파일", len(request.FileIDs))

	// Validate request
	if err := request.Validate(); err != nil {
		return nil, err
	}

	// Create operation record
	operation := entities.NewFileOperation(
		entities.OperationTypeCopy,
		request.SourceFolderID,
		request.TargetFolderID,
		request.FileIDs,
	)

	if err := uc.operationRepo.Create(ctx, operation); err != nil {
		return nil, fmt.Errorf("작업 기록 생성 실패: %w", err)
	}

	// Start operation
	operation.Start()
	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️  작업 시작 상태 업데이트 실패: %v", err)
	}

	// Type assert to access copy method
	driveAdapter, ok := uc.driveAdapter.(interface {
		CopyFile(ctx context.Context, fileID, targetFolderID string) (string, error)
		GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
	})
	if !ok {
		operation.Fail("storage provider does not support file copy")
		uc.operationRepo.Update(ctx, operation)
		return nil, fmt.Errorf("storage provider does not support file copy")
	}

	// Copy files
	results := make([]*services.CopyResult, 0, len(request.FileIDs))
	successCount := 0

	for i, fileID := range request.FileIDs {
		// Get file metadata for size calculation
		var fileSize int64
		if fileInfo, err := driveAdapter.GetFileMetadata(ctx, fileID); err == nil {
			fileSize = fileInfo.Size
			operation.TotalBytes += fileSize
		}

		// Copy file
		newFileID, err := driveAdapter.CopyFile(ctx, fileID, request.TargetFolderID)

		result := &services.CopyResult{
			OriginalFileID: fileID,
			NewFileID:      newFileID,
			Success:        err == nil,
		}

		if err != nil {
			result.Error = err.Error()
			operation.AddError(fileID, "", err.Error())
			log.Printf("❌ 파일 복사 실패 (%s): %v", fileID, err)
		} else {
			successCount++
			operation.ProcessedBytes += fileSize
			log.Printf("✅ 파일 복사 완료: %s → %s", fileID, newFileID)
		}

		results = append(results, result)

		// Update progress
		operation.UpdateProgress(i+1, operation.ProcessedBytes)
		if err := uc.operationRepo.UpdateProgress(ctx, operation.ID, operation.ProcessedFiles, operation.ProcessedBytes); err != nil {
			log.Printf("⚠️  진행 상황 업데이트 실패: %v", err)
		}
	}

	// Complete operation
	if successCount == len(request.FileIDs) {
		operation.Complete()
	} else if successCount > 0 {
		operation.Fail(fmt.Sprintf("%d/%d 파일 복사 성공", successCount, len(request.FileIDs)))
	} else {
		operation.Fail("모든 파일 복사 실패")
	}

	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️  작업 완료 상태 업데이트 실패: %v", err)
	}

	log.Printf("🎉 파일 복사 완료: %d/%d 성공", successCount, len(request.FileIDs))

	return &CopyFilesResponse{
		OperationID:  operation.ID,
		Results:      results,
		SuccessCount: successCount,
		FailureCount: len(request.FileIDs) - successCount,
	}, nil
}

// MoveFiles moves files to target folder
// 파일을 대상 폴더로 이동
func (uc *FileExplorerUseCase) MoveFiles(
	ctx context.Context,
	request *entities.FileOperationRequest,
) (*MoveFilesResponse, error) {
	log.Printf("📦 파일 이동 시작: %d개 파일", len(request.FileIDs))

	// Validate request
	if err := request.Validate(); err != nil {
		return nil, err
	}

	// Create operation record
	operation := entities.NewFileOperation(
		entities.OperationTypeMove,
		request.SourceFolderID,
		request.TargetFolderID,
		request.FileIDs,
	)

	if err := uc.operationRepo.Create(ctx, operation); err != nil {
		return nil, fmt.Errorf("작업 기록 생성 실패: %w", err)
	}

	// Start operation
	operation.Start()
	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️  작업 시작 상태 업데이트 실패: %v", err)
	}

	// Type assert to access move method
	driveAdapter, ok := uc.driveAdapter.(interface {
		MoveFile(ctx context.Context, fileID, targetFolderID string) error
		GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
	})
	if !ok {
		operation.Fail("storage provider does not support file move")
		uc.operationRepo.Update(ctx, operation)
		return nil, fmt.Errorf("storage provider does not support file move")
	}

	// Move files
	results := make([]*services.MoveResult, 0, len(request.FileIDs))
	successCount := 0

	for i, fileID := range request.FileIDs {
		// Get file metadata
		var fileName string
		var fileSize int64
		if fileInfo, err := driveAdapter.GetFileMetadata(ctx, fileID); err == nil {
			fileName = fileInfo.Name
			fileSize = fileInfo.Size
			operation.TotalBytes += fileSize
		}

		// Move file
		err := driveAdapter.MoveFile(ctx, fileID, request.TargetFolderID)

		result := &services.MoveResult{
			FileID:   fileID,
			FileName: fileName,
			Success:  err == nil,
		}

		if err != nil {
			result.Error = err.Error()
			operation.AddError(fileID, fileName, err.Error())
			log.Printf("❌ 파일 이동 실패 (%s): %v", fileID, err)
		} else {
			successCount++
			operation.ProcessedBytes += fileSize
			log.Printf("✅ 파일 이동 완료: %s", fileID)
		}

		results = append(results, result)

		// Update progress
		operation.UpdateProgress(i+1, operation.ProcessedBytes)
		if err := uc.operationRepo.UpdateProgress(ctx, operation.ID, operation.ProcessedFiles, operation.ProcessedBytes); err != nil {
			log.Printf("⚠️  진행 상황 업데이트 실패: %v", err)
		}
	}

	// Complete operation
	if successCount == len(request.FileIDs) {
		operation.Complete()
	} else if successCount > 0 {
		operation.Fail(fmt.Sprintf("%d/%d 파일 이동 성공", successCount, len(request.FileIDs)))
	} else {
		operation.Fail("모든 파일 이동 실패")
	}

	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️  작업 완료 상태 업데이트 실패: %v", err)
	}

	log.Printf("🎉 파일 이동 완료: %d/%d 성공", successCount, len(request.FileIDs))

	return &MoveFilesResponse{
		OperationID:  operation.ID,
		Results:      results,
		SuccessCount: successCount,
		FailureCount: len(request.FileIDs) - successCount,
	}, nil
}

// DeleteFiles deletes files (permanent delete or move to trash)
// 파일 삭제 (완전 삭제 또는 휴지통 이동)
func (uc *FileExplorerUseCase) DeleteFiles(
	ctx context.Context,
	fileIDs []string,
	permanentDelete bool,
) (*ExplorerDeleteFilesResponse, error) {
	actionText := "휴지통 이동"
	if permanentDelete {
		actionText = "완전 삭제"
	}
	log.Printf("🗑️  파일 %s 시작: %d개 파일", actionText, len(fileIDs))

	if len(fileIDs) == 0 {
		return nil, entities.ErrNoFilesSelected
	}

	// Create operation record
	operation := entities.NewFileOperation(
		entities.OperationTypeDelete,
		"", // No source folder for delete
		"", // No target folder for delete
		fileIDs,
	)

	if err := uc.operationRepo.Create(ctx, operation); err != nil {
		return nil, fmt.Errorf("작업 기록 생성 실패: %w", err)
	}

	// Start operation
	operation.Start()
	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️  작업 시작 상태 업데이트 실패: %v", err)
	}

	// Delete files using storage provider
	results := make([]*services.DeleteResult, 0, len(fileIDs))
	successCount := 0

	for i, fileID := range fileIDs {
		// Get file metadata first
		var fileName string
		driveAdapter, ok := uc.driveAdapter.(interface {
			GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
		})
		if ok {
			if fileInfo, err := driveAdapter.GetFileMetadata(ctx, fileID); err == nil {
				fileName = fileInfo.Name
			}
		}

		// Delete file (permanent or trash)
		var err error
		if permanentDelete {
			err = uc.storageProvider.DeleteFile(ctx, fileID)
		} else {
			err = uc.storageProvider.TrashFile(ctx, fileID)
		}

		result := &services.DeleteResult{
			FileID:   fileID,
			FileName: fileName,
			Success:  err == nil,
		}

		if err != nil {
			result.Error = err.Error()
			operation.AddError(fileID, fileName, err.Error())
			log.Printf("❌ 파일 %s 실패 (%s): %v", actionText, fileID, err)
		} else {
			successCount++
			log.Printf("✅ 파일 %s 완료: %s", actionText, fileID)
		}

		results = append(results, result)

		// Update progress
		operation.UpdateProgress(i+1, 0)
		if err := uc.operationRepo.UpdateProgress(ctx, operation.ID, operation.ProcessedFiles, 0); err != nil {
			log.Printf("⚠️  진행 상황 업데이트 실패: %v", err)
		}
	}

	// Complete operation
	if successCount == len(fileIDs) {
		operation.Complete()
	} else if successCount > 0 {
		operation.Fail(fmt.Sprintf("%d/%d 파일 %s 성공", successCount, len(fileIDs), actionText))
	} else {
		operation.Fail(fmt.Sprintf("모든 파일 %s 실패", actionText))
	}

	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️  작업 완료 상태 업데이트 실패: %v", err)
	}

	log.Printf("🎉 파일 %s 완료: %d/%d 성공", actionText, successCount, len(fileIDs))

	return &ExplorerDeleteFilesResponse{
		OperationID:  operation.ID,
		Results:      results,
		SuccessCount: successCount,
		FailureCount: len(fileIDs) - successCount,
	}, nil
}

// RenameFile renames a file
// 파일 이름 변경
func (uc *FileExplorerUseCase) RenameFile(
	ctx context.Context,
	fileID string,
	newName string,
) error {
	log.Printf("✏️  파일 이름 변경: %s → %s", fileID, newName)

	if fileID == "" || newName == "" {
		return fmt.Errorf("fileID and newName are required")
	}

	// Type assert to access rename method
	driveAdapter, ok := uc.driveAdapter.(interface {
		RenameFile(ctx context.Context, fileID, newName string) error
	})
	if !ok {
		return fmt.Errorf("storage provider does not support file rename")
	}

	// Rename file
	if err := driveAdapter.RenameFile(ctx, fileID, newName); err != nil {
		return fmt.Errorf("파일 이름 변경 실패: %w", err)
	}

	log.Printf("✅ 파일 이름 변경 완료: %s", fileID)
	return nil
}

// CreateFolder creates a new folder
// 새 폴더 생성
func (uc *FileExplorerUseCase) CreateFolder(
	ctx context.Context,
	parentFolderID string,
	folderName string,
) (*entities.FileListItem, error) {
	log.Printf("📁 새 폴더 생성: %s (부모: %s)", folderName, parentFolderID)

	if parentFolderID == "" || folderName == "" {
		return nil, fmt.Errorf("parentFolderID and folderName are required")
	}

	// Type assert to access create folder method
	type folderCreator interface {
		CreateFolderReturningID(ctx context.Context, parentID, folderName string) (string, error)
		GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
	}

	driveAdapter, ok := uc.driveAdapter.(folderCreator)
	if !ok {
		return nil, fmt.Errorf("storage provider does not support folder creation")
	}

	// Create folder and get its ID
	folderID, err := driveAdapter.CreateFolderReturningID(ctx, parentFolderID, folderName)
	if err != nil {
		return nil, fmt.Errorf("폴더 생성 실패: %w", err)
	}

	// Get complete folder metadata
	folderItem, err := driveAdapter.GetFileMetadata(ctx, folderID)
	if err != nil {
		log.Printf("⚠️  폴더 메타데이터 조회 실패: %v", err)
		// Return basic folder info
		return entities.NewFileListItem(folderID, folderName, "application/vnd.google-apps.folder"), nil
	}

	log.Printf("✅ 폴더 생성 완료: %s (%s)", folderName, folderID)
	return folderItem, nil
}

// GetFileInfo retrieves file metadata
// 파일 메타데이터 조회
func (uc *FileExplorerUseCase) GetFileInfo(
	ctx context.Context,
	fileID string,
) (*entities.FileListItem, error) {
	log.Printf("📄 파일 정보 조회: %s", fileID)

	if fileID == "" {
		return nil, fmt.Errorf("fileID is required")
	}

	// Type assert to access metadata method
	driveAdapter, ok := uc.driveAdapter.(interface {
		GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support file metadata")
	}

	// Get file metadata
	fileInfo, err := driveAdapter.GetFileMetadata(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("파일 정보 조회 실패: %w", err)
	}

	// Get folder path if the item has a parent
	if len(fileInfo.Parents) > 0 {
		// Type assert to access GetFolderPath method
		if pathProvider, ok := uc.driveAdapter.(interface {
			GetFolderPath(ctx context.Context, folderID string) (string, error)
		}); ok {
			parentPath, pathErr := pathProvider.GetFolderPath(ctx, fileInfo.Parents[0])
			if pathErr == nil && parentPath != "" {
				fileInfo.Path = parentPath + "/" + fileInfo.Name
			} else {
				// If we can't get the parent path, at least set the name
				fileInfo.Path = "/" + fileInfo.Name
			}
		} else {
			fileInfo.Path = "/" + fileInfo.Name
		}
	} else {
		// Root level item
		fileInfo.Path = "/" + fileInfo.Name
	}

	log.Printf("✅ 파일 정보 조회 완료: %s (경로: %s)", fileInfo.Name, fileInfo.Path)
	return fileInfo, nil
}

// SearchFiles searches files by query
// 쿼리로 파일 검색
func (uc *FileExplorerUseCase) SearchFiles(
	ctx context.Context,
	query string,
	options *entities.ListOptions,
) (*entities.FileListResponse, error) {
	log.Printf("🔍 파일 검색: %s", query)

	if query == "" {
		return nil, fmt.Errorf("search query is required")
	}

	if options == nil {
		options = entities.DefaultListOptions()
	}

	// Type assert to access search method
	driveAdapter, ok := uc.driveAdapter.(interface {
		SearchFilesWithOptions(ctx context.Context, query string, options *entities.ListOptions) (*entities.FileListResponse, error)
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support file search")
	}

	// Search files
	response, err := driveAdapter.SearchFilesWithOptions(ctx, query, options)
	if err != nil {
		return nil, fmt.Errorf("파일 검색 실패: %w", err)
	}

	log.Printf("✅ 파일 검색 완료: %d개 항목", len(response.Items))
	return response, nil
}

// GetOperationProgress retrieves operation progress
// 작업 진행 상황 조회
func (uc *FileExplorerUseCase) GetOperationProgress(
	ctx context.Context,
	operationID int,
) (*entities.FileOperation, error) {
	operation, err := uc.operationRepo.GetByID(ctx, operationID)
	if err != nil {
		return nil, fmt.Errorf("작업 조회 실패: %w", err)
	}

	return operation, nil
}

// CancelOperation cancels a running operation
// 실행 중인 작업 취소
func (uc *FileExplorerUseCase) CancelOperation(
	ctx context.Context,
	operationID int,
) error {
	log.Printf("🛑 작업 취소 요청: ID=%d", operationID)

	operation, err := uc.operationRepo.GetByID(ctx, operationID)
	if err != nil {
		return fmt.Errorf("작업 조회 실패: %w", err)
	}

	if !operation.IsRunning() {
		return entities.ErrOperationNotRunning
	}

	operation.Cancel()
	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		return fmt.Errorf("작업 취소 상태 업데이트 실패: %w", err)
	}

	log.Printf("✅ 작업 취소 완료: ID=%d", operationID)
	return nil
}

// GetFolderSize calculates total size of a folder including all subfolders
// 폴더의 전체 크기 계산 (모든 하위 폴더 포함)
func (uc *FileExplorerUseCase) GetFolderSize(
	ctx context.Context,
	folderID string,
) (*FolderSizeInfo, error) {
	log.Printf("📊 폴더 크기 계산 시작: %s", folderID)

	if folderID == "" {
		return nil, fmt.Errorf("folderID is required")
	}

	// Type assert to access folder listing method
	driveAdapter, ok := uc.driveAdapter.(interface {
		ListFolderContents(ctx context.Context, folderID string, options *entities.ListOptions) (*entities.FileListResponse, error)
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support folder listing")
	}

	// Initialize size info
	sizeInfo := &FolderSizeInfo{
		FolderID:       folderID,
		TotalSize:      0,
		FileCount:      0,
		FolderCount:    0,
		SubfolderSizes: make(map[string]int64),
	}

	// Calculate size recursively with depth limit
	maxDepth := 10 // Limit recursion depth to prevent timeout
	if err := uc.calculateFolderSizeRecursiveWithDepth(ctx, folderID, sizeInfo, driveAdapter, 0, maxDepth); err != nil {
		return nil, fmt.Errorf("폴더 크기 계산 실패: %w", err)
	}

	log.Printf("✅ 폴더 크기 계산 완료: %s - %d 파일, %d 폴더, %d 바이트",
		folderID, sizeInfo.FileCount, sizeInfo.FolderCount, sizeInfo.TotalSize)

	return sizeInfo, nil
}

// calculateFolderSizeRecursiveWithDepth recursively calculates folder size with depth limit
func (uc *FileExplorerUseCase) calculateFolderSizeRecursiveWithDepth(
	ctx context.Context,
	folderID string,
	sizeInfo *FolderSizeInfo,
	driveAdapter interface {
		ListFolderContents(ctx context.Context, folderID string, options *entities.ListOptions) (*entities.FileListResponse, error)
	},
	currentDepth int,
	maxDepth int,
) error {
	// Check depth limit
	if currentDepth >= maxDepth {
		log.Printf("⚠️  최대 깊이 도달 (depth=%d): %s", currentDepth, folderID)
		return nil
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// List all contents of the folder (with pagination)
	options := entities.DefaultListOptions()
	options.PageSize = 1000 // Large page size for efficiency

	var allItems []*entities.FileListItem
	nextPageToken := ""

	for {
		if nextPageToken != "" {
			options.PageToken = nextPageToken
		}

		response, err := driveAdapter.ListFolderContents(ctx, folderID, options)
		if err != nil {
			return fmt.Errorf("폴더 내용 조회 실패 (%s): %w", folderID, err)
		}

		allItems = append(allItems, response.Items...)

		// Check if there are more pages
		if response.NextPageToken == "" {
			break
		}
		nextPageToken = response.NextPageToken
	}

	// Process each item
	for _, item := range allItems {
		if item.IsFolder {
			// Count folder
			sizeInfo.FolderCount++

			// Recursively calculate subfolder size
			subfolderSize := int64(0)
			subfolderInfo := &FolderSizeInfo{
				FolderID:       item.ID,
				TotalSize:      0,
				FileCount:      0,
				FolderCount:    0,
				SubfolderSizes: make(map[string]int64),
			}

			if err := uc.calculateFolderSizeRecursiveWithDepth(ctx, item.ID, subfolderInfo, driveAdapter, currentDepth+1, maxDepth); err != nil {
				log.Printf("⚠️  하위 폴더 크기 계산 실패 (depth=%d, %s): %v", currentDepth+1, item.ID, err)
				// Continue with other folders instead of failing entirely
				continue
			}

			subfolderSize = subfolderInfo.TotalSize
			sizeInfo.TotalSize += subfolderSize
			sizeInfo.FileCount += subfolderInfo.FileCount
			sizeInfo.FolderCount += subfolderInfo.FolderCount
			sizeInfo.SubfolderSizes[item.ID] = subfolderSize

		} else {
			// Add file size
			sizeInfo.TotalSize += item.Size
			sizeInfo.FileCount++
		}
	}

	return nil
}

// Response types

type CopyFilesResponse struct {
	OperationID  int                    `json:"operationId"`
	Results      []*services.CopyResult `json:"results"`
	SuccessCount int                    `json:"successCount"`
	FailureCount int                    `json:"failureCount"`
}

type MoveFilesResponse struct {
	OperationID  int                    `json:"operationId"`
	Results      []*services.MoveResult `json:"results"`
	SuccessCount int                    `json:"successCount"`
	FailureCount int                    `json:"failureCount"`
}

type ExplorerDeleteFilesResponse struct {
	OperationID  int                      `json:"operationId"`
	Results      []*services.DeleteResult `json:"results"`
	SuccessCount int                      `json:"successCount"`
	FailureCount int                      `json:"failureCount"`
}

type FolderSizeInfo struct {
	FolderID       string           `json:"folderId"`
	TotalSize      int64            `json:"totalSize"`      // Total size in bytes
	FileCount      int              `json:"fileCount"`      // Number of files
	FolderCount    int              `json:"folderCount"`    // Number of subfolders
	SubfolderSizes map[string]int64 `json:"subfolderSizes"` // Size of each immediate subfolder
}
