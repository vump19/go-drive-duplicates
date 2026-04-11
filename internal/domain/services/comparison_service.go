package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// FolderComparator defines the interface for folder comparison strategies
type FolderComparator interface {
	// Basic comparison
	Compare(ctx context.Context, sourceFolderID, targetFolderID string) (*entities.ComparisonResult, error)
	CompareWithProgress(ctx context.Context, sourceFolderID, targetFolderID string, progressCallback func(step string, progress float64)) (*entities.ComparisonResult, error)

	// Advanced comparison options
	SetDeepComparison(enabled bool)        // Compare file contents, not just metadata
	SetIncludeSubfolders(enabled bool)     // Recursively compare subfolders
	SetIgnoreFileTypes(fileTypes []string) // Skip certain file types
	SetMinimumFileSize(minSize int64)      // Skip files smaller than threshold

	// Comparison strategies
	CompareByHash(ctx context.Context, sourceFiles, targetFiles []*entities.File) ([]*entities.File, error)
	CompareByMetadata(ctx context.Context, sourceFiles, targetFiles []*entities.File) ([]*entities.File, error)
	CompareByNameAndSize(ctx context.Context, sourceFiles, targetFiles []*entities.File) ([]*entities.File, error)

	// Configuration
	GetConfiguration() map[string]interface{}
	SetConfiguration(config map[string]interface{}) error
}

// ComparisonService defines the domain service for folder comparison operations
type ComparisonService interface {
	// Core comparison operations
	CompareFolders(ctx context.Context, sourceFolderID, targetFolderID string) (*entities.ComparisonResult, error)
	CompareWithOptions(ctx context.Context, sourceFolderID, targetFolderID string, options map[string]interface{}) (*entities.ComparisonResult, error)
	Recomparefolders(ctx context.Context, comparisonID int) (*entities.ComparisonResult, error)

	// Progress tracking
	CompareWithProgress(ctx context.Context, sourceFolderID, targetFolderID string, progressCallback func(step string, current, total int)) (*entities.ComparisonResult, error)
	GetComparisonProgress(ctx context.Context, comparisonID int) (*entities.Progress, error)

	// Result management
	SaveComparisonResult(ctx context.Context, result *entities.ComparisonResult) error
	LoadComparisonResult(ctx context.Context, comparisonID int) (*entities.ComparisonResult, error)
	DeleteComparisonResult(ctx context.Context, comparisonID int) error

	// Query operations
	GetComparisonHistory(ctx context.Context, folderID string) ([]*entities.ComparisonResult, error)
	GetRecentComparisons(ctx context.Context, limit int) ([]*entities.ComparisonResult, error)
	GetComparisonsByDuplicationLevel(ctx context.Context, minPercentage float64) ([]*entities.ComparisonResult, error)

	// Analysis operations
	AnalyzeComparisonResult(ctx context.Context, result *entities.ComparisonResult) (map[string]interface{}, error)
	GenerateDeletionPlan(ctx context.Context, comparisonID int) ([]*entities.File, error)
	CalculateSpaceSavings(ctx context.Context, comparisonID int) (int64, error)

	// Recommendation
	ShouldDeleteTargetFolder(ctx context.Context, result *entities.ComparisonResult) (bool, string)
	GetDeletionRecommendations(ctx context.Context, comparisonID int) (map[string]interface{}, error)

	// Cleanup operations
	CleanupOldComparisons(ctx context.Context, days int) (int, error)
	CleanupInvalidComparisons(ctx context.Context) (int, error)

	// Validation
	ValidateComparisonResult(ctx context.Context, result *entities.ComparisonResult) error
	VerifyFolderAccess(ctx context.Context, folderID string) error

	// Batch operations
	CompareMultipleFolders(ctx context.Context, comparisons []struct{ SourceID, TargetID string }) ([]*entities.ComparisonResult, error)

	// Export/Import
	ExportComparisonResult(ctx context.Context, comparisonID int, format string) ([]byte, error)
	ImportComparisonResult(ctx context.Context, data []byte, format string) (*entities.ComparisonResult, error)
}
