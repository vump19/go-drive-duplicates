package services

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// NotificationService defines the interface for sending notifications
type NotificationService interface {
	// NotifyOperationCompleted sends a notification when an operation completes successfully
	NotifyOperationCompleted(ctx context.Context, progress *entities.Progress) error

	// NotifyOperationFailed sends a notification when an operation fails
	NotifyOperationFailed(ctx context.Context, progress *entities.Progress) error

	// SendTestMessage sends a test notification to verify configuration
	SendTestMessage(ctx context.Context) error

	// IsEnabled returns whether notifications are currently enabled
	IsEnabled() bool

	// SetEnabled enables or disables notifications at runtime
	SetEnabled(enabled bool)

	// UpdateConfig updates the notification service configuration at runtime
	UpdateConfig(botToken, chatID string)
}
