package services

import (
	"context"
	"errors"
	"fmt"
	"log"

	"go-drive-duplicates/internal/domain/services"
)

// NamedProvider - 이름이 있는 AI 프로바이더
type NamedProvider struct {
	Name    string // "claude", "openai", "gemini"
	Service services.AIService
}

// MultiProviderAIService - 여러 AI 프로바이더를 순서대로 시도하는 래퍼
type MultiProviderAIService struct {
	providers []NamedProvider
}

// NewMultiProviderAIService creates a new multi-provider AI service
func NewMultiProviderAIService(providers []NamedProvider) *MultiProviderAIService {
	return &MultiProviderAIService{
		providers: providers,
	}
}

// Chat tries each provider in order, falling back on rate limit errors
func (m *MultiProviderAIService) Chat(ctx context.Context, messages []services.AIMessage, systemPrompt string) (*services.AIResponse, error) {
	if len(m.providers) == 0 {
		return nil, fmt.Errorf("사용 가능한 AI 프로바이더가 없습니다")
	}

	// 프로바이더가 1개면 단순 패스스루
	if len(m.providers) == 1 {
		return m.providers[0].Service.Chat(ctx, messages, systemPrompt)
	}

	var lastErr error
	for i, provider := range m.providers {
		resp, err := provider.Service.Chat(ctx, messages, systemPrompt)
		if err == nil {
			if i > 0 {
				log.Printf("✅ AI 프로바이더 '%s'로 전환 성공", provider.Name)
			}
			return resp, nil
		}

		lastErr = err

		// 레이트 리밋 또는 서버 에러인 경우 다음 프로바이더 시도
		if errors.Is(err, services.ErrRateLimit) {
			log.Printf("⚠️ AI 프로바이더 '%s' 레이트 리밋 초과, 다음 프로바이더로 전환 시도", provider.Name)
			continue
		}

		// 컨텍스트 취소는 즉시 반환
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		// 다른 에러도 다음 프로바이더 시도 (키 오류 등)
		log.Printf("⚠️ AI 프로바이더 '%s' 오류: %v, 다음 프로바이더로 전환 시도", provider.Name, err)
	}

	return nil, fmt.Errorf("모든 AI 프로바이더 실패: %w", lastErr)
}
