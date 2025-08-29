package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
	"io"
)

// StorageProvider defines the interface for external storage providers (Google Drive, OneDrive, etc.)
type StorageProvider interface {
	// Authentication
	Authenticate(ctx context.Context) error
	IsAuthenticated() bool

	// File operations
	ListFiles(ctx context.Context, folderID string) ([]*entities.File, error)
	ListAllFiles(ctx context.Context) ([]*entities.File, error)
	GetFile(ctx context.Context, fileID string) (*entities.File, error)
	DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error)
	DeleteFile(ctx context.Context, fileID string) error
	DeleteFiles(ctx context.Context, fileIDs []string) error

	// Folder operations
	GetFolder(ctx context.Context, folderID string) (*entities.File, error)
	ListFolders(ctx context.Context, parentID string) ([]*entities.File, error)
	DeleteFolder(ctx context.Context, folderID string) error
	GetFolderPath(ctx context.Context, folderID string) (string, error)

	// Search operations
	SearchFiles(ctx context.Context, query string) ([]*entities.File, error)
	SearchByMimeType(ctx context.Context, mimeType string) ([]*entities.File, error)
	SearchByName(ctx context.Context, name string) ([]*entities.File, error)

	// Metadata operations
	GetFileParents(ctx context.Context, fileID string) ([]string, error)
	UpdateFileMetadata(ctx context.Context, fileID string, metadata map[string]interface{}) error

	// Quota and limits
	GetQuota(ctx context.Context) (used, total int64, err error)
	GetRateLimit() (requestsPerSecond int)

	// Batch operations
	BatchDelete(ctx context.Context, fileIDs []string, batchSize int) error
	BatchGetFiles(ctx context.Context, fileIDs []string) ([]*entities.File, error)

	// Provider-specific operations
	GetProviderName() string
	GetMaxBatchSize() int
	SupportsResumableDownload() bool
}

// FileService defines the domain service for file operations
type FileService interface {
	// File management
	ScanFiles(ctx context.Context, folderID string) ([]*entities.File, error)
	ScanAllFiles(ctx context.Context) ([]*entities.File, error)
	RefreshFileMetadata(ctx context.Context, fileID string) (*entities.File, error)

	// Path operations
	UpdateFilePaths(ctx context.Context, files []*entities.File) error
	BuildFileTree(ctx context.Context, files []*entities.File) (map[string][]*entities.File, error)

	// Cleanup operations
	CleanupDeletedFiles(ctx context.Context) (int, error)
	CleanupEmptyFolders(ctx context.Context) (int, error)

	// Validation
	ValidateFileAccess(ctx context.Context, fileID string) error
	CheckFileExists(ctx context.Context, fileID string) (bool, error)

	// Statistics
	GenerateFileStatistics(ctx context.Context) (*entities.FileStatistics, error)
	GetStorageUsage(ctx context.Context) (used, total int64, err error)
}
