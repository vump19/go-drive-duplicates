package entities

import "time"

// ChatMessage - AI 채팅 메시지
type ChatMessage struct {
	ID        int       `json:"id"`
	SessionID string    `json:"sessionId"`
	Role      string    `json:"role"`    // user, assistant
	Content   string    `json:"content"`
	Metadata  string    `json:"metadata,omitempty"` // JSON: 규칙 제안, 함수 호출 등
	CreatedAt time.Time `json:"createdAt"`
}
