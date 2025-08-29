package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type FileRepository struct {
	db *sqlx.DB
}

func NewFileRepository(db *sqlx.DB) repositories.FileRepository {
	return &FileRepository{db: db}
}

// CreateTables creates the necessary database tables
func (r *FileRepository) CreateTables(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS files (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		size INTEGER NOT NULL,
		mime_type TEXT,
		modified_time DATETIME,
		hash TEXT,
		hash_calculated BOOLEAN DEFAULT FALSE,
		parents TEXT, -- JSON array
		path TEXT,
		web_view_link TEXT,
		last_updated DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_files_size ON files(size);
	CREATE INDEX IF NOT EXISTS idx_files_hash ON files(hash);
	CREATE INDEX IF NOT EXISTS idx_files_name ON files(name);
	CREATE INDEX IF NOT EXISTS idx_files_modified ON files(modified_time);
	CREATE INDEX IF NOT EXISTS idx_files_hash_calculated ON files(hash_calculated);
	`

	_, err := r.db.ExecContext(ctx, query)
	return err
}

func (r *FileRepository) Save(ctx context.Context, file *entities.File) error {
	query := `
	INSERT OR REPLACE INTO files (
		id, name, size, mime_type, modified_time, hash, hash_calculated, 
		parents, path, web_view_link, last_updated
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	parentsJSON := ""
	if len(file.Parents) > 0 {
		parentsJSON = `["` + strings.Join(file.Parents, `","`) + `"]`
	}

	_, err := r.db.ExecContext(ctx, query,
		file.ID, file.Name, file.Size, file.MimeType, file.ModifiedTime,
		file.Hash, file.HashCalculated, parentsJSON, file.Path,
		file.WebViewLink, file.LastUpdated,
	)

	return err
}

func (r *FileRepository) GetByID(ctx context.Context, id string) (*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE id = ?
	`

	var file entities.File
	var parentsJSON sql.NullString
	var modifiedTimeStr, lastUpdatedStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&file.ID, &file.Name, &file.Size, &file.MimeType, &modifiedTimeStr,
		&file.Hash, &file.HashCalculated, &parentsJSON, &file.Path,
		&file.WebViewLink, &lastUpdatedStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse modified time
	if modifiedTimeStr.Valid && modifiedTimeStr.String != "" {
		if modifiedTime, err := time.Parse(time.RFC3339, modifiedTimeStr.String); err == nil {
			file.ModifiedTime = modifiedTime
		} else if modifiedTime, err := time.Parse("2006-01-02 15:04:05", modifiedTimeStr.String); err == nil {
			file.ModifiedTime = modifiedTime
		}
	}

	// Parse last updated time
	if lastUpdatedStr.Valid && lastUpdatedStr.String != "" {
		if lastUpdated, err := time.Parse(time.RFC3339, lastUpdatedStr.String); err == nil {
			file.LastUpdated = lastUpdated
		} else if lastUpdated, err := time.Parse("2006-01-02 15:04:05", lastUpdatedStr.String); err == nil {
			file.LastUpdated = lastUpdated
		} else {
			file.LastUpdated = time.Now()
		}
	} else {
		file.LastUpdated = time.Now()
	}

	// Parse parents JSON
	if parentsJSON.Valid && parentsJSON.String != "" {
		parentsStr := strings.Trim(parentsJSON.String, `[]"`)
		if parentsStr != "" {
			file.Parents = strings.Split(parentsStr, `","`)
		}
	}

	return &file, nil
}

func (r *FileRepository) GetByHash(ctx context.Context, hash string) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE hash = ? AND hash_calculated = TRUE
	ORDER BY modified_time ASC
	`

	rows, err := r.db.QueryContext(ctx, query, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		var file entities.File
		var parentsJSON sql.NullString
		var modifiedTime, lastUpdated time.Time

		err := rows.Scan(
			&file.ID, &file.Name, &file.Size, &file.MimeType, &modifiedTime,
			&file.Hash, &file.HashCalculated, &parentsJSON, &file.Path,
			&file.WebViewLink, &lastUpdated,
		)
		if err != nil {
			return nil, err
		}

		file.ModifiedTime = modifiedTime
		file.LastUpdated = lastUpdated

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

func (r *FileRepository) FindBySize(ctx context.Context, size int64) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE size = ?
	ORDER BY name ASC
	`

	rows, err := r.db.QueryContext(ctx, query, size)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

func (r *FileRepository) FindFilesWithoutHash(ctx context.Context, limit int) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files 
	WHERE hash_calculated = FALSE OR hash IS NULL OR hash = ''
	ORDER BY size DESC, modified_time ASC
	LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

func (r *FileRepository) FindByParentID(ctx context.Context, parentID string) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files 
	WHERE parents LIKE ?
	ORDER BY name ASC
	`

	pattern := fmt.Sprintf("%%\"%s\"%%", parentID)
	rows, err := r.db.QueryContext(ctx, query, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

func (r *FileRepository) FindDuplicates(ctx context.Context, minSize int64) ([]*entities.File, error) {
	query := `
	SELECT f.id, f.name, f.size, f.mime_type, f.modified_time, f.hash, 
		   f.hash_calculated, f.parents, f.path, f.web_view_link, f.last_updated
	FROM files f
	INNER JOIN (
		SELECT hash, COUNT(*) as count
		FROM files 
		WHERE hash_calculated = TRUE AND hash IS NOT NULL AND hash != ''
		  AND size >= ?
		GROUP BY hash
		HAVING count > 1
	) duplicates ON f.hash = duplicates.hash
	ORDER BY f.hash, f.modified_time ASC
	`

	rows, err := r.db.QueryContext(ctx, query, minSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

func (r *FileRepository) UpdateHash(ctx context.Context, id, hash string) error {
	query := `UPDATE files SET hash = ?, hash_calculated = TRUE, last_updated = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, hash, time.Now(), id)
	return err
}

