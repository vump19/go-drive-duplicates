package entities

import (
	"time"
)

// Progress represents the progress of a long-running operation
type Progress struct {
	ID            int    `json:"id"`
	OperationType string `json:"operationType"` // "file_scan", "duplicate_search", "folder_comparison", etc.

	// Basic progress tracking
	ProcessedItems int `json:"processedItems"`
	TotalItems     int `json:"totalItems"`

	// Status information
	Status      string `json:"status"` // "pending", "running", "completed", "failed", "paused"
	CurrentStep string `json:"currentStep"`

	// Error handling
	ErrorMessage string `json:"errorMessage,omitempty"`

	// Operation-specific data
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Timestamps
	StartTime   time.Time  `json:"startTime"`
	EndTime     *time.Time `json:"endTime,omitempty"`
	LastUpdated time.Time  `json:"lastUpdated"`
}

// ProgressStatus constants
const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusPaused    = "paused"
)

// OperationType constants
const (
	OperationFileScan         = "file_scan"
	OperationDuplicateSearch  = "duplicate_search"
	OperationFolderComparison = "folder_comparison"
	OperationHashCalculation  = "hash_calculation"
	OperationFileCleanup      = "file_cleanup"
)

// NewProgress creates a new progress tracker
func NewProgress(operationType string, totalItems int) *Progress {
	now := time.Now()
	return &Progress{
		OperationType:  operationType,
		TotalItems:     totalItems,
		Status:         StatusPending,
		ProcessedItems: 0,
		Metadata:       make(map[string]interface{}),
		StartTime:      now,
		LastUpdated:    now,
	}
}

// Start marks the progress as started
func (p *Progress) Start() {
	p.Status = StatusRunning
	p.StartTime = time.Now()
	p.LastUpdated = time.Now()
}

// UpdateProgress updates the current progress
func (p *Progress) UpdateProgress(processedItems int, currentStep string) {
	p.ProcessedItems = processedItems
	p.CurrentStep = currentStep
	p.LastUpdated = time.Now()
}

// IncrementProgress increments the processed items by 1
func (p *Progress) IncrementProgress() {
	p.ProcessedItems++
	p.LastUpdated = time.Now()
}

// SetTotal updates the total items count
func (p *Progress) SetTotal(totalItems int) {
	p.TotalItems = totalItems
	p.LastUpdated = time.Now()
}

// Complete marks the progress as completed
func (p *Progress) Complete() {
	p.Status = StatusCompleted
	now := time.Now()
	p.EndTime = &now
	p.LastUpdated = now
	p.ProcessedItems = p.TotalItems // Ensure 100% completion
}

// Fail marks the progress as failed with an error message
func (p *Progress) Fail(errorMessage string) {
	p.Status = StatusFailed
	p.ErrorMessage = errorMessage
	now := time.Now()
	p.EndTime = &now
	p.LastUpdated = now
}

// Pause marks the progress as paused
func (p *Progress) Pause() {
	p.Status = StatusPaused
	p.LastUpdated = time.Now()
}

// Resume resumes a paused progress
func (p *Progress) Resume() {
	if p.Status == StatusPaused {
		p.Status = StatusRunning
		p.LastUpdated = time.Now()
	}
}

// GetPercentage returns the completion percentage (0-100)
func (p *Progress) GetPercentage() float64 {
	if p.TotalItems == 0 {
		return 0
	}
	return float64(p.ProcessedItems) / float64(p.TotalItems) * 100
}

// IsCompleted returns true if the operation is completed
func (p *Progress) IsCompleted() bool {
	return p.Status == StatusCompleted
}

// IsFailed returns true if the operation failed
func (p *Progress) IsFailed() bool {
	return p.Status == StatusFailed
}

// IsRunning returns true if the operation is currently running
func (p *Progress) IsRunning() bool {
	return p.Status == StatusRunning
}

// IsPaused returns true if the operation is paused
func (p *Progress) IsPaused() bool {
	return p.Status == StatusPaused
}

// GetDuration returns the duration of the operation
func (p *Progress) GetDuration() time.Duration {
	if p.EndTime != nil {
		return p.EndTime.Sub(p.StartTime)
	}
	return time.Since(p.StartTime)
}

// GetETA returns estimated time of arrival (completion)
func (p *Progress) GetETA() *time.Time {
	if p.TotalItems == 0 || p.ProcessedItems == 0 || p.IsCompleted() {
		return nil
	}

	elapsed := time.Since(p.StartTime)
	rate := float64(p.ProcessedItems) / elapsed.Seconds()
	remaining := p.TotalItems - p.ProcessedItems

	if rate > 0 {
		eta := time.Now().Add(time.Duration(float64(remaining)/rate) * time.Second)
		return &eta
	}

	return nil
}

// SetMetadata sets a metadata value
func (p *Progress) SetMetadata(key string, value interface{}) {
	if p.Metadata == nil {
		p.Metadata = make(map[string]interface{})
	}
	p.Metadata[key] = value
	p.LastUpdated = time.Now()
}

// GetMetadata gets a metadata value
func (p *Progress) GetMetadata(key string) (interface{}, bool) {
	if p.Metadata == nil {
		return nil, false
	}
	value, exists := p.Metadata[key]
	return value, exists
}
