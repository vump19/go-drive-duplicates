package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// FileDeleter defines the interface for file deletion strategies
type FileDeleter interface {
	// Basic deletion
	DeleteFile(ctx context.Context, fileID string) error
	DeleteFiles(ctx context.Context, fileIDs []string) error

	// Safe deletion with validation
	SafeDeleteFile(ctx context.Context, fileID string) error
	SafeDeleteFiles(ctx context.Context, fileIDs []string) error

	// Batch deletion
	BatchDelete(ctx context.Context, fileIDs []string, batchSize int) error
	BatchDeleteWithProgress(ctx context.Context, fileIDs []string, batchSize int, progressCallback func(processed, total int)) error

	// Conditional deletion
	DeleteIf(ctx context.Context, fileID string, condition func(*entities.File) bool) error
	DeleteFilesIf(ctx context.Context, fileIDs []string, condition func(*entities.File) bool) (deleted []string, errors []error)

	// Validation
	ValidateBeforeDelete(ctx context.Context, fileID string) error
	CanDelete(ctx context.Context, fileID string) (bool, error)

	// Recovery
	GetDeletionHistory(ctx context.Context, days int) ([]string, error)
	IsRecoverySupported() bool
}

// FolderCleaner defines the interface for folder cleanup operations
type FolderCleaner interface {
	// Empty folder cleanup
	CleanupEmptyFolders(ctx context.Context, rootFolderID string) (int, error)
	FindEmptyFolders(ctx context.Context, rootFolderID string) ([]*entities.File, error)

	// Recursive cleanup
	CleanupEmptyFoldersRecursive(ctx context.Context, rootFolderID string) (int, error)

	// Validation
	IsEmptyFolder(ctx context.Context, folderID string) (bool, error)
	HasSubfolders(ctx context.Context, folderID string) (bool, error)

	// Safety checks
	IsSafeToDelete(ctx context.Context, folderID string) (bool, error)
	GetFolderContents(ctx context.Context, folderID string) ([]*entities.File, error)

	// Progress tracking
	CleanupWithProgress(ctx context.Context, rootFolderID string, progressCallback func(current, total int)) (int, error)
}

// CleanupService defines the domain service for cleanup operations
type CleanupService interface {
	// File cleanup operations
	DeleteDuplicateFiles(ctx context.Context, groupID int, keepFileID string) error
	DeleteFilesFromGroup(ctx context.Context, groupID int, fileIDs []string) error
	BulkDeleteFiles(ctx context.Context, fileIDs []string) error

	// Folder cleanup operations
	CleanupEmptyFolders(ctx context.Context) (int, error)
	CleanupEmptyFoldersInPath(ctx context.Context, rootFolderID string) (int, error)
	DeleteEmptyFoldersRecursive(ctx context.Context, folderID string) (int, error)

	// Pattern-based cleanup
	DeleteFilesByPattern(ctx context.Context, folderID, pattern string) ([]string, error)
	DeleteFilesByExtension(ctx context.Context, folderID string, extensions []string) ([]string, error)
	DeleteOldFiles(ctx context.Context, folderID string, daysOld int) ([]string, error)

	// Smart cleanup
	CleanupDuplicatesInFolder(ctx context.Context, folderID string, strategy string) (int64, error) // Returns space saved
	SuggestCleanupActions(ctx context.Context, folderID string) ([]map[string]interface{}, error)

	// Progress tracking
	DeleteWithProgress(ctx context.Context, fileIDs []string, progressCallback func(deleted, total int, currentFile string)) error
	CleanupWithProgress(ctx context.Context, operations []string, progressCallback func(step string, progress float64)) error

	// Safety and validation
	ValidateCleanupOperation(ctx context.Context, operation string, targets []string) error
	PreviewCleanupOperation(ctx context.Context, operation string, targets []string) (map[string]interface{}, error)
	CanPerformCleanup(ctx context.Context, userID string) (bool, error)

	// Undo operations
	UndoLastCleanup(ctx context.Context, operationID string) error
	GetCleanupHistory(ctx context.Context, days int) ([]map[string]interface{}, error)

	// Statistics
	CalculateCleanupPotential(ctx context.Context, folderID string) (map[string]interface{}, error)
	GetCleanupStatistics(ctx context.Context) (map[string]interface{}, error)

	// Scheduling
	ScheduleCleanup(ctx context.Context, operation string, targets []string, scheduleTime string) error
	CancelScheduledCleanup(ctx context.Context, operationID string) error
	GetScheduledCleanups(ctx context.Context) ([]map[string]interface{}, error)
}