func (r *FileRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM files WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *FileRepository) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	query := fmt.Sprintf("DELETE FROM files WHERE id IN (%s)", placeholders)

	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

func (r *FileRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM files`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

func (r *FileRepository) CountByHash(ctx context.Context, hash string) (int, error) {
	query := `SELECT COUNT(*) FROM files WHERE hash = ? AND hash_calculated = TRUE`
	var count int
	err := r.db.QueryRowContext(ctx, query, hash).Scan(&count)
	return count, err
}

func (r *FileRepository) GetStatistics(ctx context.Context) (*entities.FileStatistics, error) {
	stats := &entities.FileStatistics{
		FilesByType:  make(map[string]int),
		SizesByType:  make(map[string]int64),
		FilesBySize:  make(map[string]int),
		SizesBySize:  make(map[string]int64),
		FilesByMonth: make(map[string]int),
		SizesByMonth: make(map[string]int64),
		GeneratedAt:  time.Now(),
	}

	// Get total files and size
	query := `SELECT COUNT(*), COALESCE(SUM(size), 0) FROM files`
	err := r.db.QueryRowContext(ctx, query).Scan(&stats.TotalFiles, &stats.TotalSize)
	if err != nil {
		return nil, err
	}

	// Get files by type
	query = `
	SELECT 
		CASE 
			WHEN mime_type LIKE 'image/%' THEN 'Images'
			WHEN mime_type LIKE 'video/%' THEN 'Videos'
			WHEN mime_type LIKE 'audio/%' THEN 'Audio'
			WHEN mime_type LIKE 'text/%' OR mime_type = 'application/pdf' THEN 'Documents'
			WHEN mime_type LIKE 'application/%' THEN 'Applications'
			ELSE 'Others'
		END as category,
		COUNT(*), COALESCE(SUM(size), 0)
	FROM files 
	GROUP BY category
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var category string
		var count int
		var totalSize int64
		if err := rows.Scan(&category, &count, &totalSize); err != nil {
			return nil, err
		}
		stats.FilesByType[category] = count
		stats.SizesByType[category] = totalSize
	}

	return stats, nil
}

