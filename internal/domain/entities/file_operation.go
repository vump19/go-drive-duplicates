package entities

import (
	"encoding/json"
	"time"
)

// OperationType defines types of file operations
type OperationType string

const (
	OperationTypeCopy   OperationType = "copy"   // 파일 복사
	OperationTypeMove   OperationType = "move"   // 파일 이동
	OperationTypeDelete OperationType = "delete" // 파일 삭제
	OperationTypeRename OperationType = "rename" // 파일 이름 변경
)

// OperationStatus defines status of file operations
type OperationStatus string

const (
	OperationStatusPending    OperationStatus = "pending"     // 대기 중
	OperationStatusInProgress OperationStatus = "in_progress" // 진행 중
	OperationStatusCompleted  OperationStatus = "completed"   // 완료됨
	OperationStatusFailed     OperationStatus = "failed"      // 실패함
	OperationStatusCancelled  OperationStatus = "cancelled"   // 취소됨
)

// FileOperation represents a file operation (copy, move, delete, etc.)
// 파일 작업을 나타냄 (복사, 이동, 삭제 등)
type FileOperation struct {
	ID             int              `json:"id" db:"id"`
	OperationType  OperationType    `json:"operationType" db:"operation_type"`
	SourceFolderID string           `json:"sourceFolderId" db:"source_folder_id"`
	TargetFolderID string           `json:"targetFolderId" db:"target_folder_id"`
	FileIDs        []string         `json:"fileIds" db:"file_ids"` // JSON array in DB
	Status         OperationStatus  `json:"status" db:"status"`
	TotalFiles     int              `json:"totalFiles" db:"total_files"`
	ProcessedFiles int              `json:"processedFiles" db:"processed_files"`
	TotalBytes     int64            `json:"totalBytes" db:"total_bytes"`
	ProcessedBytes int64            `json:"processedBytes" db:"processed_bytes"`
	ErrorMessage   string           `json:"errorMessage,omitempty" db:"error_message"`
	ErrorDetails   []OperationError `json:"errorDetails,omitempty" db:"-"` // Not stored in DB
	StartedAt      *time.Time       `json:"startedAt,omitempty" db:"started_at"`
	CompletedAt    *time.Time       `json:"completedAt,omitempty" db:"completed_at"`
	CreatedAt      time.Time        `json:"createdAt" db:"created_at"`
	UpdatedAt      time.Time        `json:"updatedAt" db:"updated_at"`
}

// OperationError represents an error that occurred during operation
type OperationError struct {
	FileID       string    `json:"fileId"`
	FileName     string    `json:"fileName"`
	ErrorMessage string    `json:"errorMessage"`
	Timestamp    time.Time `json:"timestamp"`
}

