package entities

import (
	"encoding/json"
	"time"
)

// RuleCondition - 규칙 조건 (JSON으로 저장)
type RuleCondition struct {
	Field    string `json:"field"`    // name, extension, mimeType, size, modifiedDate
	Operator string `json:"operator"` // contains, equals, startsWith, endsWith, greaterThan, lessThan, matches(regex)
	Value    string `json:"value"`
}

// OrganizationRule - 단일 정리 규칙
type OrganizationRule struct {
	ID          int       `json:"id"`
	RuleSetID   int       `json:"ruleSetId"`
	Priority    int       `json:"priority"`    // 낮을수록 먼저 적용
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Conditions  string    `json:"conditions"`  // JSON: [{field, operator, value}]
	Action      string    `json:"action"`      // move, copy
	TargetPath  string    `json:"targetPath"`  // 대상 폴더 경로/ID
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// GetConditions parses the JSON conditions string into RuleCondition slice
func (r *OrganizationRule) GetConditions() ([]RuleCondition, error) {
	var conditions []RuleCondition
	if r.Conditions == "" || r.Conditions == "[]" {
		return conditions, nil
	}
	err := json.Unmarshal([]byte(r.Conditions), &conditions)
	return conditions, err
}

// SetConditions serializes RuleCondition slice into JSON string
func (r *OrganizationRule) SetConditions(conditions []RuleCondition) error {
	data, err := json.Marshal(conditions)
	if err != nil {
		return err
	}
	r.Conditions = string(data)
	return nil
}

// OrganizationRuleSet - 규칙 세트 (여러 규칙 그룹)
type OrganizationRuleSet struct {
	ID           int                 `json:"id"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	WatchFolder  string              `json:"watchFolder"`  // 감시 대상 폴더 ID
	IsActive     bool                `json:"isActive"`
	BackupFileID string              `json:"backupFileId"` // Google Drive JSON 백업 파일 ID
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
	Rules        []OrganizationRule  `json:"rules,omitempty"` // DB에서 별도 조회
}

// RuleSuggestion - AI가 제안하는 규칙
type RuleSuggestion struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Conditions  []RuleCondition `json:"conditions"`
	Action      string          `json:"action"`    // move, copy
	TargetPath  string          `json:"targetPath"`
}
