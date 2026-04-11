package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"log"
	"time"
)

// FileOperationRepositoryImpl implements FileOperationRepository using SQLite
type FileOperationRepositoryImpl struct {
	db *sql.DB
}

// NewFileOperationRepository creates a new file operation repository
func NewFileOperationRepository(db *sql.DB) repositories.FileOperationRepository {
	return &FileOperationRepositoryImpl{
		db: db,
	}
}

// Create creates a new file operation record
func (r *FileOperationRepositoryImpl) Create(ctx context.Context, operation *entities.FileOperation) error {
	// Marshal FileIDs to JSON
	fileIDsJSON, err := operation.MarshalFileIDs()
	if err != nil {
		return fmt.Errorf("failed to marshal file IDs: %w", err)
	}

	query := `
		INSERT INTO file_operations (
			operation_type, source_folder_id, target_folder_id, file_ids,
			status, total_files, processed_files, total_bytes, processed_bytes,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		operation.OperationType,
		operation.SourceFolderID,
		operation.TargetFolderID,
		fileIDsJSON,
		operation.Status,
		operation.TotalFiles,
		operation.ProcessedFiles,
		operation.TotalBytes,
		operation.ProcessedBytes,
		operation.CreatedAt,
		operation.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create file operation: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	operation.ID = int(id)
	log.Printf("✅ 파일 작업 생성: ID=%d, 타입=%s", operation.ID, operation.OperationType)

	return nil
}

// GetByID retrieves a file operation by ID
func (r *FileOperationRepositoryImpl) GetByID(ctx context.Context, id int) (*entities.FileOperation, error) {
	query := `
		SELECT id, operation_type, source_folder_id, target_folder_id, file_ids,
			   status, total_files, processed_files, total_bytes, processed_bytes,
			   error_message, started_at, completed_at, created_at, updated_at
		FROM file_operations
		WHERE id = ?
	`

	operation := &entities.FileOperation{}
	var fileIDsJSON string
	var startedAt, completedAt sql.NullTime

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&operation.ID,
		&operation.OperationType,
		&operation.SourceFolderID,
		&operation.TargetFolderID,
		&fileIDsJSON,
		&operation.Status,
		&operation.TotalFiles,
		&operation.ProcessedFiles,
		&operation.TotalBytes,
		&operation.ProcessedBytes,
		&operation.ErrorMessage,
		&startedAt,
		&completedAt,
		&operation.CreatedAt,
		&operation.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, entities.ErrOperationNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file operation: %w", err)
	}

	// Unmarshal FileIDs from JSON
	if err := operation.UnmarshalFileIDs(fileIDsJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file IDs: %w", err)
	}

	if startedAt.Valid {
		operation.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		operation.CompletedAt = &completedAt.Time
	}

	return operation, nil
}

// Update updates a file operation
func (r *FileOperationRepositoryImpl) Update(ctx context.Context, operation *entities.FileOperation) error {
	// Marshal FileIDs to JSON
	fileIDsJSON, err := operation.MarshalFileIDs()
	if err != nil {
		return fmt.Errorf("failed to marshal file IDs: %w", err)
	}

	operation.UpdatedAt = time.Now()

	query := `
		UPDATE file_operations
		SET operation_type = ?, source_folder_id = ?, target_folder_id = ?, file_ids = ?,
			status = ?, total_files = ?, processed_files = ?, total_bytes = ?, processed_bytes = ?,
			error_message = ?, started_at = ?, completed_at = ?, updated_at = ?
		WHERE id = ?
	`

	_, err = r.db.ExecContext(ctx, query,
		operation.OperationType,
		operation.SourceFolderID,
		operation.TargetFolderID,
		fileIDsJSON,
		operation.Status,
		operation.TotalFiles,
		operation.ProcessedFiles,
		operation.TotalBytes,
		operation.ProcessedBytes,
		operation.ErrorMessage,
		operation.StartedAt,
		operation.CompletedAt,
		operation.UpdatedAt,
		operation.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update file operation: %w", err)
	}

	return nil
}

// UpdateStatus updates the status of a file operation
func (r *FileOperationRepositoryImpl) UpdateStatus(ctx context.Context, id int, status entities.OperationStatus) error {
	query := `
		UPDATE file_operations
		SET status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, status, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update operation status: %w", err)
	}

	log.Printf("🔄 파일 작업 상태 업데이트: ID=%d, 상태=%s", id, status)
	return nil
}