// NewFileOperation creates a new file operation
func NewFileOperation(opType OperationType, sourceFolderID, targetFolderID string, fileIDs []string) *FileOperation {
	now := time.Now()
	return &FileOperation{
		OperationType:  opType,
		SourceFolderID: sourceFolderID,
		TargetFolderID: targetFolderID,
		FileIDs:        fileIDs,
		Status:         OperationStatusPending,
		TotalFiles:     len(fileIDs),
		ProcessedFiles: 0,
		TotalBytes:     0,
		ProcessedBytes: 0,
		ErrorDetails:   make([]OperationError, 0),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// Start marks the operation as started
func (op *FileOperation) Start() {
	now := time.Now()
	op.Status = OperationStatusInProgress
	op.StartedAt = &now
	op.UpdatedAt = now
}

// Complete marks the operation as completed
func (op *FileOperation) Complete() {
	now := time.Now()
	op.Status = OperationStatusCompleted
	op.CompletedAt = &now
	op.UpdatedAt = now
}

// Fail marks the operation as failed with error message
func (op *FileOperation) Fail(errorMsg string) {
	now := time.Now()
	op.Status = OperationStatusFailed
	op.ErrorMessage = errorMsg
	op.CompletedAt = &now
	op.UpdatedAt = now
}

// Cancel marks the operation as cancelled
func (op *FileOperation) Cancel() {
	now := time.Now()
	op.Status = OperationStatusCancelled
	op.CompletedAt = &now
	op.UpdatedAt = now
}

// UpdateProgress updates the progress of the operation
func (op *FileOperation) UpdateProgress(processedFiles int, processedBytes int64) {
	op.ProcessedFiles = processedFiles
	op.ProcessedBytes = processedBytes
	op.UpdatedAt = time.Now()
}

// AddError adds an error that occurred during operation
func (op *FileOperation) AddError(fileID, fileName, errorMsg string) {
	op.ErrorDetails = append(op.ErrorDetails, OperationError{
		FileID:       fileID,
		FileName:     fileName,
		ErrorMessage: errorMsg,
		Timestamp:    time.Now(),
	})
}

// GetProgress returns the progress percentage (0-100)
func (op *FileOperation) GetProgress() float64 {
	if op.TotalFiles == 0 {
		return 0
	}
	return float64(op.ProcessedFiles) / float64(op.TotalFiles) * 100
}

// GetBytesProgress returns the bytes progress percentage (0-100)
func (op *FileOperation) GetBytesProgress() float64 {
	if op.TotalBytes == 0 {
		return 0
	}
	return float64(op.ProcessedBytes) / float64(op.TotalBytes) * 100
}

// GetDuration returns the duration of the operation
func (op *FileOperation) GetDuration() time.Duration {
	if op.StartedAt == nil {
		return 0
	}

	endTime := time.Now()
	if op.CompletedAt != nil {
		endTime = *op.CompletedAt
	}

	return endTime.Sub(*op.StartedAt)
}

// IsRunning returns true if the operation is currently running
func (op *FileOperation) IsRunning() bool {
	return op.Status == OperationStatusInProgress
}

// IsCompleted returns true if the operation is completed (success or failure)
func (op *FileOperation) IsCompleted() bool {
	return op.Status == OperationStatusCompleted ||
		op.Status == OperationStatusFailed ||
		op.Status == OperationStatusCancelled
}

// HasErrors returns true if there are any errors
func (op *FileOperation) HasErrors() bool {
	return len(op.ErrorDetails) > 0 || op.ErrorMessage != ""
}

// MarshalFileIDs converts FileIDs slice to JSON string for database storage
func (op *FileOperation) MarshalFileIDs() (string, error) {
	data, err := json.Marshal(op.FileIDs)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalFileIDs converts JSON string from database to FileIDs slice
func (op *FileOperation) UnmarshalFileIDs(data string) error {
	return json.Unmarshal([]byte(data), &op.FileIDs)
}

// FileOperationRequest represents a request to perform file operation
type FileOperationRequest struct {
	OperationType  OperationType `json:"operationType"`
	SourceFolderID string        `json:"sourceFolderId"`
	TargetFolderID string        `json:"targetFolderId"`
	FileIDs        []string      `json:"fileIds"`
}

// Validate validates the operation request
func (req *FileOperationRequest) Validate() error {
	if req.OperationType == "" {
		return ErrInvalidOperationType
	}

	if req.OperationType != OperationTypeDelete && req.TargetFolderID == "" {
		return ErrTargetFolderRequired
	}

	if len(req.FileIDs) == 0 {
		return ErrNoFilesSelected
	}

	return nil
}

// Custom errors
var (
	ErrInvalidOperationType = NewValidationError("operationType", "invalid operation type")
	ErrTargetFolderRequired = NewValidationError("targetFolderId", "target folder is required for copy/move operations")
	ErrNoFilesSelected      = NewValidationError("fileIds", "no files selected")
	ErrOperationNotFound    = NewNotFoundError("operation", "operation not found")
	ErrOperationNotRunning  = NewValidationError("status", "operation is not running")
	ErrOperationAlreadyDone = NewValidationError("status", "operation already completed")
)
