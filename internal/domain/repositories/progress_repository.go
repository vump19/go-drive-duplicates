package repositories

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// ProgressRepository defines the interface for progress tracking data access operations
type ProgressRepository interface {
	// Basic CRUD operations
	Save(ctx context.Context, progress *entities.Progress) error
	GetByID(ctx context.Context, id int) (*entities.Progress, error)
	GetAll(ctx context.Context) ([]*entities.Progress, error)
	Update(ctx context.Context, progress *entities.Progress) error
	Delete(ctx context.Context, id int) error

	// Query operations by type
	GetByOperationType(ctx context.Context, operationType string) ([]*entities.Progress, error)
	GetActiveOperations(ctx context.Context) ([]*entities.Progress, error) // Running or paused
	GetCompletedOperations(ctx context.Context) ([]*entities.Progress, error)
	GetFailedOperations(ctx context.Context) ([]*entities.Progress, error)

	// Status operations
	GetByStatus(ctx context.Context, status string) ([]*entities.Progress, error)
	GetRunningOperations(ctx context.Context) ([]*entities.Progress, error)
	GetLatestByType(ctx context.Context, operationType string) (*entities.Progress, error)

	// Cleanup operations
	DeleteCompleted(ctx context.Context) (int, error)
	DeleteOlderThan(ctx context.Context, days int) (int, error)
	DeleteByStatus(ctx context.Context, status string) (int, error)

	// Statistics operations
	Count(ctx context.Context) (int, error)
	CountByStatus(ctx context.Context) (map[string]int, error)
	CountByType(ctx context.Context) (map[string]int, error)
	GetAverageCompletionTime(ctx context.Context, operationType string) (float64, error)

	// Monitoring operations
	GetStuckOperations(ctx context.Context, timeoutMinutes int) ([]*entities.Progress, error)
	GetLongRunningOperations(ctx context.Context, durationMinutes int) ([]*entities.Progress, error)

	// Batch operations
	SaveBatch(ctx context.Context, progresses []*entities.Progress) error
	UpdateBatch(ctx context.Context, progresses []*entities.Progress) error
	DeleteBatch(ctx context.Context, ids []int) error

	// Pagination
	GetPaginated(ctx context.Context, offset, limit int) ([]*entities.Progress, error)
	GetRecentOperations(ctx context.Context, limit int) ([]*entities.Progress, error)

	// Validation
	Exists(ctx context.Context, id int) (bool, error)
	IsOperationRunning(ctx context.Context, operationType string) (bool, error)
}
