package entities

import "time"

// OrganizationLog - 파일 정리 기록
type OrganizationLog struct {
	ID           int       `json:"id"`
	RuleSetID    int       `json:"ruleSetId"`
	RuleID       int       `json:"ruleId"`
	RuleName     string    `json:"ruleName"`
	FileID       string    `json:"fileId"`
	FileName     string    `json:"fileName"`
	FileSize     int64     `json:"fileSize"`
	FileMimeType string    `json:"fileMimeType"`
	SourceFolder string    `json:"sourceFolder"`
	SourcePath   string    `json:"sourcePath"`
	TargetFolder string    `json:"targetFolder"`
	TargetPath   string    `json:"targetPath"`
	Action       string    `json:"action"` // moved, copied, skipped, failed
	ErrorMessage string    `json:"errorMessage,omitempty"`
	ExecutedAt   time.Time `json:"executedAt"`
}