// Helper method to scan a file row
func (r *FileRepository) scanFile(rows *sql.Rows) (*entities.File, error) {
	var file entities.File
	var parentsJSON sql.NullString
	var modifiedTimeStr, lastUpdatedStr sql.NullString

	err := rows.Scan(
		&file.ID, &file.Name, &file.Size, &file.MimeType, &modifiedTimeStr,
		&file.Hash, &file.HashCalculated, &parentsJSON, &file.Path,
		&file.WebViewLink, &lastUpdatedStr,
	)
	if err != nil {
		return nil, err
	}

	// Parse modified time
	if modifiedTimeStr.Valid && modifiedTimeStr.String != "" {
		if modifiedTime, err := time.Parse(time.RFC3339, modifiedTimeStr.String); err == nil {
			file.ModifiedTime = modifiedTime
		} else if modifiedTime, err := time.Parse("2006-01-02 15:04:05", modifiedTimeStr.String); err == nil {
			file.ModifiedTime = modifiedTime
		}
	}

	// Parse last updated time
	if lastUpdatedStr.Valid && lastUpdatedStr.String != "" {
		if lastUpdated, err := time.Parse(time.RFC3339, lastUpdatedStr.String); err == nil {
			file.LastUpdated = lastUpdated
		} else if lastUpdated, err := time.Parse("2006-01-02 15:04:05", lastUpdatedStr.String); err == nil {
			file.LastUpdated = lastUpdated
		} else {
			file.LastUpdated = time.Now()
		}
	} else {
		file.LastUpdated = time.Now()
	}

	// Parse parents JSON
	if parentsJSON.Valid && parentsJSON.String != "" {
		parentsStr := strings.Trim(parentsJSON.String, `[]"`)
		if parentsStr != "" {
			file.Parents = strings.Split(parentsStr, `","`)
		}
	}

	return &file, nil
}

