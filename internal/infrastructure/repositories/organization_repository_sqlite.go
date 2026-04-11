package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/repositories"
)

// --- 시간 파싱 헬퍼 ---

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("failed to parse time: %s", s)
}

// ============================================================
// OrganizationRuleSetRepository SQLite 구현
// ============================================================

type orgRuleSetRepoSQLite struct {
	db *sql.DB
}

func NewOrganizationRuleSetRepository(db *sql.DB) repositories.OrganizationRuleSetRepository {
	return &orgRuleSetRepoSQLite{db: db}
}

func (r *orgRuleSetRepoSQLite) Create(ctx context.Context, rs *entities.OrganizationRuleSet) error {
	query := `INSERT INTO organization_rule_sets (name, description, watch_folder, is_active, backup_file_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		rs.Name, rs.Description, rs.WatchFolder, rs.IsActive, rs.BackupFileID, now, now)
	if err != nil {
		return fmt.Errorf("규칙 세트 생성 실패: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("마지막 삽입 ID 조회 실패: %w", err)
	}
	rs.ID = int(id)
	rs.CreatedAt = now
	rs.UpdatedAt = now
	return nil
}

func (r *orgRuleSetRepoSQLite) GetByID(ctx context.Context, id int) (*entities.OrganizationRuleSet, error) {
	query := `SELECT id, name, description, watch_folder, is_active, backup_file_id, created_at, updated_at
		FROM organization_rule_sets WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	rs, err := scanRuleSet(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("규칙 세트 조회 실패: %w", err)
	}
	return rs, nil
}

func (r *orgRuleSetRepoSQLite) List(ctx context.Context) ([]*entities.OrganizationRuleSet, error) {
	query := `SELECT id, name, description, watch_folder, is_active, backup_file_id, created_at, updated_at
		FROM organization_rule_sets ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("규칙 세트 목록 조회 실패: %w", err)
	}
	defer rows.Close()

	return scanRuleSetRows(rows)
}

func (r *orgRuleSetRepoSQLite) Update(ctx context.Context, rs *entities.OrganizationRuleSet) error {
	query := `UPDATE organization_rule_sets
		SET name = ?, description = ?, watch_folder = ?, is_active = ?, backup_file_id = ?, updated_at = ?
		WHERE id = ?`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		rs.Name, rs.Description, rs.WatchFolder, rs.IsActive, rs.BackupFileID, now, rs.ID)
	if err != nil {
		return fmt.Errorf("규칙 세트 수정 실패: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("규칙 세트를 찾을 수 없습니다: ID %d", rs.ID)
	}
	rs.UpdatedAt = now
	return nil
}

func (r *orgRuleSetRepoSQLite) Delete(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM organization_rule_sets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("규칙 세트 삭제 실패: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("규칙 세트를 찾을 수 없습니다: ID %d", id)
	}
	return nil
}

func (r *orgRuleSetRepoSQLite) GetActive(ctx context.Context) ([]*entities.OrganizationRuleSet, error) {
	query := `SELECT id, name, description, watch_folder, is_active, backup_file_id, created_at, updated_at
		FROM organization_rule_sets WHERE is_active = 1 ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("활성 규칙 세트 조회 실패: %w", err)
	}
	defer rows.Close()

	return scanRuleSetRows(rows)
}

