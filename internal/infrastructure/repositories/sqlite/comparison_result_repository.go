package sqlite

import (
	"context"
	"database/sql"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type ComparisonResultRepository struct {
	db       *sqlx.DB
	fileRepo repositories.FileRepository
}

func NewComparisonResultRepository(db *sqlx.DB, fileRepo repositories.FileRepository) repositories.ComparisonRepository {
	return &ComparisonResultRepository{
		db:       db,
		fileRepo: fileRepo,
	}
}

// CreateTables creates the necessary database tables
func (r *ComparisonResultRepository) CreateTables(ctx context.Context) error {
	// First, create the base table structure
	query := `
	CREATE TABLE IF NOT EXISTS comparison_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_folder_id TEXT NOT NULL,
		target_folder_id TEXT NOT NULL,
		source_folder_name TEXT,
		target_folder_name TEXT,
		source_file_count INTEGER DEFAULT 0,
		target_file_count INTEGER DEFAULT 0,
		duplicate_count INTEGER DEFAULT 0,
		source_total_size INTEGER DEFAULT 0,
		target_total_size INTEGER DEFAULT 0,
		duplicate_size INTEGER DEFAULT 0,
		can_delete_target_folder BOOLEAN DEFAULT FALSE,
		duplication_percentage REAL DEFAULT 0.0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS comparison_duplicate_files (
		comparison_id INTEGER,
		file_id TEXT,
		PRIMARY KEY (comparison_id, file_id),
		FOREIGN KEY (comparison_id) REFERENCES comparison_results(id) ON DELETE CASCADE,
		FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_comparison_results_source_folder ON comparison_results(source_folder_id);
	CREATE INDEX IF NOT EXISTS idx_comparison_results_target_folder ON comparison_results(target_folder_id);
	CREATE INDEX IF NOT EXISTS idx_comparison_results_created_at ON comparison_results(created_at);
	CREATE INDEX IF NOT EXISTS idx_comparison_duplicate_files_comparison_id ON comparison_duplicate_files(comparison_id);
	CREATE INDEX IF NOT EXISTS idx_comparison_duplicate_files_file_id ON comparison_duplicate_files(file_id);
	`

	_, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return err
	}

	// Run migration to add missing columns for existing databases
	return r.migrateSchema(ctx)
}

// migrateSchema adds missing columns to existing tables
func (r *ComparisonResultRepository) migrateSchema(ctx context.Context) error {
	log.Printf("🔧 데이터베이스 스키마 마이그레이션 시작...")
	
	// Check if duplicate_size column exists
	checkQuery := `SELECT COUNT(*) FROM pragma_table_info('comparison_results') WHERE name='duplicate_size'`
	var count int
	err := r.db.QueryRowContext(ctx, checkQuery).Scan(&count)
	if err != nil {
		return err
	}
	
	if count == 0 {
		log.Printf("➕ duplicate_size 컬럼이 없어서 추가합니다...")
		alterQuery := `ALTER TABLE comparison_results ADD COLUMN duplicate_size INTEGER DEFAULT 0`
		_, err = r.db.ExecContext(ctx, alterQuery)
		if err != nil {
			log.Printf("❌ duplicate_size 컬럼 추가 실패: %v", err)
			return err
		}
		log.Printf("✅ duplicate_size 컬럼 추가 완료")
	} else {
		log.Printf("✅ duplicate_size 컬럼이 이미 존재합니다")
	}
	
	log.Printf("✅ 데이터베이스 스키마 마이그레이션 완료")
	return nil
}

