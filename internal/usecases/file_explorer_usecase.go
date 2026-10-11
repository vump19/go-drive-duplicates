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

	// Cancel functions of file operations running in this process, by operation ID
	runningOps map[int]context.CancelFunc
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
		runningOps:      make(map[int]context.CancelFunc),
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

// fileOperationStep processes a single file of a background file operation.
// It returns the file size in bytes and its name (best effort, zero values if unknown).
type fileOperationStep func(ctx context.Context, fileID string) (size int64, name string, err error)

// fileMetadataGetter provides file metadata (size, name) for progress reporting.
type fileMetadataGetter interface {
	GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
}

// lookupFileInfo returns the size and name of a file, or zero values if unavailable.
func lookupFileInfo(ctx context.Context, getter fileMetadataGetter, fileID string) (int64, string) {
	if getter == nil {
		return 0, ""
	}
	info, err := getter.GetFileMetadata(ctx, fileID)
	if err != nil || info == nil {
		return 0, ""
	}
	return info.Size, info.Name
}

// CopyFiles starts copying files to the target folder in the background.
// Progress can be polled with GetOperationProgress and stopped with CancelOperation.
// 파일을 대상 폴더로 복사 (백그라운드 실행)
func (uc *FileExplorerUseCase) CopyFiles(
	ctx context.Context,
	request *entities.FileOperationRequest,
) (*FileOperationStartedResponse, error) {
	log.Printf("📋 파일 복사 시작: %d개 파일", len(request.FileIDs))

	if err := request.Validate(); err != nil {
		return nil, err
	}

	driveAdapter, ok := uc.driveAdapter.(interface {
		CopyFile(ctx context.Context, fileID, targetFolderID string) (string, error)
		fileMetadataGetter
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support file copy")
	}

	step := func(ctx context.Context, fileID string) (int64, string, error) {
		size, name := lookupFileInfo(ctx, driveAdapter, fileID)
		newFileID, err := driveAdapter.CopyFile(ctx, fileID, request.TargetFolderID)
		if err == nil {
			log.Printf("✅ 파일 복사 완료: %s → %s", fileID, newFileID)
		}
		return size, name, err
	}

	operation := entities.NewFileOperation(
		entities.OperationTypeCopy,
		request.SourceFolderID,
		request.TargetFolderID,
		request.FileIDs,
	)
	return uc.startFileOperation(ctx, operation, "복사", step)
}

// MoveFiles starts moving files to the target folder in the background.
// 파일을 대상 폴더로 이동 (백그라운드 실행)
func (uc *FileExplorerUseCase) MoveFiles(
	ctx context.Context,
	request *entities.FileOperationRequest,
) (*FileOperationStartedResponse, error) {
	log.Printf("📦 파일 이동 시작: %d개 파일", len(request.FileIDs))

	if err := request.Validate(); err != nil {
		return nil, err
	}

	driveAdapter, ok := uc.driveAdapter.(interface {
		MoveFile(ctx context.Context, fileID, targetFolderID string) error
		fileMetadataGetter
	})
	if !ok {
		return nil, fmt.Errorf("storage provider does not support file move")
	}

	step := func(ctx context.Context, fileID string) (int64, string, error) {
		size, name := lookupFileInfo(ctx, driveAdapter, fileID)
		err := driveAdapter.MoveFile(ctx, fileID, request.TargetFolderID)
		if err == nil {
			log.Printf("✅ 파일 이동 완료: %s", fileID)
		}
		return size, name, err
	}

	operation := entities.NewFileOperation(
		entities.OperationTypeMove,
		request.SourceFolderID,
		request.TargetFolderID,
		request.FileIDs,
	)
	return uc.startFileOperation(ctx, operation, "이동", step)
}

// DeleteFiles starts moving files to trash (or permanently deleting them) in the background.
// 파일 삭제 (기본 휴지통 이동, permanentDelete 시 완전 삭제, 백그라운드 실행)
func (uc *FileExplorerUseCase) DeleteFiles(
	ctx context.Context,
	fileIDs []string,
	permanentDelete bool,
) (*FileOperationStartedResponse, error) {
	actionText := "휴지통 이동"
	if permanentDelete {
		actionText = "완전 삭제"
	}
	log.Printf("🗑️ 파일 %s 시작: %d개 파일", actionText, len(fileIDs))

	if len(fileIDs) == 0 {
		return nil, entities.ErrNoFilesSelected
	}

	metadataGetter, _ := uc.driveAdapter.(fileMetadataGetter)

	step := func(ctx context.Context, fileID string) (int64, string, error) {
		_, name := lookupFileInfo(ctx, metadataGetter, fileID)
		var err error
		if permanentDelete {
			err = uc.storageProvider.DeleteFile(ctx, fileID)
		} else {
			err = uc.storageProvider.TrashFile(ctx, fileID)
		}
		if err == nil {
			log.Printf("✅ 파일 %s 완료: %s", actionText, fileID)
		}
		return 0, name, err
	}

	operation := entities.NewFileOperation(entities.OperationTypeDelete, "", "", fileIDs)
	return uc.startFileOperation(ctx, operation, actionText, step)
}

// startFileOperation records the operation and runs it in the background with a cancellable context.
func (uc *FileExplorerUseCase) startFileOperation(
	ctx context.Context,
	operation *entities.FileOperation,
	actionText string,
	step fileOperationStep,
) (*FileOperationStartedResponse, error) {
	if err := uc.operationRepo.Create(ctx, operation); err != nil {
		return nil, fmt.Errorf("작업 기록 생성 실패: %w", err)
	}

	operation.Start()
	if err := uc.operationRepo.Update(ctx, operation); err != nil {
		log.Printf("⚠️ 작업 시작 상태 업데이트 실패: %v", err)
	}

	// The request context ends with the HTTP request, so the work gets its own context
	bgCtx, cancel := context.WithCancel(context.Background())
	uc.mu.Lock()
	uc.runningOps[operation.ID] = cancel
	uc.mu.Unlock()

	go uc.runFileOperation(bgCtx, operation, actionText, step)

	return &FileOperationStartedResponse{
		OperationID: operation.ID,
		TotalFiles:  operation.TotalFiles,
	}, nil
}

// runFileOperation processes the files one by one, stopping when the context is cancelled.
// It is the only writer of the operation's final status.
func (uc *FileExplorerUseCase) runFileOperation(
	ctx context.Context,
	operation *entities.FileOperation,
	actionText string,
	step fileOperationStep,
) {
	// Repository writes must not fail because the work context was cancelled
	dbCtx := context.Background()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("💥 파일 %s 중 패닉 발생: %v", actionText, r)
			operation.Fail(fmt.Sprintf("패닉 발생: %v", r))
			if err := uc.operationRepo.Update(dbCtx, operation); err != nil {
				log.Printf("⚠️ 작업 상태 업데이트 실패: %v", err)
			}
		}

		uc.mu.Lock()
		if cancel, ok := uc.runningOps[operation.ID]; ok {
			cancel()
			delete(uc.runningOps, operation.ID)
		}
		uc.mu.Unlock()
	}()

	total := len(operation.FileIDs)
	successCount := 0

	for i, fileID := range operation.FileIDs {
		if ctx.Err() != nil {
			break
		}

		size, name, err := step(ctx, fileID)
		operation.TotalBytes += size

		if err != nil {
			if ctx.Err() != nil {
				// 취소로 중단된 파일은 오류로 기록하지 않음
				break
			}
			operation.AddError(fileID, name, err.Error())
			log.Printf("❌ 파일 %s 실패 (%s): %v", actionText, fileID, err)
		} else {
			successCount++
			operation.ProcessedBytes += size
		}

		operation.UpdateProgress(i+1, operation.ProcessedBytes)
		if err := uc.operationRepo.UpdateProgress(dbCtx, operation.ID, operation.ProcessedFiles, operation.ProcessedBytes); err != nil {
			log.Printf("⚠️ 진행 상황 업데이트 실패: %v", err)
		}
	}

	switch {
	case ctx.Err() != nil:
		operation.Cancel()
		log.Printf("🛑 파일 %s 취소됨: %d/%d 처리 후 중단 (성공 %d)", actionText, operation.ProcessedFiles, total, successCount)
	case successCount == total:
		operation.Complete()
		log.Printf("🎉 파일 %s 완료: %d/%d 성공", actionText, successCount, total)
	case successCount > 0:
		operation.Fail(fmt.Sprintf("%d/%d 파일 %s 성공", successCount, total, actionText))
		log.Printf("⚠️ 파일 %s 일부 실패: %d/%d 성공", actionText, successCount, total)
	default:
		operation.Fail(fmt.Sprintf("모든 파일 %s 실패", actionText))
		log.Printf("❌ 모든 파일 %s 실패", actionText)
	}

	if err := uc.operationRepo.Update(dbCtx, operation); err != nil {
		log.Printf("⚠️ 작업 완료 상태 업데이트 실패: %v", err)
	}
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

	// Create folder and get its ID
	folderID, err := uc.storageProvider.CreateFolder(ctx, parentFolderID, folderName)
	if err != nil {
		return nil, fmt.Errorf("폴더 생성 실패: %w", err)
	}
	log.Printf("✅ 폴더 생성 완료: %s (%s)", folderName, folderID)

	basicItem := entities.NewFileListItem(folderID, folderName, "application/vnd.google-apps.folder")

	// Get complete folder metadata if the provider supports it
	metadataProvider, ok := uc.driveAdapter.(interface {
		GetFileMetadata(ctx context.Context, fileID string) (*entities.FileListItem, error)
	})
	if !ok {
		return basicItem, nil
	}

	folderItem, err := metadataProvider.GetFileMetadata(ctx, folderID)
	if err != nil {
		log.Printf("⚠️ 폴더 메타데이터 조회 실패, 기본 정보 반환: %v", err)
		return basicItem, nil
	}
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

	uc.mu.Lock()
	cancel, running := uc.runningOps[operationID]
	uc.mu.Unlock()

	if running {
		// The background goroutine stops after the current file and records the cancelled status
		cancel()
		log.Printf("✅ 작업 취소 요청 전달: ID=%d", operationID)
		return nil
	}

	// Not running in this process (e.g. the server restarted): mark it cancelled directly
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

// FileOperationStartedResponse is returned when a background file operation (copy, move, delete) starts.
// Per-file results are available through GetOperationProgress (errorDetails).
type FileOperationStartedResponse struct {
	OperationID int `json:"operationId"`
	TotalFiles  int `json:"totalFiles"`
}

type FolderSizeInfo struct {
	FolderID       string           `json:"folderId"`
	TotalSize      int64            `json:"totalSize"`      // Total size in bytes
	FileCount      int              `json:"fileCount"`      // Number of files
	FolderCount    int              `json:"folderCount"`    // Number of subfolders
	SubfolderSizes map[string]int64 `json:"subfolderSizes"` // Size of each immediate subfolder
}
