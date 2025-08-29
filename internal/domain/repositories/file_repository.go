package repositories

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// FileRepository defines the interface for file data access operations
type FileRepository interface {
	// Basic CRUD operations
	Save(ctx context.Context, file *entities.File) error
	SaveBatch(ctx context.Context, files []*entities.File) error
	GetByID(ctx context.Context, fileID string) (*entities.File, error)
	GetAll(ctx context.Context) ([]*entities.File, error)
	Update(ctx context.Context, file *entities.File) error
	Delete(ctx context.Context, fileID string) error
	DeleteBatch(ctx context.Context, fileIDs []string) error

	// Query operations
	GetByHash(ctx context.Context, hash string) ([]*entities.File, error)
	GetByParent(ctx context.Context, parentID string) ([]*entities.File, error)
	GetWithoutHash(ctx context.Context) ([]*entities.File, error)
	GetByHashCalculated(ctx context.Context, calculated bool) ([]*entities.File, error)

	// Search operations
	Search(ctx context.Context, query string, filters map[string]interface{}) ([]*entities.File, error)
	GetByMimeType(ctx context.Context, mimeType string) ([]*entities.File, error)
	GetByExtension(ctx context.Context, extension string) ([]*entities.File, error)
	GetLargeFiles(ctx context.Context, minSize int64) ([]*entities.File, error)
	GetByDateRange(ctx context.Context, startDate, endDate string) ([]*entities.File, error)

	// Statistics operations
	Count(ctx context.Context) (int, error)
	GetTotalSize(ctx context.Context) (int64, error)
	GetCountByType(ctx context.Context) (map[string]int, error)
	GetSizeByType(ctx context.Context) (map[string]int64, error)

	// Cleanup operations
	DeleteOrphaned(ctx context.Context) (int, error)
	DeleteOlderThan(ctx context.Context, days int) (int, error)

	// Hash operations
	UpdateHash(ctx context.Context, fileID, hash string) error
	GetFilesNeedingHash(ctx context.Context, limit int) ([]*entities.File, error)

	// Path operations
	UpdatePath(ctx context.Context, fileID, path string) error
	GetFilesByPath(ctx context.Context, path string) ([]*entities.File, error)

	// Pagination
	GetPaginated(ctx context.Context, offset, limit int) ([]*entities.File, error)

	// Validation
	Exists(ctx context.Context, fileID string) (bool, error)
	ExistsByHash(ctx context.Context, hash string) (bool, error)
}