func (r *ComparisonResultRepository) Save(ctx context.Context, result *entities.ComparisonResult) error {
	log.Printf("💾 ComparisonResultRepository.Save 시작 - ID: %d, 중복 파일 수: %d", result.ID, len(result.DuplicateFiles))
	
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		log.Printf("❌ 트랜잭션 시작 실패: %v", err)
		return err
	}
	defer tx.Rollback()

	if result.ID == 0 {
		// Insert new comparison result
		log.Printf("📝 새로운 비교 결과 삽입 중...")
		query := `
		INSERT INTO comparison_results (
			source_folder_id, target_folder_id, source_folder_name, target_folder_name,
			source_file_count, target_file_count, duplicate_count,
			source_total_size, target_total_size, duplicate_size,
			can_delete_target_folder, duplication_percentage,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`

		res, err := tx.ExecContext(ctx, query,
			result.SourceFolderID, result.TargetFolderID,
			result.SourceFolderName, result.TargetFolderName,
			result.SourceFileCount, result.TargetFileCount, result.DuplicateCount,
			result.SourceTotalSize, result.TargetTotalSize, result.DuplicateSize,
			result.CanDeleteTargetFolder, result.DuplicationPercentage,
			result.CreatedAt, result.UpdatedAt,
		)
		if err != nil {
			log.Printf("❌ 비교 결과 삽입 실패: %v", err)
			return err
		}

		id, err := res.LastInsertId()
		if err != nil {
			log.Printf("❌ 삽입된 ID 조회 실패: %v", err)
			return err
		}
		result.ID = int(id)
		log.Printf("✅ 새로운 비교 결과 삽입 완료 - ID: %d", result.ID)
	} else {
		// Update existing comparison result
		query := `
		UPDATE comparison_results SET
			source_folder_id = ?, target_folder_id = ?, source_folder_name = ?, target_folder_name = ?,
			source_file_count = ?, target_file_count = ?, duplicate_count = ?,
			source_total_size = ?, target_total_size = ?, duplicate_size = ?,
			can_delete_target_folder = ?, duplication_percentage = ?, updated_at = ?
		WHERE id = ?
		`

		_, err := tx.ExecContext(ctx, query,
			result.SourceFolderID, result.TargetFolderID,
			result.SourceFolderName, result.TargetFolderName,
			result.SourceFileCount, result.TargetFileCount, result.DuplicateCount,
			result.SourceTotalSize, result.TargetTotalSize, result.DuplicateSize,
			result.CanDeleteTargetFolder, result.DuplicationPercentage,
			result.UpdatedAt, result.ID,
		)
		if err != nil {
			return err
		}

		// Delete existing duplicate file relationships
		_, err = tx.ExecContext(ctx, "DELETE FROM comparison_duplicate_files WHERE comparison_id = ?", result.ID)
		if err != nil {
			return err
		}
	}

	// Insert duplicate file relationships
	if len(result.DuplicateFiles) > 0 {
		log.Printf("🔗 중복 파일 관계 저장 시작 - %d개 파일", len(result.DuplicateFiles))
		query := "INSERT INTO comparison_duplicate_files (comparison_id, file_id) VALUES "
		values := make([]string, len(result.DuplicateFiles))
		args := make([]interface{}, len(result.DuplicateFiles)*2)

		for i, file := range result.DuplicateFiles {
			values[i] = "(?, ?)"
			args[i*2] = result.ID
			args[i*2+1] = file.ID
		}

		query += strings.Join(values, ", ")
		log.Printf("📝 중복 파일 관계 쿼리 실행 중...")
		_, err = tx.ExecContext(ctx, query, args...)
		if err != nil {
			log.Printf("❌ 중복 파일 관계 저장 실패: %v", err)
			return err
		}
		log.Printf("✅ 중복 파일 관계 저장 완료")
	} else {
		log.Printf("ℹ️ 중복 파일이 없어서 관계 저장 건너뛰기")
	}

	log.Printf("💾 트랜잭션 커밋 중...")
	err = tx.Commit()
	if err != nil {
		log.Printf("❌ 트랜잭션 커밋 실패: %v", err)
		return err
	}
	log.Printf("✅ ComparisonResultRepository.Save 완료")
	return nil
}

