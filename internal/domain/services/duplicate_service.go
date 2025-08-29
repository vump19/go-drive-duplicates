package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// DuplicateFinder defines the interface for duplicate detection strategies
type DuplicateFinder interface {
	// Basic duplicate detection
	FindDuplicates(ctx context.Context, files []*entities.File) ([]*entities.DuplicateGroup, error)
	FindDuplicatesInFolder(ctx context.Context, folderID string) ([]*entities.DuplicateGroup, error)

	// Advanced detection
	FindDuplicatesByHash(ctx context.Context, files []*entities.File) (map[string][]*entities.File, error)
	FindDuplicatesBySize(ctx context.Context, files []*entities.File) (map[int64][]*entities.File, error)
	FindDuplicatesByName(ctx context.Context, files []*entities.File) (map[string][]*entities.File, error)

	// Filtering
	FilterByMinimumFileSize(minSize int64)
	FilterByFileTypes(allowedTypes []string)
	FilterByDateRange(startDate, endDate string)

	// Configuration
	SetMinimumGroupSize(minSize int)
	SetMaximumResults(maxResults int)
	GetConfiguration() map[string]interface{}
}

// DuplicateService defines the domain service for duplicate operations
type DuplicateService interface {
	// Core duplicate operations
	DetectDuplicates(ctx context.Context) ([]*entities.DuplicateGroup, error)
	DetectDuplicatesInFolder(ctx context.Context, folderID string) ([]*entities.DuplicateGroup, error)
	RefreshDuplicateGroups(ctx context.Context) error

	// Group management
	CreateDuplicateGroup(ctx context.Context, hash string, files []*entities.File) (*entities.DuplicateGroup, error)
	AddFileToGroup(ctx context.Context, groupID int, file *entities.File) error
	RemoveFileFromGroup(ctx context.Context, groupID int, fileID string) error
	DeleteGroup(ctx context.Context, groupID int) error

	// Query operations
	GetDuplicateGroups(ctx context.Context, filters map[string]interface{}) ([]*entities.DuplicateGroup, error)
	GetGroupsByMinimumSize(ctx context.Context, minSize int64) ([]*entities.DuplicateGroup, error)
	GetGroupsByFileCount(ctx context.Context, minFiles int) ([]*entities.DuplicateGroup, error)
	GetLargestGroups(ctx context.Context, limit int) ([]*entities.DuplicateGroup, error)

	// Statistics
	CalculateDuplicateStatistics(ctx context.Context) (map[string]interface{}, error)
	GetTotalWastedSpace(ctx context.Context) (int64, error)
	GetDuplicateCount(ctx context.Context) (int, error)

	// Cleanup operations
	CleanupEmptyGroups(ctx context.Context) (int, error)
	CleanupInvalidGroups(ctx context.Context) (int, error)
	MergeGroups(ctx context.Context, groupIDs []int) (*entities.DuplicateGroup, error)

	// Recommendation
	GetDeletionRecommendations(ctx context.Context, groupID int) ([]*entities.File, error)
	SuggestFilesToKeep(ctx context.Context, groupID int) (*entities.File, error)

	// Validation
	ValidateGroup(ctx context.Context, group *entities.DuplicateGroup) error
	VerifyDuplicates(ctx context.Context, groupID int) (bool, error)
}
