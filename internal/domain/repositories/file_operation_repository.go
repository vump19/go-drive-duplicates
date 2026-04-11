package repositories

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// FileOperationRepository defines interface for file operation persistence
// 파일 작업 영속성을 위한 리포지토리 인터페이스
type FileOperationRepository interface {
	// Create creates a new file operation record
	Create(ctx context.Context, operation *entities.FileOperation) error

	// GetByID retrieves a file operation by ID
	GetByID(ctx context.Context, id int) (*entities.FileOperation, error)

	// Update updates a file operation
	Update(ctx context.Context, operation *entities.FileOperation) error

	// UpdateStatus updates the status of a file operation
	UpdateStatus(ctx context.Context, id int, status entities.OperationStatus) error

	// UpdateProgress updates the progress of a file operation
	UpdateProgress(ctx context.Context, id int, processedFiles int, processedBytes int64) error

	// UpdateError updates the error message of a file operation
	UpdateError(ctx context.Context, id int, errorMsg string) error

	// Delete deletes a file operation by ID
	Delete(ctx context.Context, id int) error

	// GetPendingOperations retrieves all pending operations
	GetPendingOperations(ctx context.Context) ([]*entities.FileOperation, error)

	// GetInProgressOperations retrieves all in-progress operations
	GetInProgressOperations(ctx context.Context) ([]*entities.FileOperation, error)

	// GetRecentOperations retrieves recent operations (last N operations)
	GetRecentOperations(ctx context.Context, limit int) ([]*entities.FileOperation, error)

	// GetOperationsByStatus retrieves operations by status
	GetOperationsByStatus(ctx context.Context, status entities.OperationStatus, limit int) ([]*entities.FileOperation, error)

	// DeleteOldOperations deletes operations older than specified duration
	DeleteOldOperations(ctx context.Context, olderThanDays int) error

	// CountByStatus counts operations by status
	CountByStatus(ctx context.Context, status entities.OperationStatus) (int, error)
}