func (r *ComparisonResultRepository) FindByID(ctx context.Context, id int) (*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results WHERE id = ?
	`

	var result entities.ComparisonResult
	var createdAtStr, updatedAtStr string

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&result.ID, &result.SourceFolderID, &result.TargetFolderID,
		&result.SourceFolderName, &result.TargetFolderName,
		&result.SourceFileCount, &result.TargetFileCount, &result.DuplicateCount,
		&result.SourceTotalSize, &result.TargetTotalSize, &result.DuplicateSize,
		&result.CanDeleteTargetFolder, &result.DuplicationPercentage,
		&createdAtStr, &updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse time strings to time.Time
	if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		result.CreatedAt = createdAt
	} else if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		result.CreatedAt = createdAt
	} else {
		result.CreatedAt = time.Now() // fallback
	}

	if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
		result.UpdatedAt = updatedAt
	} else if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
		result.UpdatedAt = updatedAt
	} else {
		result.UpdatedAt = time.Now() // fallback
	}

	// Load duplicate files
	files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
	if err != nil {
		return nil, err
	}
	result.DuplicateFiles = files

	return &result, nil
}

func (r *ComparisonResultRepository) FindByFolders(ctx context.Context, sourceFolderID, targetFolderID string) (*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	WHERE source_folder_id = ? AND target_folder_id = ?
	ORDER BY created_at DESC
	LIMIT 1
	`

	var result entities.ComparisonResult
	var createdAtStr, updatedAtStr string

	err := r.db.QueryRowContext(ctx, query, sourceFolderID, targetFolderID).Scan(
		&result.ID, &result.SourceFolderID, &result.TargetFolderID,
		&result.SourceFolderName, &result.TargetFolderName,
		&result.SourceFileCount, &result.TargetFileCount, &result.DuplicateCount,
		&result.SourceTotalSize, &result.TargetTotalSize, &result.DuplicateSize,
		&result.CanDeleteTargetFolder, &result.DuplicationPercentage,
		&createdAtStr, &updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse time strings to time.Time
	if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		result.CreatedAt = createdAt
	} else if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		result.CreatedAt = createdAt
	} else {
		result.CreatedAt = time.Now() // fallback
	}

	if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
		result.UpdatedAt = updatedAt
	} else if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
		result.UpdatedAt = updatedAt
	} else {
		result.UpdatedAt = time.Now() // fallback
	}

	// Load duplicate files
	files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
	if err != nil {
		return nil, err
	}
	result.DuplicateFiles = files

	return &result, nil
}

func (r *ComparisonResultRepository) FindAll(ctx context.Context, limit, offset int) ([]*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	ORDER BY created_at DESC
	LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.ComparisonResult
	for rows.Next() {
		result, err := r.scanComparisonResult(rows)
		if err != nil {
			return nil, err
		}

		// Load duplicate files
		files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
		if err != nil {
			return nil, err
		}
		result.DuplicateFiles = files

		results = append(results, result)
	}

	return results, rows.Err()
}

func (r *ComparisonResultRepository) FindBySourceFolder(ctx context.Context, sourceFolderID string) ([]*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	WHERE source_folder_id = ?
	ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, sourceFolderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.ComparisonResult
	for rows.Next() {
		result, err := r.scanComparisonResult(rows)
		if err != nil {
			return nil, err
		}

		// Load duplicate files
		files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
		if err != nil {
			return nil, err
		}
		result.DuplicateFiles = files

		results = append(results, result)
	}

	return results, rows.Err()
}

func (r *ComparisonResultRepository) FindByTargetFolder(ctx context.Context, targetFolderID string) ([]*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	WHERE target_folder_id = ?
	ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, targetFolderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.ComparisonResult
	for rows.Next() {
		result, err := r.scanComparisonResult(rows)
		if err != nil {
			return nil, err
		}

		// Load duplicate files
		files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
		if err != nil {
			return nil, err
		}
		result.DuplicateFiles = files

		results = append(results, result)
	}

	return results, rows.Err()
}

