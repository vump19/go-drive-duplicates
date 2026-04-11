package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
)

type verifiedDuplicateRepositorySQLite struct {
	db *sql.DB
}

// NewVerifiedDuplicateRepository creates a new SQLite implementation
func NewVerifiedDuplicateRepository(db *sql.DB) repositories.VerifiedDuplicateRepository {
	return &verifiedDuplicateRepositorySQLite{db: db}
}

func (r *verifiedDuplicateRepositorySQLite) Create(ctx context.Context, verified *entities.VerifiedDuplicate) error {
	query := `
		INSERT INTO verified_duplicates (hash, file_count, total_size, status, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		verified.Hash,
		verified.FileCount,
		verified.TotalSize,
		verified.Status,
		verified.Description,
		now,
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to create verified duplicate: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}

	verified.ID = int(id)
	verified.CreatedAt = now
	verified.UpdatedAt = now

	return nil
}

func (r *verifiedDuplicateRepositorySQLite) GetByHash(ctx context.Context, hash string) (*entities.VerifiedDuplicate, error) {
	query := `
		SELECT id, hash, file_count, total_size, status, description, created_at, updated_at
		FROM verified_duplicates
		WHERE hash = ?
	`

	row := r.db.QueryRowContext(ctx, query, hash)

	var verified entities.VerifiedDuplicate
	var createdAtStr, updatedAtStr string

	err := row.Scan(
		&verified.ID,
		&verified.Hash,
		&verified.FileCount,
		&verified.TotalSize,
		&verified.Status,
		&verified.Description,
		&createdAtStr,
		&updatedAtStr,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get verified duplicate by hash: %w", err)
	}

	// Parse time strings
	if verified.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr); err != nil {
		if verified.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr); err != nil {
			return nil, fmt.Errorf("failed to parse created_at: %w", err)
		}
	}

	if verified.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr); err != nil {
		if verified.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr); err != nil {
			return nil, fmt.Errorf("failed to parse updated_at: %w", err)
		}
	}

	return &verified, nil
}

func (r *verifiedDuplicateRepositorySQLite) GetByStatus(ctx context.Context, status entities.VerifiedDuplicateStatus) ([]*entities.VerifiedDuplicate, error) {
	query := `
		SELECT id, hash, file_count, total_size, status, description, created_at, updated_at
		FROM verified_duplicates
		WHERE status = ?
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to query verified duplicates by status: %w", err)
	}
	defer rows.Close()

	var results []*entities.VerifiedDuplicate

	for rows.Next() {
		var verified entities.VerifiedDuplicate
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&verified.ID,
			&verified.Hash,
			&verified.FileCount,
			&verified.TotalSize,
			&verified.Status,
			&verified.Description,
			&createdAtStr,
			&updatedAtStr,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan verified duplicate row: %w", err)
		}

		// Parse time strings
		if verified.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr); err != nil {
			if verified.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr); err != nil {
				return nil, fmt.Errorf("failed to parse created_at: %w", err)
			}
		}

		if verified.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr); err != nil {
			if verified.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr); err != nil {
				return nil, fmt.Errorf("failed to parse updated_at: %w", err)
			}
		}

		results = append(results, &verified)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating verified duplicate rows: %w", err)
	}

	return results, nil
}

func (r *verifiedDuplicateRepositorySQLite) Update(ctx context.Context, verified *entities.VerifiedDuplicate) error {
	query := `
		UPDATE verified_duplicates
		SET status = ?, description = ?, updated_at = ?
		WHERE id = ?
	`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		verified.Status,
		verified.Description,
		now,
		verified.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update verified duplicate: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("verified duplicate with ID %d not found", verified.ID)
	}

	verified.UpdatedAt = now

	return nil
}

