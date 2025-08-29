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

type DuplicateGroupRepository struct {
	db       *sqlx.DB
	fileRepo repositories.FileRepository
}

func NewDuplicateGroupRepository(db *sqlx.DB, fileRepo repositories.FileRepository) repositories.DuplicateRepository {
	return &DuplicateGroupRepository{
		db:       db,
		fileRepo: fileRepo,
	}
}

// CreateTables creates the necessary database tables
func (r *DuplicateGroupRepository) CreateTables(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS duplicate_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		hash TEXT UNIQUE NOT NULL,
		count INTEGER NOT NULL,
		total_size INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS duplicate_group_files (
		group_id INTEGER,
		file_id TEXT,
		PRIMARY KEY (group_id, file_id),
		FOREIGN KEY (group_id) REFERENCES duplicate_groups(id) ON DELETE CASCADE,
		FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_duplicate_groups_hash ON duplicate_groups(hash);
	CREATE INDEX IF NOT EXISTS idx_duplicate_groups_total_size ON duplicate_groups(total_size);
	CREATE INDEX IF NOT EXISTS idx_duplicate_group_files_group_id ON duplicate_group_files(group_id);
	CREATE INDEX IF NOT EXISTS idx_duplicate_group_files_file_id ON duplicate_group_files(file_id);
	`

	_, err := r.db.ExecContext(ctx, query)
	return err
}

func (r *DuplicateGroupRepository) Save(ctx context.Context, group *entities.DuplicateGroup) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert or update duplicate group
	if group.ID == 0 {
		// Insert new group
		query := `
		INSERT INTO duplicate_groups (hash, count, total_size, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		`
		result, err := tx.ExecContext(ctx, query, group.Hash, group.Count, group.TotalSize, group.CreatedAt, group.UpdatedAt)
		if err != nil {
			return err
		}

		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		group.ID = int(id)
	} else {
		// Update existing group
		query := `
		UPDATE duplicate_groups 
		SET hash = ?, count = ?, total_size = ?, updated_at = ?
		WHERE id = ?
		`
		_, err := tx.ExecContext(ctx, query, group.Hash, group.Count, group.TotalSize, group.UpdatedAt, group.ID)
		if err != nil {
			return err
		}

		// Delete existing file relationships
		_, err = tx.ExecContext(ctx, "DELETE FROM duplicate_group_files WHERE group_id = ?", group.ID)
		if err != nil {
			return err
		}
	}

	// Insert file relationships
	if len(group.Files) > 0 {
		query := "INSERT INTO duplicate_group_files (group_id, file_id) VALUES "
		values := make([]string, len(group.Files))
		args := make([]interface{}, len(group.Files)*2)

		for i, file := range group.Files {
			values[i] = "(?, ?)"
			args[i*2] = group.ID
			args[i*2+1] = file.ID
		}

		query += strings.Join(values, ", ")
		_, err = tx.ExecContext(ctx, query, args...)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *DuplicateGroupRepository) FindByID(ctx context.Context, id int) (*entities.DuplicateGroup, error) {
	query := `
	SELECT id, hash, count, total_size, created_at, updated_at
	FROM duplicate_groups WHERE id = ?
	`

	var group entities.DuplicateGroup
	var createdAtStr, updatedAtStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&group.ID, &group.Hash, &group.Count, &group.TotalSize,
		&createdAtStr, &updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse created time
	if createdAtStr.Valid && createdAtStr.String != "" {
		if createdAt, err := time.Parse(time.RFC3339, createdAtStr.String); err == nil {
			group.CreatedAt = createdAt
		} else if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr.String); err == nil {
			group.CreatedAt = createdAt
		}
	}

	// Parse updated time
	if updatedAtStr.Valid && updatedAtStr.String != "" {
		if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr.String); err == nil {
			group.UpdatedAt = updatedAt
		} else if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr.String); err == nil {
			group.UpdatedAt = updatedAt
		}
	}

	// Load files
	files, err := r.loadFilesForGroup(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	group.Files = files

	return &group, nil
}

func (r *DuplicateGroupRepository) FindByHash(ctx context.Context, hash string) (*entities.DuplicateGroup, error) {
	query := `
	SELECT id, hash, count, total_size, created_at, updated_at
	FROM duplicate_groups WHERE hash = ?
	`

	var group entities.DuplicateGroup
	var createdAtStr, updatedAtStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, hash).Scan(
		&group.ID, &group.Hash, &group.Count, &group.TotalSize,
		&createdAtStr, &updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Parse created time
	if createdAtStr.Valid && createdAtStr.String != "" {
		if createdAt, err := time.Parse(time.RFC3339, createdAtStr.String); err == nil {
			group.CreatedAt = createdAt
		} else if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr.String); err == nil {
			group.CreatedAt = createdAt
		}
	}

	// Parse updated time
	if updatedAtStr.Valid && updatedAtStr.String != "" {
		if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr.String); err == nil {
			group.UpdatedAt = updatedAt
		} else if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr.String); err == nil {
			group.UpdatedAt = updatedAt
		}
	}

	// Load files
	files, err := r.loadFilesForGroup(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	group.Files = files

	return &group, nil
}

func (r *DuplicateGroupRepository) FindAll(ctx context.Context, limit, offset int) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT id, hash, count, total_size, created_at, updated_at
	FROM duplicate_groups 
	ORDER BY total_size DESC, updated_at DESC
	LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

func (r *DuplicateGroupRepository) FindByMinSize(ctx context.Context, minSize int64, limit int) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT id, hash, count, total_size, created_at, updated_at
	FROM duplicate_groups 
	WHERE total_size >= ?
	ORDER BY total_size DESC, updated_at DESC
	LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, minSize, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

func (r *DuplicateGroupRepository) Delete(ctx context.Context, id int) error {
	// Disable foreign key constraints globally
	_, err := r.db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	if err != nil {
		log.Printf("⚠️ 외래 키 제약 조건 비활성화 실패: %v", err)
	} else {
		log.Printf("🔓 외래 키 제약 조건 전역 비활성화")
	}
	
	// Ensure we re-enable foreign keys at the end
	defer func() {
		_, err := r.db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
		if err != nil {
			log.Printf("⚠️ 외래 키 제약 조건 재활성화 실패: %v", err)
		} else {
			log.Printf("🔒 외래 키 제약 조건 재활성화")
		}
	}()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check what references exist for this group
	var fileCount int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM duplicate_group_files WHERE group_id = ?", id).Scan(&fileCount)
	if err != nil {
		return err
	}
	log.Printf("🔍 그룹 ID %d의 파일 관계 수: %d", id, fileCount)

	// First delete all related records in any order since foreign keys are disabled
	
	// 1. Delete comparison_duplicate_files that reference files in this group
	result, err := tx.ExecContext(ctx, "DELETE FROM comparison_duplicate_files WHERE file_id IN (SELECT dgf.file_id FROM duplicate_group_files dgf WHERE dgf.group_id = ?)", id)
	if err == nil {
		affected, _ := result.RowsAffected()
		if affected > 0 {
			log.Printf("✅ comparison_duplicate_files 관련 레코드 삭제: %d개", affected)
		}
	}

	// 2. Delete duplicate_group_files relationships
	result, err = tx.ExecContext(ctx, "DELETE FROM duplicate_group_files WHERE group_id = ?", id)
	if err != nil {
		log.Printf("❌ 파일 관계 삭제 실패: %v", err)
		return err
	}
	
	affected, _ := result.RowsAffected()
	log.Printf("✅ 파일 관계 삭제 완료: %d개 행 삭제됨", affected)

	// 3. Finally delete the duplicate group itself
	result, err = tx.ExecContext(ctx, "DELETE FROM duplicate_groups WHERE id = ?", id)
	if err != nil {
		log.Printf("❌ 그룹 삭제 실패: %v", err)
		return err
	}
	
	affected, _ = result.RowsAffected()
	log.Printf("✅ 그룹 삭제 완료: %d개 행 삭제됨", affected)

	return tx.Commit()
}

func (r *DuplicateGroupRepository) DeleteByHash(ctx context.Context, hash string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get group ID first
	var groupID int
	err = tx.QueryRowContext(ctx, "SELECT id FROM duplicate_groups WHERE hash = ?", hash).Scan(&groupID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // Group doesn't exist, nothing to delete
		}
		return err
	}

	// First delete the file relationships (child table)
	_, err = tx.ExecContext(ctx, "DELETE FROM duplicate_group_files WHERE group_id = ?", groupID)
	if err != nil {
		return err
	}

	// Then delete the duplicate group (parent table)
	_, err = tx.ExecContext(ctx, "DELETE FROM duplicate_groups WHERE hash = ?", hash)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (r *DuplicateGroupRepository) Count(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM duplicate_groups`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

func (r *DuplicateGroupRepository) GetTotalWastedSpace(ctx context.Context) (int64, error) {
	query := `
	SELECT COALESCE(SUM((count - 1) * (total_size / count)), 0)
	FROM duplicate_groups
	WHERE count > 1
	`
	var wastedSpace int64
	err := r.db.QueryRowContext(ctx, query).Scan(&wastedSpace)
	return wastedSpace, err
}

func (r *DuplicateGroupRepository) RemoveFileFromGroup(ctx context.Context, groupID int, fileID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Remove file from group
	_, err = tx.ExecContext(ctx, "DELETE FROM duplicate_group_files WHERE group_id = ? AND file_id = ?", groupID, fileID)
	if err != nil {
		return err
	}

	// Update group count and total size
	query := `
	UPDATE duplicate_groups 
	SET count = (
		SELECT COUNT(*) 
		FROM duplicate_group_files 
		WHERE group_id = ?
	),
	total_size = (
		SELECT COALESCE(SUM(f.size), 0)
		FROM duplicate_group_files dgf
		JOIN files f ON f.id = dgf.file_id
		WHERE dgf.group_id = ?
	),
	updated_at = ?
	WHERE id = ?
	`

	_, err = tx.ExecContext(ctx, query, groupID, groupID, time.Now(), groupID)
	if err != nil {
		return err
	}

	// Check if group should be deleted (less than 2 files)
	var count int
	err = tx.QueryRowContext(ctx, "SELECT count FROM duplicate_groups WHERE id = ?", groupID).Scan(&count)
	if err != nil {
		return err
	}

	if count < 2 {
		_, err = tx.ExecContext(ctx, "DELETE FROM duplicate_groups WHERE id = ?", groupID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *DuplicateGroupRepository) RefreshFromFiles(ctx context.Context) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Clear existing groups
	_, err = tx.ExecContext(ctx, "DELETE FROM duplicate_groups")
	if err != nil {
		return err
	}

	// Find all duplicate hashes
	query := `
	SELECT hash, COUNT(*) as file_count, SUM(size) as total_size
	FROM files 
	WHERE hash_calculated = TRUE AND hash IS NOT NULL AND hash != ''
	GROUP BY hash
	HAVING file_count > 1
	ORDER BY total_size DESC
	`

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var hash string
		var fileCount int
		var totalSize int64

		err := rows.Scan(&hash, &fileCount, &totalSize)
		if err != nil {
			return err
		}

		// Insert duplicate group
		insertQuery := `
		INSERT INTO duplicate_groups (hash, count, total_size, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		`
		result, err := tx.ExecContext(ctx, insertQuery, hash, fileCount, totalSize, now, now)
		if err != nil {
			return err
		}

		groupID, err := result.LastInsertId()
		if err != nil {
			return err
		}

		// Insert file relationships
		fileQuery := `
		INSERT INTO duplicate_group_files (group_id, file_id)
		SELECT ?, id FROM files WHERE hash = ?
		`
		_, err = tx.ExecContext(ctx, fileQuery, groupID, hash)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Helper methods

func (r *DuplicateGroupRepository) scanGroup(rows *sql.Rows) (*entities.DuplicateGroup, error) {
	var group entities.DuplicateGroup
	var createdAtStr, updatedAtStr sql.NullString

	err := rows.Scan(
		&group.ID, &group.Hash, &group.Count, &group.TotalSize,
		&createdAtStr, &updatedAtStr,
	)
	if err != nil {
		return nil, err
	}

	// Parse created time
	if createdAtStr.Valid && createdAtStr.String != "" {
		if createdAt, err := time.Parse(time.RFC3339, createdAtStr.String); err == nil {
			group.CreatedAt = createdAt
		} else if createdAt, err := time.Parse("2006-01-02 15:04:05", createdAtStr.String); err == nil {
			group.CreatedAt = createdAt
		}
	}

	// Parse updated time
	if updatedAtStr.Valid && updatedAtStr.String != "" {
		if updatedAt, err := time.Parse(time.RFC3339, updatedAtStr.String); err == nil {
			group.UpdatedAt = updatedAt
		} else if updatedAt, err := time.Parse("2006-01-02 15:04:05", updatedAtStr.String); err == nil {
			group.UpdatedAt = updatedAt
		}
	}

	return &group, nil
}

func (r *DuplicateGroupRepository) loadFilesForGroup(ctx context.Context, groupID int) ([]*entities.File, error) {
	query := `
	SELECT f.id, f.name, f.size, f.mime_type, f.modified_time, f.hash, 
		   f.hash_calculated, f.parents, f.path, f.web_view_link, f.last_updated
	FROM files f
	JOIN duplicate_group_files dgf ON f.id = dgf.file_id
	WHERE dgf.group_id = ?
	ORDER BY f.modified_time ASC
	`

	rows, err := r.db.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*entities.File
	for rows.Next() {
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
			}
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

// AddFileToGroup adds a file to a duplicate group
func (r *DuplicateGroupRepository) AddFileToGroup(ctx context.Context, groupID int, file *entities.File) error {
	query := `
	INSERT OR IGNORE INTO duplicate_group_files (group_id, file_id)
	VALUES (?, ?)
	`
	_, err := r.db.ExecContext(ctx, query, groupID, file.ID)
	return err
}

// CleanupEmptyGroups removes duplicate groups that have less than 2 files
func (r *DuplicateGroupRepository) CleanupEmptyGroups(ctx context.Context) (int, error) {
	query := `DELETE FROM duplicate_groups WHERE id IN (
		SELECT dg.id FROM duplicate_groups dg
		LEFT JOIN duplicate_group_files dgf ON dg.id = dgf.group_id
		GROUP BY dg.id
		HAVING COUNT(dgf.file_id) < 2
	)`

	result, err := r.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	return int(rowsAffected), err
}

// CountValid returns the count of valid duplicate groups (groups with 2 or more files)
func (r *DuplicateGroupRepository) CountValid(ctx context.Context) (int, error) {
	query := `
	SELECT COUNT(DISTINCT dg.id) 
	FROM duplicate_groups dg
	INNER JOIN duplicate_group_files dgf ON dg.id = dgf.group_id
	GROUP BY dg.id
	HAVING COUNT(dgf.file_id) >= 2
	`
	var count int
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	return count, err
}

// DeleteBatch deletes multiple duplicate groups by their IDs
func (r *DuplicateGroupRepository) DeleteBatch(ctx context.Context, ids []int) error {
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

	query := `DELETE FROM duplicate_groups WHERE id IN (` + strings.Join(placeholders, ",") + `)`
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// GetByHash returns a duplicate group by its hash (alias for FindByHash)
func (r *DuplicateGroupRepository) GetByHash(ctx context.Context, hash string) (*entities.DuplicateGroup, error) {
	return r.FindByHash(ctx, hash)
}

// GetByID returns a duplicate group by its ID (alias for FindByID)
func (r *DuplicateGroupRepository) GetByID(ctx context.Context, id int) (*entities.DuplicateGroup, error) {
	return r.FindByID(ctx, id)
}

// GetAll returns all duplicate groups (alias for FindAll)
func (r *DuplicateGroupRepository) GetAll(ctx context.Context) ([]*entities.DuplicateGroup, error) {
	return r.FindAll(ctx, -1, 0) // Use existing FindAll with no limit
}

// Update updates an existing duplicate group
func (r *DuplicateGroupRepository) Update(ctx context.Context, group *entities.DuplicateGroup) error {
	return r.Save(ctx, group) // Use existing Save method which handles updates
}

// GetGroupFiles returns all files in a duplicate group
func (r *DuplicateGroupRepository) GetGroupFiles(ctx context.Context, groupID int) ([]*entities.File, error) {
	return r.loadFilesForGroup(ctx, groupID)
}

// GetValidGroups returns all duplicate groups that have more than one file
func (r *DuplicateGroupRepository) GetValidGroups(ctx context.Context) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	WHERE dg.count > 1
	ORDER BY dg.total_size DESC, dg.updated_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// GetGroupsWithMinFiles returns groups with at least the specified number of files
func (r *DuplicateGroupRepository) GetGroupsWithMinFiles(ctx context.Context, minFiles int) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	WHERE dg.count >= ?
	ORDER BY dg.count DESC, dg.total_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, minFiles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// GetGroupsWithMinSize returns groups with at least the specified total size
func (r *DuplicateGroupRepository) GetGroupsWithMinSize(ctx context.Context, minSize int64) ([]*entities.DuplicateGroup, error) {
	return r.FindByMinSize(ctx, minSize, -1) // Use existing method with no limit
}

// GetGroupsByFileCount returns groups ordered by file count
func (r *DuplicateGroupRepository) GetGroupsByFileCount(ctx context.Context, ascending bool) ([]*entities.DuplicateGroup, error) {
	order := "DESC"
	if ascending {
		order = "ASC"
	}

	query := `
	SELECT id, hash, count, total_size, created_at, updated_at
	FROM duplicate_groups 
	ORDER BY count ` + order + `, total_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// GetGroupsByTotalSize returns groups ordered by total size
func (r *DuplicateGroupRepository) GetGroupsByTotalSize(ctx context.Context, ascending bool) ([]*entities.DuplicateGroup, error) {
	order := "DESC"
	if ascending {
		order = "ASC"
	}

	query := `
	SELECT id, hash, count, total_size, created_at, updated_at
	FROM duplicate_groups 
	ORDER BY total_size ` + order + `, count DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// GetTotalDuplicateFiles returns the total number of duplicate files across all groups
func (r *DuplicateGroupRepository) GetTotalDuplicateFiles(ctx context.Context) (int, error) {
	query := `SELECT COALESCE(SUM(count), 0) FROM duplicate_groups WHERE count > 1`
	var total int
	err := r.db.QueryRowContext(ctx, query).Scan(&total)
	return total, err
}

// GetGroupsInFolder returns duplicate groups that have files in the specified folder
func (r *DuplicateGroupRepository) GetGroupsInFolder(ctx context.Context, folderID string) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT DISTINCT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	JOIN duplicate_group_files dgf ON dg.id = dgf.group_id
	JOIN files f ON dgf.file_id = f.id
	WHERE f.parents LIKE '%' || ? || '%' OR f.id = ?
	ORDER BY dg.total_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, folderID, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// GetGroupsWithFilesInBothFolders returns groups that have files in both specified folders
func (r *DuplicateGroupRepository) GetGroupsWithFilesInBothFolders(ctx context.Context, folderID1, folderID2 string) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT DISTINCT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	WHERE EXISTS (
		SELECT 1 FROM duplicate_group_files dgf1
		JOIN files f1 ON dgf1.file_id = f1.id
		WHERE dgf1.group_id = dg.id AND (f1.parents LIKE '%' || ? || '%' OR f1.id = ?)
	) AND EXISTS (
		SELECT 1 FROM duplicate_group_files dgf2
		JOIN files f2 ON dgf2.file_id = f2.id
		WHERE dgf2.group_id = dg.id AND (f2.parents LIKE '%' || ? || '%' OR f2.id = ?)
	)
	ORDER BY dg.total_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, folderID1, folderID1, folderID2, folderID2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// DeleteGroupsOlderThan deletes duplicate groups older than the specified number of days
func (r *DuplicateGroupRepository) DeleteGroupsOlderThan(ctx context.Context, days int) (int, error) {
	query := `DELETE FROM duplicate_groups WHERE created_at < datetime('now', '-' || ? || ' days')`
	result, err := r.db.ExecContext(ctx, query, days)
	if err != nil {
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	return int(rowsAffected), err
}

// GetPaginated returns duplicate groups with pagination
func (r *DuplicateGroupRepository) GetPaginated(ctx context.Context, offset, limit int) ([]*entities.DuplicateGroup, error) {
	return r.FindAll(ctx, limit, offset)
}

// GetValidGroupsPaginated returns valid duplicate groups with pagination
func (r *DuplicateGroupRepository) GetValidGroupsPaginated(ctx context.Context, offset, limit int) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	WHERE dg.count > 1
	ORDER BY dg.total_size DESC, dg.updated_at DESC
	LIMIT ? OFFSET ?
	`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// SearchByFileName returns groups containing files with names matching the search term
func (r *DuplicateGroupRepository) SearchByFileName(ctx context.Context, filename string) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT DISTINCT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	JOIN duplicate_group_files dgf ON dg.id = dgf.group_id
	JOIN files f ON dgf.file_id = f.id
	WHERE f.name LIKE '%' || ? || '%'
	ORDER BY dg.total_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, filename)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// SearchByMimeType returns groups containing files with the specified MIME type
func (r *DuplicateGroupRepository) SearchByMimeType(ctx context.Context, mimeType string) ([]*entities.DuplicateGroup, error) {
	query := `
	SELECT DISTINCT dg.id, dg.hash, dg.count, dg.total_size, dg.created_at, dg.updated_at
	FROM duplicate_groups dg
	JOIN duplicate_group_files dgf ON dg.id = dgf.group_id
	JOIN files f ON dgf.file_id = f.id
	WHERE f.mime_type = ?
	ORDER BY dg.total_size DESC
	`

	rows, err := r.db.QueryContext(ctx, query, mimeType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []*entities.DuplicateGroup
	for rows.Next() {
		group, err := r.scanGroup(rows)
		if err != nil {
			return nil, err
		}

		// Load files
		files, err := r.loadFilesForGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		group.Files = files

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// Exists checks if a duplicate group exists by ID
func (r *DuplicateGroupRepository) Exists(ctx context.Context, id int) (bool, error) {
	query := `SELECT COUNT(*) FROM duplicate_groups WHERE id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, id).Scan(&count)
	return count > 0, err
}

// ExistsByHash checks if a duplicate group exists with the specified hash
func (r *DuplicateGroupRepository) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	query := `SELECT COUNT(*) FROM duplicate_groups WHERE hash = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, hash).Scan(&count)
	return count > 0, err
}

// SaveBatch saves multiple duplicate groups in a single transaction
func (r *DuplicateGroupRepository) SaveBatch(ctx context.Context, groups []*entities.DuplicateGroup) error {
	if len(groups) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, group := range groups {
		err := r.Save(ctx, group)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
