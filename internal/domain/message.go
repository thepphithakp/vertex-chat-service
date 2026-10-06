package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type MessageID uuid.UUID

// ClientMsgID คือ UUID ที่ client สร้างเอง — คีย์กันข้อความซ้ำตอน retry
type ClientMsgID string

const maxBodyRunes = 4000

type Message struct {
	ID             MessageID
	ConversationID ConversationID
	SenderID       UserID
	ClientMsgID    ClientMsgID
	Body           string
	CreatedAt      time.Time
}

// NewMessageBody ตรวจและ trim ข้อความ — ตรวจที่ domain layer เพราะเป็นกฎของ
// ธุรกิจ (ความยาวข้อความ) ไม่ใช่กฎของ HTTP
func NewMessageBody(raw string) (string, error) {
	body := strings.TrimSpace(raw)
	if body == "" {
		return "", fmt.Errorf("ข้อความต้องไม่ว่าง")
	}
	if n := utf8.RuneCountInString(body); n > maxBodyRunes {
		return "", fmt.Errorf("ข้อความยาวเกิน %d ตัวอักษร (ได้ %d)", maxBodyRunes, n)
	}
	return body, nil
}
