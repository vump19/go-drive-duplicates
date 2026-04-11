package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go-drive-duplicates/internal/domain/entities"
	"go-drive-duplicates/internal/domain/services"
)

// ClaudeAPIAdapter - Claude API 어댑터
type ClaudeAPIAdapter struct {
	apiKey     string
	model      string
	maxRetries int
	httpClient *http.Client
}

// NewClaudeAPIAdapter creates a new Claude API adapter
func NewClaudeAPIAdapter(apiKey, model string) *ClaudeAPIAdapter {
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}
	return &ClaudeAPIAdapter{
		apiKey:     apiKey,
		model:      model,
		maxRetries: 3,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// claudeRequest - Claude API 요청 구조체
type claudeRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []claudeMessage  `json:"messages"`
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeResponse - Claude API 응답 구조체
type claudeResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Chat sends a message to Claude API and returns the response
func (c *ClaudeAPIAdapter) Chat(ctx context.Context, messages []services.AIMessage, systemPrompt string) (*services.AIResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("CLAUDE_API_KEY가 설정되지 않았습니다")
	}

	// Convert to Claude message format
	claudeMessages := make([]claudeMessage, 0, len(messages))
	for _, msg := range messages {
		claudeMessages = append(claudeMessages, claudeMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	reqBody := claudeRequest{
		Model:     c.model,
		MaxTokens: 4096,
		System:    systemPrompt,
		Messages:  claudeMessages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 2 * time.Second
			log.Printf("⏳ Claude API 재시도 대기 (%d/%d): %v", attempt+1, c.maxRetries, wait)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("HTTP 요청 생성 실패: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := c.httpClient.Do(req)
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
			lastErr = fmt.Errorf("Claude API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Claude API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		var claudeResp claudeResponse
		if err := json.Unmarshal(respBody, &claudeResp); err != nil {
			return nil, fmt.Errorf("응답 파싱 실패: %w", err)
		}

		if claudeResp.Error != nil {
			return nil, fmt.Errorf("Claude API 오류: %s", claudeResp.Error.Message)
		}

		// Extract text content
		var contentText string
		for _, block := range claudeResp.Content {
			if block.Type == "text" {
				contentText += block.Text
			}
		}

		log.Printf("✅ Claude API 응답 수신 (입력: %d 토큰, 출력: %d 토큰)",
			claudeResp.Usage.InputTokens, claudeResp.Usage.OutputTokens)

		// Parse rule suggestions from response
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
	return nil, fmt.Errorf("Claude API 최대 재시도 초과: %w", lastErr)
}

// extractRuleSuggestions parses rule suggestions from AI response text
func extractRuleSuggestions(content string) []entities.RuleSuggestion {
	// Look for JSON block with ruleSuggestions
	re := regexp.MustCompile("(?s)```json\\s*\\n?(\\{[^`]*\"ruleSuggestions\"[^`]*\\})\\s*\\n?```")
	matches := re.FindStringSubmatch(content)

	if len(matches) < 2 {
		// Try without code block markers
		re2 := regexp.MustCompile("(?s)(\\{[^{}]*\"ruleSuggestions\"\\s*:\\s*\\[.*?\\]\\s*\\})")
		matches = re2.FindStringSubmatch(content)
	}

	if len(matches) < 2 {
		return nil
	}

	jsonStr := strings.TrimSpace(matches[1])

	var wrapper struct {
		RuleSuggestions []entities.RuleSuggestion `json:"ruleSuggestions"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err != nil {
		log.Printf("⚠️ 규칙 제안 파싱 실패: %v", err)
		return nil
	}

	return wrapper.RuleSuggestions
}