func (r *ComparisonResultRepository) Delete(ctx context.Context, id int) error {
	// SQLite will cascade delete the duplicate file relationships
	query := `DELETE FROM comparison_results WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *ComparisonResultRepository) DeleteByFolders(ctx context.Context, sourceFolderID, targetFolderID string) error {
	query := `DELETE FROM comparison_results WHERE source_folder_id = ? AND target_folder_id = ?`
	_, err := r.db.ExecContext(ctx, query, sourceFolderID, targetFolderID)
	return err
}

func (r *ComparisonResultRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM comparison_results`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

func (r *ComparisonResultRepository) GetTotalPotentialSavings(ctx context.Context) (int64, error) {
	query := `
	SELECT COALESCE(SUM(duplicate_size), 0)
	FROM comparison_results
	WHERE can_delete_target_folder = TRUE
	`
	var totalSavings int64
	err := r.db.QueryRowContext(ctx, query).Scan(&totalSavings)
	return totalSavings, err
}

// Helper methods

func (r *ComparisonResultRepository) scanComparisonResult(rows *sql.Rows) (*entities.ComparisonResult, error) {
	var result entities.ComparisonResult
	var createdAtStr, updatedAtStr string

	err := rows.Scan(
		&result.ID, &result.SourceFolderID, &result.TargetFolderID,
		&result.SourceFolderName, &result.TargetFolderName,
		&result.SourceFileCount, &result.TargetFileCount, &result.DuplicateCount,
		&result.SourceTotalSize, &result.TargetTotalSize, &result.DuplicateSize,
		&result.CanDeleteTargetFolder, &result.DuplicationPercentage,
		&createdAtStr, &updatedAtStr,
	)
	if err != nil {
		return nil, err
	}

	// Parse time strings to time.Time
	if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		result.CreatedAt = createdAt
	} else if createdAt, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
		result.CreatedAt = createdAt
	} else {
		result.CreatedAt = time.Now() // fallback
	}

	if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr); err == nil {
		result.UpdatedAt = updatedAt
	} else if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr); err == nil {
		result.UpdatedAt = updatedAt
	} else {
		result.UpdatedAt = time.Now() // fallback
	}

	return &result, nil
}

func (r *ComparisonResultRepository) loadDuplicateFilesForComparison(ctx context.Context, comparisonID int) ([]*entities.File, error) {
	query := `
	SELECT f.id, f.name, f.size, f.mime_type, f.modified_time, f.hash, 
		   f.hash_calculated, f.parents, f.path, f.web_view_link, f.last_updated
	FROM files f
	JOIN comparison_duplicate_files cdf ON f.id = cdf.file_id
	WHERE cdf.comparison_id = ?
	ORDER BY f.size DESC, f.name ASC
	`

	rows, err := r.db.QueryContext(ctx, query, comparisonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		var file entities.File
		var parentsJSON sql.NullString
		var modifiedTimeStr, lastUpdatedStr string

		err := rows.Scan(
			&file.ID, &file.Name, &file.Size, &file.MimeType, &modifiedTimeStr,
			&file.Hash, &file.HashCalculated, &parentsJSON, &file.Path,
			&file.WebViewLink, &lastUpdatedStr,
		)
		if err != nil {
			return nil, err
		}

		// Parse time strings to time.Time
		if modifiedTime, err := time.Parse("2006-01-02 15:04:05", modifiedTimeStr); err == nil {
			file.ModifiedTime = modifiedTime
		} else if modifiedTime, err := time.Parse(time.RFC3339, modifiedTimeStr); err == nil {
			file.ModifiedTime = modifiedTime
		} else {
			file.ModifiedTime = time.Now() // fallback
		}

		if lastUpdated, err := time.Parse("2006-01-02 15:04:05", lastUpdatedStr); err == nil {
			file.LastUpdated = lastUpdated
		} else if lastUpdated, err := time.Parse(time.RFC3339, lastUpdatedStr); err == nil {
			file.LastUpdated = lastUpdated
		} else {
			file.LastUpdated = time.Now() // fallback
		}

		// Parse parents JSON
		if parentsJSON.Valid && parentsJSON.String != "" {
			parentsStr := strings.Trim(parentsJSON.String, `[]"`)
			if parentsStr != "" {
				file.Parents = strings.Split(parentsStr, `","`)
			}
		}

		files = append(files, &file)
	}

	return files, rows.Err()
}

// AddDuplicateFile adds a duplicate file to a comparison result
func (r *ComparisonResultRepository) AddDuplicateFile(ctx context.Context, comparisonID int, file *entities.File) error {
	query := `
	INSERT OR IGNORE INTO comparison_duplicate_files (comparison_id, file_id)
	VALUES (?, ?)
	`
	_, err := r.db.ExecContext(ctx, query, comparisonID, file.ID)
	return err
}

