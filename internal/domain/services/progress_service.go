package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
	"time"
)

// ProgressTracker defines the interface for progress tracking strategies
type ProgressTracker interface {
	// Basic tracking
	StartOperation(operationType string, totalItems int) (*entities.Progress, error)
	UpdateProgress(progressID int, processedItems int, currentStep string) error
	CompleteOperation(progressID int) error
	FailOperation(progressID int, errorMessage string) error

	// Real-time updates
	GetCurrentProgress(progressID int) (*entities.Progress, error)
	GetProgressPercentage(progressID int) (float64, error)
	GetETA(progressID int) (*time.Time, error)

	// Status management
	PauseOperation(progressID int) error
	ResumeOperation(progressID int) error
	CancelOperation(progressID int) error

	// Metadata management
	SetMetadata(progressID int, key string, value interface{}) error
	GetMetadata(progressID int, key string) (interface{}, error)
}

// ProgressNotifier defines the interface for progress notifications
type ProgressNotifier interface {
	// Notification methods
	NotifyProgressUpdate(progress *entities.Progress) error
	NotifyOperationCompleted(progress *entities.Progress) error
	NotifyOperationFailed(progress *entities.Progress) error

	// Subscription management
	Subscribe(operationType string, callback func(*entities.Progress)) error
	Unsubscribe(operationType string) error

	// Batch notifications
	NotifyBatch(progresses []*entities.Progress) error

	// Configuration
	SetNotificationThreshold(percentage float64) // Only notify when progress changes by this much
	SetMaxNotificationRate(maxPerSecond int)     // Rate limiting
}

// ProgressService defines the domain service for progress management
type ProgressService interface {
	// Operation lifecycle
	StartOperation(ctx context.Context, operationType string, totalItems int) (*entities.Progress, error)
	UpdateOperation(ctx context.Context, progressID int, processedItems int, currentStep string) error
	CompleteOperation(ctx context.Context, progressID int) error
	FailOperation(ctx context.Context, progressID int, errorMessage string) error

	// Progress queries
	GetProgress(ctx context.Context, progressID int) (*entities.Progress, error)
	GetActiveOperations(ctx context.Context) ([]*entities.Progress, error)
	GetOperationsByType(ctx context.Context, operationType string) ([]*entities.Progress, error)
	GetRecentOperations(ctx context.Context, limit int) ([]*entities.Progress, error)

	// Status management
	PauseOperation(ctx context.Context, progressID int) error
	ResumeOperation(ctx context.Context, progressID int) error
	CancelOperation(ctx context.Context, progressID int) error

	// Monitoring
	GetStuckOperations(ctx context.Context, timeoutMinutes int) ([]*entities.Progress, error)
	GetLongRunningOperations(ctx context.Context, durationMinutes int) ([]*entities.Progress, error)
	MonitorOperations(ctx context.Context) error

	// Statistics
	GetOperationStatistics(ctx context.Context) (map[string]interface{}, error)
	GetAverageCompletionTime(ctx context.Context, operationType string) (time.Duration, error)
	GetSuccessRate(ctx context.Context, operationType string, days int) (float64, error)

	// Cleanup
	CleanupCompletedOperations(ctx context.Context, olderThan time.Duration) (int, error)
	CleanupFailedOperations(ctx context.Context, olderThan time.Duration) (int, error)
	CleanupAllOldOperations(ctx context.Context, olderThan time.Duration) (int, error)

	// Real-time updates
	StreamProgress(ctx context.Context, progressID int) (<-chan *entities.Progress, error)
	StreamOperationsByType(ctx context.Context, operationType string) (<-chan *entities.Progress, error)
	BroadcastProgress(ctx context.Context, progress *entities.Progress) error

	// Notifications
	RegisterProgressCallback(operationType string, callback func(*entities.Progress)) error
	UnregisterProgressCallback(operationType string) error
	NotifyProgressUpdate(ctx context.Context, progress *entities.Progress) error

	// Metadata management
	SetOperationMetadata(ctx context.Context, progressID int, metadata map[string]interface{}) error
	GetOperationMetadata(ctx context.Context, progressID int) (map[string]interface{}, error)
	AddOperationLog(ctx context.Context, progressID int, message string) error
	GetOperationLogs(ctx context.Context, progressID int) ([]string, error)

	// Performance analytics
	AnalyzeOperationPerformance(ctx context.Context, operationType string, days int) (map[string]interface{}, error)
	GetBottlenecks(ctx context.Context, operationType string) ([]string, error)
	OptimizeOperationParameters(ctx context.Context, operationType string) (map[string]interface{}, error)

	// Health checks
	CheckProgressHealth(ctx context.Context) (map[string]interface{}, error)
	ValidateProgress(ctx context.Context, progressID int) error
	RecoverCorruptedProgress(ctx context.Context, progressID int) error
}
