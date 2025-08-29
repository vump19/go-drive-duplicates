package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"time"

	"github.com/jmoiron/sqlx"
)

type ProgressRepository struct {
	db *sqlx.DB
}

func NewProgressRepository(db *sqlx.DB) repositories.ProgressRepository {
	return &ProgressRepository{db: db}
}

// CreateTables creates the necessary database tables
func (r *ProgressRepository) CreateTables(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS progress (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		operation_type TEXT NOT NULL,
		processed_items INTEGER DEFAULT 0,
		total_items INTEGER DEFAULT 0,
		status TEXT DEFAULT 'pending',
		current_step TEXT,
		error_message TEXT,
		start_time DATETIME DEFAULT CURRENT_TIMESTAMP,
		end_time DATETIME,
		last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
		metadata TEXT -- JSON
	);

	CREATE INDEX IF NOT EXISTS idx_progress_operation_type ON progress(operation_type);
	CREATE INDEX IF NOT EXISTS idx_progress_status ON progress(status);
	CREATE INDEX IF NOT EXISTS idx_progress_start_time ON progress(start_time);
	`

	_, err := r.db.ExecContext(ctx, query)
	return err
}

func (r *ProgressRepository) Save(ctx context.Context, progress *entities.Progress) error {
	metadataJSON, err := json.Marshal(progress.Metadata)
	if err != nil {
		return err
	}

	if progress.ID == 0 {
		// Insert new progress
		query := `
		INSERT INTO progress (
			operation_type, processed_items, total_items, status, current_step,
			error_message, start_time, end_time, last_updated, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		result, err := r.db.ExecContext(ctx, query,
			progress.OperationType, progress.ProcessedItems, progress.TotalItems,
			progress.Status, progress.CurrentStep, progress.ErrorMessage,
			progress.StartTime, progress.EndTime, progress.LastUpdated, string(metadataJSON),
		)
		if err != nil {
			return err
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		progress.ID = int(id)
	} else {
		// Update existing progress
		query := `
		UPDATE progress SET
			operation_type = ?, processed_items = ?, total_items = ?, status = ?,
			current_step = ?, error_message = ?, start_time = ?, end_time = ?,
			last_updated = ?, metadata = ?
		WHERE id = ?
		`

		_, err := r.db.ExecContext(ctx, query,
			progress.OperationType, progress.ProcessedItems, progress.TotalItems,
			progress.Status, progress.CurrentStep, progress.ErrorMessage,
			progress.StartTime, progress.EndTime, progress.LastUpdated, string(metadataJSON),
			progress.ID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *ProgressRepository) GetByID(ctx context.Context, id int) (*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress WHERE id = ?
	`

	var progress entities.Progress
	var endTime sql.NullTime
	var currentStep sql.NullString
	var errorMessage sql.NullString
	var startTime, lastUpdated time.Time
	var metadataJSON sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&progress.ID, &progress.OperationType, &progress.ProcessedItems,
		&progress.TotalItems, &progress.Status, &currentStep,
		&errorMessage, &startTime, &endTime, &lastUpdated, &metadataJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	progress.StartTime = startTime
	progress.LastUpdated = lastUpdated
	if endTime.Valid {
		progress.EndTime = &endTime.Time
	}
	if currentStep.Valid {
		progress.CurrentStep = currentStep.String
	}
	if errorMessage.Valid {
		progress.ErrorMessage = errorMessage.String
	}

	// Parse metadata JSON
	if metadataJSON.Valid && metadataJSON.String != "" {
		err = json.Unmarshal([]byte(metadataJSON.String), &progress.Metadata)
		if err != nil {
			return nil, err
		}
	}

	return &progress, nil
}

func (r *ProgressRepository) GetByOperationType(ctx context.Context, operationType string) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	WHERE operation_type = ? 
	ORDER BY start_time DESC
	`

	rows, err := r.db.QueryContext(ctx, query, operationType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (r *ProgressRepository) GetActiveOperations(ctx context.Context) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	WHERE status IN ('pending', 'running', 'in_progress')
	ORDER BY start_time DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

// Helper method to scan a progress row
func (r *ProgressRepository) scanProgress(rows *sql.Rows) (*entities.Progress, error) {
	var progress entities.Progress
	var endTime sql.NullTime
	var currentStep sql.NullString
	var errorMessage sql.NullString
	var startTime, lastUpdated time.Time
	var metadataJSON sql.NullString

	err := rows.Scan(
		&progress.ID, &progress.OperationType, &progress.ProcessedItems,
		&progress.TotalItems, &progress.Status, &currentStep,
		&errorMessage, &startTime, &endTime, &lastUpdated, &metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	progress.StartTime = startTime
	progress.LastUpdated = lastUpdated
	if endTime.Valid {
		progress.EndTime = &endTime.Time
	}
	if currentStep.Valid {
		progress.CurrentStep = currentStep.String
	}
	if errorMessage.Valid {
		progress.ErrorMessage = errorMessage.String
	}

	// Parse metadata JSON
	if metadataJSON.Valid && metadataJSON.String != "" {
		err = json.Unmarshal([]byte(metadataJSON.String), &progress.Metadata)
		if err != nil {
			return nil, err
		}
		if progress.Metadata == nil {
			progress.Metadata = make(map[string]interface{})
		}
	} else {
		progress.Metadata = make(map[string]interface{})
	}

	return &progress, nil
}

// Other required methods with minimal implementations
func (r *ProgressRepository) Update(ctx context.Context, progress *entities.Progress) error {
	return r.Save(ctx, progress)
}

func (r *ProgressRepository) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM progress WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *ProgressRepository) GetRecentOperations(ctx context.Context, limit int) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	ORDER BY start_time DESC
	LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (r *ProgressRepository) GetStuckOperations(ctx context.Context, timeoutMinutes int) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	WHERE status IN ('pending', 'running', 'in_progress')
	AND datetime(last_updated, '+' || ? || ' minutes') < datetime('now')
	ORDER BY start_time DESC
	`

	rows, err := r.db.QueryContext(ctx, query, timeoutMinutes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (r *ProgressRepository) GetLongRunningOperations(ctx context.Context, minutes int) ([]*entities.Progress, error) {
	return r.GetStuckOperations(ctx, minutes)
}

func (r *ProgressRepository) DeleteCompleted(ctx context.Context) (int, error) {
	query := `DELETE FROM progress WHERE status = 'completed'`
	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

func (r *ProgressRepository) DeleteByStatus(ctx context.Context, status string) (int, error) {
	query := `DELETE FROM progress WHERE status = ?`
	result, err := r.db.ExecContext(ctx, query, status)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

func (r *ProgressRepository) DeleteOlderThan(ctx context.Context, days int) (int, error) {
	query := `DELETE FROM progress WHERE datetime(start_time, '+' || ? || ' days') < datetime('now')`
	result, err := r.db.ExecContext(ctx, query, days)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

func (r *ProgressRepository) GetAverageCompletionTime(ctx context.Context, operationType string) (float64, error) {
	query := `
	SELECT AVG(julianday(end_time) - julianday(start_time)) * 86400 as avg_seconds
	FROM progress 
	WHERE operation_type = ? AND status = 'completed' AND end_time IS NOT NULL
	`
	var avgSeconds sql.NullFloat64
	err := r.db.QueryRowContext(ctx, query, operationType).Scan(&avgSeconds)
	if err != nil {
		return 0, err
	}
	if avgSeconds.Valid {
		return avgSeconds.Float64, nil
	}
	return 0, nil
}

func (r *ProgressRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM progress`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

func (r *ProgressRepository) CountByStatus(ctx context.Context) (map[string]int, error) {
	query := `SELECT status, COUNT(*) FROM progress GROUP BY status`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		result[status] = count
	}

	return result, rows.Err()
}

func (r *ProgressRepository) UpdateMetadata(ctx context.Context, id int, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}

	query := `
	UPDATE progress SET 
		metadata = ?, 
		last_updated = ?
	WHERE id = ?
	`
	_, err = r.db.ExecContext(ctx, query, string(metadataJSON), time.Now(), id)
	return err
}

func (r *ProgressRepository) CountByType(ctx context.Context) (map[string]int, error) {
	query := `SELECT operation_type, COUNT(*) FROM progress GROUP BY operation_type`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var operationType string
		var count int
		if err := rows.Scan(&operationType, &count); err != nil {
			return nil, err
		}
		result[operationType] = count
	}

	return result, rows.Err()
}

func (r *ProgressRepository) DeleteBatch(ctx context.Context, ids []int) error {
	if len(ids) == 0 {
		return nil
	}

	// Create placeholders for the IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `DELETE FROM progress WHERE id IN (` + 
		placeholders[0]
	for i := 1; i < len(placeholders); i++ {
		query += "," + placeholders[i]
	}
	query += `)`

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *ProgressRepository) Exists(ctx context.Context, id int) (bool, error) {
	query := `SELECT 1 FROM progress WHERE id = ? LIMIT 1`
	var exists int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *ProgressRepository) GetAll(ctx context.Context) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	ORDER BY start_time DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (r *ProgressRepository) GetByStatus(ctx context.Context, status string) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	WHERE status = ?
	ORDER BY start_time DESC
	`

	rows, err := r.db.QueryContext(ctx, query, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (r *ProgressRepository) GetCompletedOperations(ctx context.Context) ([]*entities.Progress, error) {
	return r.GetByStatus(ctx, "completed")
}

func (r *ProgressRepository) GetFailedOperations(ctx context.Context) ([]*entities.Progress, error) {
	return r.GetByStatus(ctx, "failed")
}

func (r *ProgressRepository) GetRunningOperations(ctx context.Context) ([]*entities.Progress, error) {
	return r.GetByStatus(ctx, "running")
}

func (r *ProgressRepository) GetLatestByType(ctx context.Context, operationType string) (*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	WHERE operation_type = ?
	ORDER BY start_time DESC
	LIMIT 1
	`

	var progress entities.Progress
	var endTime sql.NullTime
	var currentStep sql.NullString
	var errorMessage sql.NullString
	var startTime, lastUpdated time.Time
	var metadataJSON sql.NullString

	err := r.db.QueryRowContext(ctx, query, operationType).Scan(
		&progress.ID, &progress.OperationType, &progress.ProcessedItems,
		&progress.TotalItems, &progress.Status, &currentStep,
		&errorMessage, &startTime, &endTime, &lastUpdated, &metadataJSON,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	progress.StartTime = startTime
	progress.LastUpdated = lastUpdated
	if endTime.Valid {
		progress.EndTime = &endTime.Time
	}
	if currentStep.Valid {
		progress.CurrentStep = currentStep.String
	}
	if errorMessage.Valid {
		progress.ErrorMessage = errorMessage.String
	}

	// Parse metadata JSON
	if metadataJSON.Valid && metadataJSON.String != "" {
		err = json.Unmarshal([]byte(metadataJSON.String), &progress.Metadata)
		if err != nil {
			return nil, err
		}
	}

	return &progress, nil
}

func (r *ProgressRepository) SaveBatch(ctx context.Context, progresses []*entities.Progress) error {
	for _, progress := range progresses {
		if err := r.Save(ctx, progress); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProgressRepository) UpdateBatch(ctx context.Context, progresses []*entities.Progress) error {
	for _, progress := range progresses {
		if err := r.Update(ctx, progress); err != nil {
			return err
		}
	}
	return nil
}

func (r *ProgressRepository) GetPaginated(ctx context.Context, offset, limit int) ([]*entities.Progress, error) {
	query := `
	SELECT id, operation_type, processed_items, total_items, status, current_step,
		   error_message, start_time, end_time, last_updated, metadata
	FROM progress 
	ORDER BY start_time DESC
	LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progressList []*entities.Progress
	for rows.Next() {
		progress, err := r.scanProgress(rows)
		if err != nil {
			return nil, err
		}
		progressList = append(progressList, progress)
	}

	return progressList, rows.Err()
}

func (r *ProgressRepository) IsOperationRunning(ctx context.Context, operationType string) (bool, error) {
	query := `SELECT 1 FROM progress WHERE operation_type = ? AND status IN ('pending', 'running', 'in_progress') LIMIT 1`
	var exists int
	err := r.db.QueryRowContext(ctx, query, operationType).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}