// SaveBatch saves multiple files in a batch transaction
func (r *FileRepository) SaveBatch(ctx context.Context, files []*entities.File) error {
	if len(files) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
	INSERT OR REPLACE INTO files (
		id, name, size, mime_type, modified_time, hash, hash_calculated, 
		parents, path, web_view_link, last_updated
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, file := range files {
		parentsJSON := ""
		if len(file.Parents) > 0 {
			parentsJSON = `["` + strings.Join(file.Parents, `","`) + `"]`
		}

		_, err = stmt.ExecContext(ctx,
			file.ID, file.Name, file.Size, file.MimeType, file.ModifiedTime,
			file.Hash, file.HashCalculated, parentsJSON, file.Path,
			file.WebViewLink, file.LastUpdated,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAll retrieves all files from the database
func (r *FileRepository) GetAll(ctx context.Context) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files
	ORDER BY name ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// Update updates an existing file
func (r *FileRepository) Update(ctx context.Context, file *entities.File) error {
	return r.Save(ctx, file) // Save handles both insert and update
}

// GetByParent retrieves files by parent ID
func (r *FileRepository) GetByParent(ctx context.Context, parentID string) ([]*entities.File, error) {
	return r.FindByParentID(ctx, parentID)
}

// GetWithoutHash retrieves files that don't have calculated hashes
func (r *FileRepository) GetWithoutHash(ctx context.Context) ([]*entities.File, error) {
	return r.FindFilesWithoutHash(ctx, 1000) // Default limit
}

// GetByHashCalculated retrieves files based on hash calculation status
func (r *FileRepository) GetByHashCalculated(ctx context.Context, calculated bool) ([]*entities.File, error) {
	var condition string
	if calculated {
		condition = "hash_calculated = TRUE AND hash IS NOT NULL AND hash != ''"
	} else {
		condition = "hash_calculated = FALSE OR hash IS NULL OR hash = ''"
	}

	query := fmt.Sprintf(`
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE %s
	ORDER BY size DESC, name ASC
	`, condition)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// Search performs a search with filters
func (r *FileRepository) Search(ctx context.Context, query string, filters map[string]interface{}) ([]*entities.File, error) {
	// Simple implementation - can be enhanced
	searchQuery := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files 
	WHERE name LIKE ? OR path LIKE ?
	ORDER BY name ASC
	LIMIT 100
	`

	searchTerm := "%" + query + "%"
	rows, err := r.db.QueryContext(ctx, searchQuery, searchTerm, searchTerm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetByMimeType retrieves files by MIME type
func (r *FileRepository) GetByMimeType(ctx context.Context, mimeType string) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE mime_type = ?
	ORDER BY name ASC
	`

	rows, err := r.db.QueryContext(ctx, query, mimeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetByExtension retrieves files by extension
func (r *FileRepository) GetByExtension(ctx context.Context, extension string) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE name LIKE ?
	ORDER BY name ASC
	`

	pattern := "%." + extension
	rows, err := r.db.QueryContext(ctx, query, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetLargeFiles retrieves files larger than minSize
func (r *FileRepository) GetLargeFiles(ctx context.Context, minSize int64) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE size >= ?
	ORDER BY size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, minSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetByDateRange retrieves files within a date range
func (r *FileRepository) GetByDateRange(ctx context.Context, startDate, endDate string) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files 
	WHERE modified_time BETWEEN ? AND ?
	ORDER BY modified_time DESC
	`

	rows, err := r.db.QueryContext(ctx, query, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetTotalSize returns the total size of all files
func (r *FileRepository) GetTotalSize(ctx context.Context) (int64, error) {
	query := `SELECT COALESCE(SUM(size), 0) FROM files`
	var totalSize int64
	err := r.db.QueryRowContext(ctx, query).Scan(&totalSize)
	return totalSize, err
}

// GetCountByType returns file count by type
func (r *FileRepository) GetCountByType(ctx context.Context) (map[string]int, error) {
	query := `
	SELECT 
		CASE 
			WHEN mime_type LIKE 'image/%' THEN 'Images'
			WHEN mime_type LIKE 'video/%' THEN 'Videos'
			WHEN mime_type LIKE 'audio/%' THEN 'Audio'
			WHEN mime_type LIKE 'text/%' OR mime_type = 'application/pdf' THEN 'Documents'
			WHEN mime_type LIKE 'application/%' THEN 'Applications'
			ELSE 'Others'
		END as category,
		COUNT(*) as count
	FROM files 
	GROUP BY category
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var category string
		var count int
		if err := rows.Scan(&category, &count); err != nil {
			return nil, err
		}
		result[category] = count
	}

	return result, rows.Err()
}

// GetSizeByType returns total size by type
func (r *FileRepository) GetSizeByType(ctx context.Context) (map[string]int64, error) {
	query := `
	SELECT 
		CASE 
			WHEN mime_type LIKE 'image/%' THEN 'Images'
			WHEN mime_type LIKE 'video/%' THEN 'Videos'
			WHEN mime_type LIKE 'audio/%' THEN 'Audio'
			WHEN mime_type LIKE 'text/%' OR mime_type = 'application/pdf' THEN 'Documents'
			WHEN mime_type LIKE 'application/%' THEN 'Applications'
			ELSE 'Others'
		END as category,
		COALESCE(SUM(size), 0) as total_size
	FROM files 
	GROUP BY category
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int64)
	for rows.Next() {
		var category string
		var totalSize int64
		if err := rows.Scan(&category, &totalSize); err != nil {
			return nil, err
		}
		result[category] = totalSize
	}

	return result, rows.Err()
}

// DeleteOrphaned deletes orphaned file records
func (r *FileRepository) DeleteOrphaned(ctx context.Context) (int, error) {
	// This would require integration with Google Drive API to check if files still exist
	// For now, return 0 as a placeholder
	return 0, nil
}

// DeleteOlderThan deletes files older than specified days
func (r *FileRepository) DeleteOlderThan(ctx context.Context, days int) (int, error) {
	query := `DELETE FROM files WHERE last_updated < datetime('now', '-' || ? || ' days')`
	result, err := r.db.ExecContext(ctx, query, days)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	return int(rowsAffected), err
}

// GetFilesNeedingHash retrieves files that need hash calculation
func (r *FileRepository) GetFilesNeedingHash(ctx context.Context, limit int) ([]*entities.File, error) {
	return r.FindFilesWithoutHash(ctx, limit)
}

// UpdatePath updates the path of a file
func (r *FileRepository) UpdatePath(ctx context.Context, fileID, path string) error {
	query := `UPDATE files SET path = ?, last_updated = ? WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, path, time.Now(), fileID)
	return err
}

// GetFilesByPath retrieves files by path
func (r *FileRepository) GetFilesByPath(ctx context.Context, path string) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files WHERE path LIKE ?
	ORDER BY path ASC
	`

	pattern := path + "%"
	rows, err := r.db.QueryContext(ctx, query, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// GetPaginated retrieves files with pagination
func (r *FileRepository) GetPaginated(ctx context.Context, offset, limit int) ([]*entities.File, error) {
	query := `
	SELECT id, name, size, mime_type, modified_time, hash, hash_calculated, 
		   parents, path, web_view_link, last_updated
	FROM files
	ORDER BY name ASC
	LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
		file, err := r.scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, rows.Err()
}

// Exists checks if a file exists
func (r *FileRepository) Exists(ctx context.Context, fileID string) (bool, error) {
	query := `SELECT COUNT(*) FROM files WHERE id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, fileID).Scan(&count)
	return count > 0, err
}

// ExistsByHash checks if a file with the given hash exists
func (r *FileRepository) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	query := `SELECT COUNT(*) FROM files WHERE hash = ? AND hash_calculated = TRUE`
	var count int
	err := r.db.QueryRowContext(ctx, query, hash).Scan(&count)
	return count > 0, err
}
