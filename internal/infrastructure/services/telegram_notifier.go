package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go-drive-duplicates/internal/domain/entities"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// TelegramNotifier implements NotificationService using Telegram Bot API
type TelegramNotifier struct {
	mu       sync.RWMutex
	enabled  bool
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramNotifier creates a new TelegramNotifier
func NewTelegramNotifier(botToken, chatID string, enabled bool) *TelegramNotifier {
	return &TelegramNotifier{
		enabled:  enabled,
		botToken: botToken,
		chatID:   chatID,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// IsEnabled returns whether notifications are currently enabled
func (t *TelegramNotifier) IsEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.enabled && t.botToken != "" && t.chatID != ""
}

// SetEnabled enables or disables notifications at runtime
func (t *TelegramNotifier) SetEnabled(enabled bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.enabled = enabled
}

// UpdateConfig updates the bot token and chat ID at runtime
func (t *TelegramNotifier) UpdateConfig(botToken, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.botToken = botToken
	t.chatID = chatID
}

// UpdateChatID updates only the chat ID at runtime
func (t *TelegramNotifier) UpdateChatID(chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.chatID = chatID
}

// GetConfig returns the current configuration (token masked)
func (t *TelegramNotifier) GetConfig() (enabled bool, botToken, chatID string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	maskedToken := ""
	if t.botToken != "" {
		if len(t.botToken) > 8 {
			maskedToken = t.botToken[:4] + "****" + t.botToken[len(t.botToken)-4:]
		} else {
			maskedToken = "****"
		}
	}
	return t.enabled, maskedToken, t.chatID
}

// NotifyOperationCompleted sends a notification when an operation completes
func (t *TelegramNotifier) NotifyOperationCompleted(ctx context.Context, progress *entities.Progress) error {
	if !t.IsEnabled() {
		return nil
	}

	duration := progress.GetDuration()
	message := fmt.Sprintf(
		"✅ *작업 완료*\n\n"+
			"📋 작업: %s\n"+
			"⏱ 소요 시간: %s\n"+
			"📊 처리: %s / %s 항목\n"+
			"🕐 시작: %s\n"+
			"🕐 완료: %s",
		translateOperationType(progress.OperationType),
		formatDuration(duration),
		formatNumber(progress.ProcessedItems),
		formatNumber(progress.TotalItems),
		progress.StartTime.Format("2006-01-02 15:04:05"),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return t.sendMessage(ctx, message)
}

// NotifyOperationFailed sends a notification when an operation fails
func (t *TelegramNotifier) NotifyOperationFailed(ctx context.Context, progress *entities.Progress) error {
	if !t.IsEnabled() {
		return nil
	}

	duration := progress.GetDuration()
	message := fmt.Sprintf(
		"❌ *작업 실패*\n\n"+
			"📋 작업: %s\n"+
			"⏱ 소요 시간: %s\n"+
			"📊 처리: %s / %s 항목\n"+
			"⚠️ 오류: %s\n"+
			"🕐 시작: %s\n"+
			"🕐 실패: %s",
		translateOperationType(progress.OperationType),
		formatDuration(duration),
		formatNumber(progress.ProcessedItems),
		formatNumber(progress.TotalItems),
		progress.ErrorMessage,
		progress.StartTime.Format("2006-01-02 15:04:05"),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return t.sendMessage(ctx, message)
}

// SendTestMessage sends a test notification
func (t *TelegramNotifier) SendTestMessage(ctx context.Context) error {
	t.mu.RLock()
	token := t.botToken
	chatID := t.chatID
	t.mu.RUnlock()

	if token == "" || chatID == "" {
		return fmt.Errorf("텔레그램 설정이 완료되지 않았습니다 (Bot Token 또는 Chat ID 누락)")
	}

	message := fmt.Sprintf(
		"🔔 *테스트 알림*\n\n"+
			"Go Drive Duplicates 텔레그램 알림이 정상적으로 설정되었습니다\\!\n"+
			"🕐 시간: %s",
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return t.sendMessageWithToken(ctx, token, chatID, message)
}

// sendMessage sends a message using the current configuration
func (t *TelegramNotifier) sendMessage(ctx context.Context, message string) error {
	t.mu.RLock()
	token := t.botToken
	chatID := t.chatID
	t.mu.RUnlock()

	return t.sendMessageWithToken(ctx, token, chatID, message)
}

// sendMessageWithToken sends a message using the specified token and chat ID
func (t *TelegramNotifier) sendMessageWithToken(ctx context.Context, token, chatID, message string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       message,
		"parse_mode": "Markdown",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("메시지 직렬화 실패: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("요청 생성 실패: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("텔레그램 API 호출 실패: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("텔레그램 API 오류 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	log.Printf("📨 텔레그램 알림 전송 완료")
	return nil
}

// translateOperationType converts operation type constants to Korean
func translateOperationType(opType string) string {
	switch opType {
	case entities.OperationFileScan:
		return "파일 스캔"
	case entities.OperationDuplicateSearch:
		return "중복 검색"
	case entities.OperationFolderComparison:
		return "폴더 비교"
	case entities.OperationHashCalculation:
		return "해시 계산"
	case entities.OperationFileCleanup:
		return "파일 정리"
	case entities.OperationValidateDuplicates:
		return "중복 그룹 검증"
	default:
		return opType
	}
}

// formatDuration formats a duration in Korean (e.g., "1시간 30분 12초")
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%d시간 %d분 %d초", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d분 %d초", minutes, seconds)
	}
	return fmt.Sprintf("%d초", seconds)
}

// formatNumber formats a number with comma separators
func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	s := fmt.Sprintf("%d", n)
	result := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}