func scanRuleSet(row *sql.Row) (*entities.OrganizationRuleSet, error) {
	var rs entities.OrganizationRuleSet
	var createdStr, updatedStr string

	err := row.Scan(&rs.ID, &rs.Name, &rs.Description, &rs.WatchFolder,
		&rs.IsActive, &rs.BackupFileID, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	rs.CreatedAt, _ = parseTime(createdStr)
	rs.UpdatedAt, _ = parseTime(updatedStr)
	return &rs, nil
}

func scanRuleSetRows(rows *sql.Rows) ([]*entities.OrganizationRuleSet, error) {
	var results []*entities.OrganizationRuleSet
	for rows.Next() {
		var rs entities.OrganizationRuleSet
		var createdStr, updatedStr string

		err := rows.Scan(&rs.ID, &rs.Name, &rs.Description, &rs.WatchFolder,
			&rs.IsActive, &rs.BackupFileID, &createdStr, &updatedStr)
		if err != nil {
			return nil, fmt.Errorf("규칙 세트 행 스캔 실패: %w", err)
		}

		rs.CreatedAt, _ = parseTime(createdStr)
		rs.UpdatedAt, _ = parseTime(updatedStr)
		results = append(results, &rs)
	}
	return results, rows.Err()
}

// ============================================================
// OrganizationRuleRepository SQLite 구현
// ============================================================

type orgRuleRepoSQLite struct {
	db *sql.DB
}

func NewOrganizationRuleRepository(db *sql.DB) repositories.OrganizationRuleRepository {
	return &orgRuleRepoSQLite{db: db}
}

func (r *orgRuleRepoSQLite) Create(ctx context.Context, rule *entities.OrganizationRule) error {
	query := `INSERT INTO organization_rules (rule_set_id, priority, name, description, conditions, action, target_path, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		rule.RuleSetID, rule.Priority, rule.Name, rule.Description,
		rule.Conditions, rule.Action, rule.TargetPath, rule.Enabled, now, now)
	if err != nil {
		return fmt.Errorf("규칙 생성 실패: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("마지막 삽입 ID 조회 실패: %w", err)
	}
	rule.ID = int(id)
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return nil
}

func (r *orgRuleRepoSQLite) GetByID(ctx context.Context, id int) (*entities.OrganizationRule, error) {
	query := `SELECT id, rule_set_id, priority, name, description, conditions, action, target_path, enabled, created_at, updated_at
		FROM organization_rules WHERE id = ?`

	row := r.db.QueryRowContext(ctx, query, id)
	rule, err := scanRule(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("규칙 조회 실패: %w", err)
	}
	return rule, nil
}

func (r *orgRuleRepoSQLite) GetByRuleSetID(ctx context.Context, ruleSetID int) ([]*entities.OrganizationRule, error) {
	query := `SELECT id, rule_set_id, priority, name, description, conditions, action, target_path, enabled, created_at, updated_at
		FROM organization_rules WHERE rule_set_id = ? ORDER BY priority ASC, id ASC`

	rows, err := r.db.QueryContext(ctx, query, ruleSetID)
	if err != nil {
		return nil, fmt.Errorf("규칙 목록 조회 실패: %w", err)
	}
	defer rows.Close()

	return scanRuleRows(rows)
}

func (r *orgRuleRepoSQLite) Update(ctx context.Context, rule *entities.OrganizationRule) error {
	query := `UPDATE organization_rules
		SET priority = ?, name = ?, description = ?, conditions = ?, action = ?, target_path = ?, enabled = ?, updated_at = ?
		WHERE id = ?`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		rule.Priority, rule.Name, rule.Description, rule.Conditions,
		rule.Action, rule.TargetPath, rule.Enabled, now, rule.ID)
	if err != nil {
		return fmt.Errorf("규칙 수정 실패: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("규칙을 찾을 수 없습니다: ID %d", rule.ID)
	}
	rule.UpdatedAt = now
	return nil
}

func (r *orgRuleRepoSQLite) Delete(ctx context.Context, id int) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM organization_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("규칙 삭제 실패: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("규칙을 찾을 수 없습니다: ID %d", id)
	}
	return nil
}

func (r *orgRuleRepoSQLite) DeleteByRuleSetID(ctx context.Context, ruleSetID int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM organization_rules WHERE rule_set_id = ?`, ruleSetID)
	if err != nil {
		return fmt.Errorf("규칙 세트의 규칙 삭제 실패: %w", err)
	}
	return nil
}

func scanRule(row *sql.Row) (*entities.OrganizationRule, error) {
	var rule entities.OrganizationRule
	var createdStr, updatedStr string

	err := row.Scan(&rule.ID, &rule.RuleSetID, &rule.Priority, &rule.Name, &rule.Description,
		&rule.Conditions, &rule.Action, &rule.TargetPath, &rule.Enabled, &createdStr, &updatedStr)
	if err != nil {
		return nil, err
	}

	rule.CreatedAt, _ = parseTime(createdStr)
	rule.UpdatedAt, _ = parseTime(updatedStr)
	return &rule, nil
}

func scanRuleRows(rows *sql.Rows) ([]*entities.OrganizationRule, error) {
	var results []*entities.OrganizationRule
	for rows.Next() {
		var rule entities.OrganizationRule
		var createdStr, updatedStr string

		err := rows.Scan(&rule.ID, &rule.RuleSetID, &rule.Priority, &rule.Name, &rule.Description,
			&rule.Conditions, &rule.Action, &rule.TargetPath, &rule.Enabled, &createdStr, &updatedStr)
		if err != nil {
			return nil, fmt.Errorf("규칙 행 스캔 실패: %w", err)
		}

		rule.CreatedAt, _ = parseTime(createdStr)
		rule.UpdatedAt, _ = parseTime(updatedStr)
		results = append(results, &rule)
	}
	return results, rows.Err()
}

// ============================================================
// OrganizationLogRepository SQLite 구현
// ============================================================

type orgLogRepoSQLite struct {
	db *sql.DB
}

func NewOrganizationLogRepository(db *sql.DB) repositories.OrganizationLogRepository {
	return &orgLogRepoSQLite{db: db}
}

