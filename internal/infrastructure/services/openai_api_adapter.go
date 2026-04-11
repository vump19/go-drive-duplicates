package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"go-drive-duplicates/internal/domain/services"
)

// OpenAIAPIAdapter - OpenAI(ChatGPT) API 어댑터
type OpenAIAPIAdapter struct {
	apiKey     string
	model      string
	maxRetries int
	httpClient *http.Client
}

// NewOpenAIAPIAdapter creates a new OpenAI API adapter
func NewOpenAIAPIAdapter(apiKey, model string) *OpenAIAPIAdapter {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIAPIAdapter{
		apiKey:     apiKey,
		model:      model,
		maxRetries: 3,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// openaiRequest - OpenAI API 요청 구조체
type openaiRequest struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openaiResponse - OpenAI API 응답 구조체
type openaiResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Chat sends a message to OpenAI API and returns the response
func (o *OpenAIAPIAdapter) Chat(ctx context.Context, messages []services.AIMessage, systemPrompt string) (*services.AIResponse, error) {
	if o.apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY가 설정되지 않았습니다")
	}

	// Build messages: system prompt first, then conversation
	openaiMessages := make([]openaiMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		openaiMessages = append(openaiMessages, openaiMessage{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, msg := range messages {
		openaiMessages = append(openaiMessages, openaiMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	reqBody := openaiRequest{
		Model:    o.model,
		Messages: openaiMessages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < o.maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 2 * time.Second
			log.Printf("⏳ OpenAI API 재시도 대기 (%d/%d): %v", attempt+1, o.maxRetries, wait)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/chat/completions", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("HTTP 요청 생성 실패: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+o.apiKey)

		resp, err := o.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP 요청 실패: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("응답 읽기 실패: %w", err)
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("OpenAI API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("OpenAI API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		var openaiResp openaiResponse
		if err := json.Unmarshal(respBody, &openaiResp); err != nil {
			return nil, fmt.Errorf("응답 파싱 실패: %w", err)
		}

		if openaiResp.Error != nil {
			return nil, fmt.Errorf("OpenAI API 오류: %s", openaiResp.Error.Message)
		}

		if len(openaiResp.Choices) == 0 {
			return nil, fmt.Errorf("OpenAI API 응답에 내용이 없습니다")
		}

		contentText := openaiResp.Choices[0].Message.Content

		log.Printf("✅ OpenAI API 응답 수신 (입력: %d 토큰, 출력: %d 토큰)",
			openaiResp.Usage.PromptTokens, openaiResp.Usage.CompletionTokens)

		suggestions := extractRuleSuggestions(contentText)

		return &services.AIResponse{
			Content:         contentText,
			RuleSuggestions: suggestions,
		}, nil
	}

	// 429 레이트 리밋 에러인 경우 ErrRateLimit 래핑
	if lastErr != nil && strings.Contains(lastErr.Error(), "HTTP 429") {
		return nil, fmt.Errorf("%w: %v", services.ErrRateLimit, lastErr)
	}
	return nil, fmt.Errorf("OpenAI API 최대 재시도 초과: %w", lastErr)
}