// CleanupInvalidComparisons removes comparison results that reference non-existent folders
func (r *ComparisonResultRepository) CleanupInvalidComparisons(ctx context.Context) (int, error) {
	// This is a simple implementation that just counts affected rows
	// In a real implementation, you might want to verify folder existence
	query := `DELETE FROM comparison_results WHERE id IN (
		SELECT cr.id FROM comparison_results cr
		LEFT JOIN files sf ON sf.id = cr.source_folder_id
		LEFT JOIN files tf ON tf.id = cr.target_folder_id
		WHERE sf.id IS NULL OR tf.id IS NULL
	)`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	return int(rowsAffected), err
}

// DeleteBatch deletes multiple comparison results by their IDs
func (r *ComparisonResultRepository) DeleteBatch(ctx context.Context, ids []int) error {
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

	query := `DELETE FROM comparison_results WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// DeleteOlderThan deletes comparison results older than the specified number of days
func (r *ComparisonResultRepository) DeleteOlderThan(ctx context.Context, days int) (int, error) {
	query := `DELETE FROM comparison_results WHERE created_at < datetime('now', '-' || ? || ' days')`
	result, err := r.db.ExecContext(ctx, query, days)
	if err != nil {
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	return int(rowsAffected), err
}

// GetAll returns all comparison results without pagination
func (r *ComparisonResultRepository) GetAll(ctx context.Context) ([]*entities.ComparisonResult, error) {
	return r.FindAll(ctx, -1, 0) // Use existing FindAll with no limit
}

// Update updates an existing comparison result
func (r *ComparisonResultRepository) Update(ctx context.Context, result *entities.ComparisonResult) error {
	return r.Save(ctx, result) // Use existing Save method which handles updates
}

// GetByFolders returns the comparison result for the specified source and target folders
func (r *ComparisonResultRepository) GetByFolders(ctx context.Context, sourceFolderID, targetFolderID string) (*entities.ComparisonResult, error) {
	return r.FindByFolders(ctx, sourceFolderID, targetFolderID) // Use existing FindByFolders
}

// GetBySourceFolder returns all comparison results for the specified source folder
func (r *ComparisonResultRepository) GetBySourceFolder(ctx context.Context, sourceFolderID string) ([]*entities.ComparisonResult, error) {
	return r.FindBySourceFolder(ctx, sourceFolderID) // Use existing FindBySourceFolder
}

// GetByTargetFolder returns all comparison results for the specified target folder
func (r *ComparisonResultRepository) GetByTargetFolder(ctx context.Context, targetFolderID string) ([]*entities.ComparisonResult, error) {
	return r.FindByTargetFolder(ctx, targetFolderID) // Use existing FindByTargetFolder
}

// GetByID returns a comparison result by its ID
func (r *ComparisonResultRepository) GetByID(ctx context.Context, id int) (*entities.ComparisonResult, error) {
	return r.FindByID(ctx, id) // Use existing FindByID
}

// GetRecentComparisons returns the most recent comparison results
func (r *ComparisonResultRepository) GetRecentComparisons(ctx context.Context, limit int) ([]*entities.ComparisonResult, error) {
	return r.FindAll(ctx, limit, 0)
}

// RemoveDuplicateFile removes a duplicate file from a comparison result
func (r *ComparisonResultRepository) RemoveDuplicateFile(ctx context.Context, comparisonID int, fileID string) error {
	query := `DELETE FROM comparison_duplicate_files WHERE comparison_id = ? AND file_id = ?`
	_, err := r.db.ExecContext(ctx, query, comparisonID, fileID)
	return err
}

// GetDuplicateFiles returns all duplicate files for a comparison
func (r *ComparisonResultRepository) GetDuplicateFiles(ctx context.Context, comparisonID int) ([]*entities.File, error) {
	return r.loadDuplicateFilesForComparison(ctx, comparisonID)
}

// GetTotalSpaceSavings returns the total potential space savings from all comparisons
func (r *ComparisonResultRepository) GetTotalSpaceSavings(ctx context.Context) (int64, error) {
	return r.GetTotalPotentialSavings(ctx) // Use existing method
}

// GetComparisonStats returns various statistics about comparisons
func (r *ComparisonResultRepository) GetComparisonStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total comparisons
	totalComparisons, err := r.Count(ctx)
	if err != nil {
		return nil, err
	}
	stats["total_comparisons"] = totalComparisons

	// Total potential savings
	totalSavings, err := r.GetTotalSpaceSavings(ctx)
	if err != nil {
		return nil, err
	}
	stats["total_potential_savings"] = totalSavings

	// Average duplication percentage
	query := `SELECT AVG(duplication_percentage) FROM comparison_results WHERE duplication_percentage > 0`
	var avgDuplication sql.NullFloat64
	err = r.db.QueryRowContext(ctx, query).Scan(&avgDuplication)
	if err != nil {
		return nil, err
	}
	if avgDuplication.Valid {
		stats["average_duplication_percentage"] = avgDuplication.Float64
	} else {
		stats["average_duplication_percentage"] = 0.0
	}

	return stats, nil
}

// GetWithSignificantSavings returns comparisons with potential savings above the minimum threshold
func (r *ComparisonResultRepository) GetWithSignificantSavings(ctx context.Context, minSavings int64) ([]*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	WHERE duplicate_size >= ?
	ORDER BY duplicate_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, minSavings)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.ComparisonResult
	for rows.Next() {
		result, err := r.scanComparisonResult(rows)
		if err != nil {
			return nil, err
		}

		// Load duplicate files
		files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
		if err != nil {
			return nil, err
		}
		result.DuplicateFiles = files

		results = append(results, result)
	}

	return results, rows.Err()
}

// GetDeletableComparisons returns comparisons where the target folder can be deleted
func (r *ComparisonResultRepository) GetDeletableComparisons(ctx context.Context) ([]*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	WHERE can_delete_target_folder = TRUE
	ORDER BY duplicate_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.ComparisonResult
	for rows.Next() {
		result, err := r.scanComparisonResult(rows)
		if err != nil {
			return nil, err
		}

		// Load duplicate files
		files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
		if err != nil {
			return nil, err
		}
		result.DuplicateFiles = files

		results = append(results, result)
	}

	return results, rows.Err()
}

// GetByDuplicationPercentage returns comparisons with duplication percentage above the minimum threshold
func (r *ComparisonResultRepository) GetByDuplicationPercentage(ctx context.Context, minPercentage float64) ([]*entities.ComparisonResult, error) {
	query := `
	SELECT id, source_folder_id, target_folder_id, source_folder_name, target_folder_name,
		   source_file_count, target_file_count, duplicate_count,
		   source_total_size, target_total_size, duplicate_size,
		   can_delete_target_folder, duplication_percentage,
		   created_at, updated_at
	FROM comparison_results 
	WHERE duplication_percentage >= ?
	ORDER BY duplication_percentage DESC
	`

	rows, err := r.db.QueryContext(ctx, query, minPercentage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*entities.ComparisonResult
	for rows.Next() {
		result, err := r.scanComparisonResult(rows)
		if err != nil {
			return nil, err
		}

		// Load duplicate files
		files, err := r.loadDuplicateFilesForComparison(ctx, result.ID)
		if err != nil {
			return nil, err
		}
		result.DuplicateFiles = files

		results = append(results, result)
	}

	return results, rows.Err()
}

// GetPaginated returns comparison results with pagination
func (r *ComparisonResultRepository) GetPaginated(ctx context.Context, offset, limit int) ([]*entities.ComparisonResult, error) {
	return r.FindAll(ctx, limit, offset)
}

// Exists checks if a comparison result exists by ID
func (r *ComparisonResultRepository) Exists(ctx context.Context, id int) (bool, error) {
	query := `SELECT COUNT(*) FROM comparison_results WHERE id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&count)
	return count > 0, err
}

// ExistsByFolders checks if a comparison result exists for the specified folders
func (r *ComparisonResultRepository) ExistsByFolders(ctx context.Context, sourceFolderID, targetFolderID string) (bool, error) {
	query := `SELECT COUNT(*) FROM comparison_results WHERE source_folder_id = ? AND target_folder_id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, sourceFolderID, targetFolderID).Scan(&count)
	return count > 0, err
}

// SaveBatch saves multiple comparison results in a single transaction
func (r *ComparisonResultRepository) SaveBatch(ctx context.Context, results []*entities.ComparisonResult) error {
	if len(results) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, result := range results {
		err := r.Save(ctx, result)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
