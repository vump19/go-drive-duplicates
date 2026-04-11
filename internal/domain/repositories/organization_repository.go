package repositories

import (
	"context"
	"go-drive-duplicates/internal/domain/entities"
)

// OrganizationRuleSetRepository - 규칙 세트 리포지토리 인터페이스
type OrganizationRuleSetRepository interface {
	Create(ctx context.Context, ruleSet *entities.OrganizationRuleSet) error
	GetByID(ctx context.Context, id int) (*entities.OrganizationRuleSet, error)
	List(ctx context.Context) ([]*entities.OrganizationRuleSet, error)
	Update(ctx context.Context, ruleSet *entities.OrganizationRuleSet) error
	Delete(ctx context.Context, id int) error
	GetActive(ctx context.Context) ([]*entities.OrganizationRuleSet, error)
}

// OrganizationRuleRepository - 규칙 리포지토리 인터페이스
type OrganizationRuleRepository interface {
	Create(ctx context.Context, rule *entities.OrganizationRule) error
	GetByID(ctx context.Context, id int) (*entities.OrganizationRule, error)
	GetByRuleSetID(ctx context.Context, ruleSetID int) ([]*entities.OrganizationRule, error)
	Update(ctx context.Context, rule *entities.OrganizationRule) error
	Delete(ctx context.Context, id int) error
	DeleteByRuleSetID(ctx context.Context, ruleSetID int) error
}

// OrganizationLogRepository - 정리 기록 리포지토리 인터페이스
type OrganizationLogRepository interface {
	Create(ctx context.Context, log *entities.OrganizationLog) error
	GetByRuleSetID(ctx context.Context, ruleSetID int, limit, offset int) ([]*entities.OrganizationLog, error)
	CountByRuleSetID(ctx context.Context, ruleSetID int) (int, error)
	GetRecent(ctx context.Context, limit int) ([]*entities.OrganizationLog, error)
	DeleteByRuleSetID(ctx context.Context, ruleSetID int) error
}

// ChatMessageRepository - 채팅 메시지 리포지토리 인터페이스
type ChatMessageRepository interface {
	Create(ctx context.Context, msg *entities.ChatMessage) error
	GetBySessionID(ctx context.Context, sessionID string) ([]*entities.ChatMessage, error)
	DeleteBySessionID(ctx context.Context, sessionID string) error
}