func (r *orgLogRepoSQLite) Create(ctx context.Context, logEntry *entities.OrganizationLog) error {
	query := `INSERT INTO organization_logs (rule_set_id, rule_id, rule_name, file_id, file_name, file_size, file_mime_type, source_folder, source_path, target_folder, target_path, action, error_message, executed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		logEntry.RuleSetID, logEntry.RuleID, logEntry.RuleName,
		logEntry.FileID, logEntry.FileName, logEntry.FileSize, logEntry.FileMimeType,
		logEntry.SourceFolder, logEntry.SourcePath, logEntry.TargetFolder, logEntry.TargetPath,
		logEntry.Action, logEntry.ErrorMessage, now)
	if err != nil {
		return fmt.Errorf("정리 기록 생성 실패: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("마지막 삽입 ID 조회 실패: %w", err)
	}
	logEntry.ID = int(id)
	logEntry.ExecutedAt = now
	return nil
}

func (r *orgLogRepoSQLite) GetByRuleSetID(ctx context.Context, ruleSetID int, limit, offset int) ([]*entities.OrganizationLog, error) {
	query := `SELECT id, rule_set_id, rule_id, rule_name, file_id, file_name, file_size, file_mime_type,
		source_folder, source_path, target_folder, target_path, action, error_message, executed_at
		FROM organization_logs WHERE rule_set_id = ? ORDER BY executed_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, ruleSetID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("정리 기록 조회 실패: %w", err)
	}
	defer rows.Close()

	return scanLogRows(rows)
}

func (r *orgLogRepoSQLite) CountByRuleSetID(ctx context.Context, ruleSetID int) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization_logs WHERE rule_set_id = ?`, ruleSetID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("정리 기록 개수 조회 실패: %w", err)
	}
	return count, nil
}

func (r *orgLogRepoSQLite) GetRecent(ctx context.Context, limit int) ([]*entities.OrganizationLog, error) {
	query := `SELECT id, rule_set_id, rule_id, rule_name, file_id, file_name, file_size, file_mime_type,
		source_folder, source_path, target_folder, target_path, action, error_message, executed_at
		FROM organization_logs ORDER BY executed_at DESC LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("최근 정리 기록 조회 실패: %w", err)
	}
	defer rows.Close()

	return scanLogRows(rows)
}

func (r *orgLogRepoSQLite) DeleteByRuleSetID(ctx context.Context, ruleSetID int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM organization_logs WHERE rule_set_id = ?`, ruleSetID)
	if err != nil {
		return fmt.Errorf("정리 기록 삭제 실패: %w", err)
	}
	return nil
}

func scanLogRows(rows *sql.Rows) ([]*entities.OrganizationLog, error) {
	var results []*entities.OrganizationLog
	for rows.Next() {
		var l entities.OrganizationLog
		var executedStr string

		err := rows.Scan(&l.ID, &l.RuleSetID, &l.RuleID, &l.RuleName,
			&l.FileID, &l.FileName, &l.FileSize, &l.FileMimeType,
			&l.SourceFolder, &l.SourcePath, &l.TargetFolder, &l.TargetPath,
			&l.Action, &l.ErrorMessage, &executedStr)
		if err != nil {
			return nil, fmt.Errorf("정리 기록 행 스캔 실패: %w", err)
		}
		l.ExecutedAt, _ = parseTime(executedStr)
		results = append(results, &l)
	}
	return results, rows.Err()
}

// ============================================================
// ChatMessageRepository SQLite 구현
// ============================================================

type chatMsgRepoSQLite struct {
	db *sql.DB
}

func NewChatMessageRepository(db *sql.DB) repositories.ChatMessageRepository {
	return &chatMsgRepoSQLite{db: db}
}

func (r *chatMsgRepoSQLite) Create(ctx context.Context, msg *entities.ChatMessage) error {
	query := `INSERT INTO chat_messages (session_id, role, content, metadata, created_at)
		VALUES (?, ?, ?, ?, ?)`

	now := time.Now()
	result, err := r.db.ExecContext(ctx, query,
		msg.SessionID, msg.Role, msg.Content, msg.Metadata, now)
	if err != nil {
		return fmt.Errorf("채팅 메시지 생성 실패: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("마지막 삽입 ID 조회 실패: %w", err)
	}
	msg.ID = int(id)
	msg.CreatedAt = now
	return nil
}

func (r *chatMsgRepoSQLite) GetBySessionID(ctx context.Context, sessionID string) ([]*entities.ChatMessage, error) {
	query := `SELECT id, session_id, role, content, metadata, created_at
		FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("채팅 메시지 조회 실패: %w", err)
	}
	defer rows.Close()

	var results []*entities.ChatMessage
	for rows.Next() {
		var msg entities.ChatMessage
		var createdStr string

		err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.Metadata, &createdStr)
		if err != nil {
			return nil, fmt.Errorf("채팅 메시지 행 스캔 실패: %w", err)
		}
		msg.CreatedAt, _ = parseTime(createdStr)
		results = append(results, &msg)
	}
	return results, rows.Err()
}

func (r *chatMsgRepoSQLite) DeleteBySessionID(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM chat_messages WHERE session_id = ?`, sessionID)
	if err != nil {
		return fmt.Errorf("채팅 세션 삭제 실패: %w", err)
	}
	return nil
}
