package services

import (
	"context"
	"errors"
	"go-drive-duplicates/internal/domain/entities"
)

// ErrRateLimit - AI API 레이트 리밋 초과 에러
var ErrRateLimit = errors.New("AI API 레이트 리밋 초과")

// AIService - AI 서비스 인터페이스
type AIService interface {
	Chat(ctx context.Context, messages []AIMessage, systemPrompt string) (*AIResponse, error)
}

// AIMessage - AI에게 보내는 메시지
type AIMessage struct {
	Role    string `json:"role"`    // user, assistant
	Content string `json:"content"`
}

// AIResponse - AI 응답
type AIResponse struct {
	Content         string                    `json:"content"`
	RuleSuggestions []entities.RuleSuggestion  `json:"ruleSuggestions,omitempty"`
}