// UpdateProgress updates the progress of a file operation
func (r *FileOperationRepositoryImpl) UpdateProgress(ctx context.Context, id int, processedFiles int, processedBytes int64) error {
	query := `
		UPDATE file_operations
		SET processed_files = ?, processed_bytes = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, processedFiles, processedBytes, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update operation progress: %w", err)
	}

	return nil
}

// UpdateError updates the error message of a file operation
func (r *FileOperationRepositoryImpl) UpdateError(ctx context.Context, id int, errorMsg string) error {
	query := `
		UPDATE file_operations
		SET error_message = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query, errorMsg, entities.OperationStatusFailed, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update operation error: %w", err)
	}

	log.Printf("❌ 파일 작업 실패: ID=%d, 오류=%s", id, errorMsg)
	return nil
}

// Delete deletes a file operation by ID
func (r *FileOperationRepositoryImpl) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM file_operations WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete file operation: %w", err)
	}

	log.Printf("🗑️  파일 작업 삭제: ID=%d", id)
	return nil
}

// GetPendingOperations retrieves all pending operations
func (r *FileOperationRepositoryImpl) GetPendingOperations(ctx context.Context) ([]*entities.FileOperation, error) {
	return r.GetOperationsByStatus(ctx, entities.OperationStatusPending, 100)
}

// GetInProgressOperations retrieves all in-progress operations
func (r *FileOperationRepositoryImpl) GetInProgressOperations(ctx context.Context) ([]*entities.FileOperation, error) {
	return r.GetOperationsByStatus(ctx, entities.OperationStatusInProgress, 100)
}

// GetRecentOperations retrieves recent operations (last N operations)
func (r *FileOperationRepositoryImpl) GetRecentOperations(ctx context.Context, limit int) ([]*entities.FileOperation, error) {
	query := `
		SELECT id, operation_type, source_folder_id, target_folder_id, file_ids,
			   status, total_files, processed_files, total_bytes, processed_bytes,
			   error_message, started_at, completed_at, created_at, updated_at
		FROM file_operations
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent operations: %w", err)
	}
	defer rows.Close()

	return r.scanOperations(rows)
}

// GetOperationsByStatus retrieves operations by status
func (r *FileOperationRepositoryImpl) GetOperationsByStatus(ctx context.Context, status entities.OperationStatus, limit int) ([]*entities.FileOperation, error) {
	query := `
		SELECT id, operation_type, source_folder_id, target_folder_id, file_ids,
			   status, total_files, processed_files, total_bytes, processed_bytes,
			   error_message, started_at, completed_at, created_at, updated_at
		FROM file_operations
		WHERE status = ?
		ORDER BY created_at DESC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, status, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get operations by status: %w", err)
	}
	defer rows.Close()

	return r.scanOperations(rows)
}

// DeleteOldOperations deletes operations older than specified duration
func (r *FileOperationRepositoryImpl) DeleteOldOperations(ctx context.Context, olderThanDays int) error {
	query := `
		DELETE FROM file_operations
		WHERE created_at < datetime('now', '-' || ? || ' days')
		  AND status IN (?, ?, ?)
	`

	result, err := r.db.ExecContext(ctx, query,
		olderThanDays,
		entities.OperationStatusCompleted,
		entities.OperationStatusFailed,
		entities.OperationStatusCancelled,
	)
	if err != nil {
		return fmt.Errorf("failed to delete old operations: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("🗑️  오래된 파일 작업 삭제: %d개", rowsAffected)

	return nil
}

// CountByStatus counts operations by status
func (r *FileOperationRepositoryImpl) CountByStatus(ctx context.Context, status entities.OperationStatus) (int, error) {
	query := `SELECT COUNT(*) FROM file_operations WHERE status = ?`

	var count int
	err := r.db.QueryRowContext(ctx, query, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count operations by status: %w", err)
	}

	return count, nil
}

// scanOperations scans multiple operation rows
func (r *FileOperationRepositoryImpl) scanOperations(rows *sql.Rows) ([]*entities.FileOperation, error) {
	operations := make([]*entities.FileOperation, 0)

	for rows.Next() {
		operation := &entities.FileOperation{}
		var fileIDsJSON string
		var startedAt, completedAt sql.NullTime

		err := rows.Scan(
			&operation.ID,
			&operation.OperationType,
			&operation.SourceFolderID,
			&operation.TargetFolderID,
			&fileIDsJSON,
			&operation.Status,
			&operation.TotalFiles,
			&operation.ProcessedFiles,
			&operation.TotalBytes,
			&operation.ProcessedBytes,
			&operation.ErrorMessage,
			&startedAt,
			&completedAt,
			&operation.CreatedAt,
			&operation.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan operation: %w", err)
		}

		// Unmarshal FileIDs from JSON
		if err := json.Unmarshal([]byte(fileIDsJSON), &operation.FileIDs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal file IDs: %w", err)
		}

		if startedAt.Valid {
			operation.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			operation.CompletedAt = &completedAt.Time
		}

		operations = append(operations, operation)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating operations: %w", err)
	}

	return operations, nil
}