func (r *verifiedDuplicateRepositorySQLite) Delete(ctx context.Context, id int) error {
	query := `DELETE FROM verified_duplicates WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete verified duplicate: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("verified duplicate with ID %d not found", id)
	}

	return nil
}

func (r *verifiedDuplicateRepositorySQLite) List(ctx context.Context, filter *entities.VerifiedDuplicateFilter) ([]*entities.VerifiedDuplicate, error) {
	query := `
		SELECT id, hash, file_count, total_size, status, description, created_at, updated_at
		FROM verified_duplicates
		WHERE 1=1
	`

	var args []interface{}

	if filter != nil {
		if filter.Status != "" {
			query += " AND status = ?"
			args = append(args, filter.Status)
		}

		if len(filter.HashList) > 0 {
			placeholders := strings.Repeat("?,", len(filter.HashList))
			placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma
			query += " AND hash IN (" + placeholders + ")"

			for _, hash := range filter.HashList {
				args = append(args, hash)
			}
		}

		if filter.CreatedAt != nil {
			query += " AND created_at >= ?"
			args = append(args, filter.CreatedAt.Format(time.RFC3339))
		}
	}

	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query verified duplicates: %w", err)
	}
	defer rows.Close()

	var results []*entities.VerifiedDuplicate

	for rows.Next() {
		var verified entities.VerifiedDuplicate
		var createdAtStr, updatedAtStr string

		err := rows.Scan(
			&verified.ID,
			&verified.Hash,
			&verified.FileCount,
			&verified.TotalSize,
			&verified.Status,
			&verified.Description,
			&createdAtStr,
			&updatedAtStr,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan verified duplicate row: %w", err)
		}

		// Parse time strings
		if verified.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr); err != nil {
			if verified.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr); err != nil {
				return nil, fmt.Errorf("failed to parse created_at: %w", err)
			}
		}

		if verified.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr); err != nil {
			if verified.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr); err != nil {
				return nil, fmt.Errorf("failed to parse updated_at: %w", err)
			}
		}

		results = append(results, &verified)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating verified duplicate rows: %w", err)
	}

	return results, nil
}

func (r *verifiedDuplicateRepositorySQLite) ExistsByHash(ctx context.Context, hash string) (bool, error) {
	query := `SELECT 1 FROM verified_duplicates WHERE hash = ? LIMIT 1`

	var exists int
	err := r.db.QueryRowContext(ctx, query, hash).Scan(&exists)

	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check if verified duplicate exists: %w", err)
	}

	return true, nil
}

func (r *verifiedDuplicateRepositorySQLite) GetVerifiedHashes(ctx context.Context) ([]string, error) {
	query := `
		SELECT hash 
		FROM verified_duplicates 
		WHERE status IN ('verified', 'ignored')
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get verified hashes: %w", err)
	}
	defer rows.Close()

	var hashes []string

	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("failed to scan hash: %w", err)
		}
		hashes = append(hashes, hash)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating hash rows: %w", err)
	}

	return hashes, nil
}

func (r *verifiedDuplicateRepositorySQLite) BatchCheckVerified(ctx context.Context, hashes []string) (map[string]*entities.VerifiedDuplicate, error) {
	if len(hashes) == 0 {
		return make(map[string]*entities.VerifiedDuplicate), nil
	}

	// SQLite has a limit on the number of variables in a query
	const batchSize = 999
	result := make(map[string]*entities.VerifiedDuplicate)

	for i := 0; i < len(hashes); i += batchSize {
		end := i + batchSize
		if end > len(hashes) {
			end = len(hashes)
		}

		batch := hashes[i:end]
		placeholders := strings.Repeat("?,", len(batch))
		placeholders = placeholders[:len(placeholders)-1] // Remove trailing comma

		query := fmt.Sprintf(`
			SELECT id, hash, file_count, total_size, status, description, created_at, updated_at
			FROM verified_duplicates
			WHERE hash IN (%s)
		`, placeholders)

		var args []interface{}
		for _, hash := range batch {
			args = append(args, hash)
		}

		rows, err := r.db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to batch check verified duplicates: %w", err)
		}

		for rows.Next() {
			var verified entities.VerifiedDuplicate
			var createdAtStr, updatedAtStr string

			err := rows.Scan(
				&verified.ID,
				&verified.Hash,
				&verified.FileCount,
				&verified.TotalSize,
				&verified.Status,
				&verified.Description,
				&createdAtStr,
				&updatedAtStr,
			)

			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan verified duplicate row: %w", err)
			}

			// Parse time strings
			if verified.CreatedAt, err = time.Parse(time.RFC3339, createdAtStr); err != nil {
				if verified.CreatedAt, err = time.Parse("2006-01-02 15:04:05", createdAtStr); err != nil {
					rows.Close()
					return nil, fmt.Errorf("failed to parse created_at: %w", err)
				}
			}

			if verified.UpdatedAt, err = time.Parse(time.RFC3339, updatedAtStr); err != nil {
				if verified.UpdatedAt, err = time.Parse("2006-01-02 15:04:05", updatedAtStr); err != nil {
					rows.Close()
					return nil, fmt.Errorf("failed to parse updated_at: %w", err)
				}
			}

			result[verified.Hash] = &verified
		}

		rows.Close()

		if err = rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating verified duplicate rows: %w", err)
		}
	}

	return result, nil
}
