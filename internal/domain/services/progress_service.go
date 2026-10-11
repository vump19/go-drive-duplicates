package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// ProgressService defines the domain service for progress management
type ProgressService interface {
	// Operation lifecycle
	StartOperation(ctx context.Context, operationType string, totalItems int) (*entities.Progress, error)
	UpdateOperation(ctx context.Context, progressID int, processedItems int, currentStep string) error
	CompleteOperation(ctx context.Context, progressID int) error
	FailOperation(ctx context.Context, progressID int, errorMessage string) error
	CancelOperation(ctx context.Context, progressID int) error

	// Progress queries
	GetProgress(ctx context.Context, progressID int) (*entities.Progress, error)
	GetActiveOperations(ctx context.Context) ([]*entities.Progress, error)
	GetOperationsByType(ctx context.Context, operationType string) ([]*entities.Progress, error)

	// Metadata management
	SetOperationMetadata(ctx context.Context, progressID int, metadata map[string]interface{}) error
}
