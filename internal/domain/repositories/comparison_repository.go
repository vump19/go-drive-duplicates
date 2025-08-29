package repositories

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// ComparisonRepository defines the interface for folder comparison data access operations
type ComparisonRepository interface {
	// Basic CRUD operations
	Save(ctx context.Context, result *entities.ComparisonResult) error
	GetByID(ctx context.Context, id int) (*entities.ComparisonResult, error)
	GetAll(ctx context.Context) ([]*entities.ComparisonResult, error)
	Update(ctx context.Context, result *entities.ComparisonResult) error
	Delete(ctx context.Context, id int) error

	// Query operations
	GetByFolders(ctx context.Context, sourceFolderID, targetFolderID string) (*entities.ComparisonResult, error)
	GetBySourceFolder(ctx context.Context, sourceFolderID string) ([]*entities.ComparisonResult, error)
	GetByTargetFolder(ctx context.Context, targetFolderID string) ([]*entities.ComparisonResult, error)
	GetRecentComparisons(ctx context.Context, limit int) ([]*entities.ComparisonResult, error)

	// File operations within comparison
	AddDuplicateFile(ctx context.Context, comparisonID int, file *entities.File) error
	RemoveDuplicateFile(ctx context.Context, comparisonID int, fileID string) error
	GetDuplicateFiles(ctx context.Context, comparisonID int) ([]*entities.File, error)

	// Statistics operations
	Count(ctx context.Context) (int, error)
	GetTotalSpaceSavings(ctx context.Context) (int64, error)
	GetComparisonStats(ctx context.Context) (map[string]interface{}, error)

	// Filtering operations
	GetWithSignificantSavings(ctx context.Context, minSavings int64) ([]*entities.ComparisonResult, error)
	GetDeletableComparisons(ctx context.Context) ([]*entities.ComparisonResult, error)
	GetByDuplicationPercentage(ctx context.Context, minPercentage float64) ([]*entities.ComparisonResult, error)

	// Cleanup operations
	DeleteOlderThan(ctx context.Context, days int) (int, error)
	CleanupInvalidComparisons(ctx context.Context) (int, error)

	// Pagination
	GetPaginated(ctx context.Context, offset, limit int) ([]*entities.ComparisonResult, error)

	// Validation
	Exists(ctx context.Context, id int) (bool, error)
	ExistsByFolders(ctx context.Context, sourceFolderID, targetFolderID string) (bool, error)

	// Batch operations
	SaveBatch(ctx context.Context, results []*entities.ComparisonResult) error
	DeleteBatch(ctx context.Context, ids []int) error
}
