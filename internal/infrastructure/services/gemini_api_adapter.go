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

// GeminiAPIAdapter - Google Gemini API 어댑터
type GeminiAPIAdapter struct {
	apiKey     string
	model      string
	maxRetries int
	httpClient *http.Client
}

// NewGeminiAPIAdapter creates a new Gemini API adapter
func NewGeminiAPIAdapter(apiKey, model string) *GeminiAPIAdapter {
	if model == "" {
		model = "gemini-2.0-flash"
	}
	return &GeminiAPIAdapter{
		apiKey:     apiKey,
		model:      model,
		maxRetries: 3,
		httpClient: &http.Client{Timeout: 120 * time.Second},
	}
}

// geminiRequest - Gemini API 요청 구조체
type geminiRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"systemInstruction,omitempty"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

// geminiResponse - Gemini API 응답 구조체
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// Chat sends a message to Gemini API and returns the response
func (g *GeminiAPIAdapter) Chat(ctx context.Context, messages []services.AIMessage, systemPrompt string) (*services.AIResponse, error) {
	if g.apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY가 설정되지 않았습니다")
	}

	// Build Gemini request
	reqBody := geminiRequest{}

	// System prompt
	if systemPrompt != "" {
		reqBody.SystemInstruction = &geminiSystemInstruction{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	// Convert messages: Gemini uses "user" and "model" roles
	for _, msg := range messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		reqBody.Contents = append(reqBody.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("요청 직렬화 실패: %w", err)
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", g.model, g.apiKey)

	var lastErr error
	for attempt := 0; attempt < g.maxRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 2 * time.Second
			log.Printf("⏳ Gemini API 재시도 대기 (%d/%d): %v", attempt+1, g.maxRetries, wait)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("HTTP 요청 생성 실패: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := g.httpClient.Do(req)
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
			lastErr = fmt.Errorf("Gemini API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("Gemini API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		var geminiResp geminiResponse
		if err := json.Unmarshal(respBody, &geminiResp); err != nil {
			return nil, fmt.Errorf("응답 파싱 실패: %w", err)
		}

		if geminiResp.Error != nil {
			return nil, fmt.Errorf("Gemini API 오류: %s", geminiResp.Error.Message)
		}

		if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("Gemini API 응답에 내용이 없습니다")
		}

		contentText := geminiResp.Candidates[0].Content.Parts[0].Text

		log.Printf("✅ Gemini API 응답 수신 (입력: %d 토큰, 출력: %d 토큰)",
			geminiResp.UsageMetadata.PromptTokenCount, geminiResp.UsageMetadata.CandidatesTokenCount)

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
	return nil, fmt.Errorf("Gemini API 최대 재시도 초과: %w", lastErr)
}
