package domain

import (
	"time"

	"github.com/google/uuid"
)

type ConversationID uuid.UUID

type Conversation struct {
	ID          ConversationID
	Pair        Pair
	OriginPetID uuid.UUID
	CreatedAt   time.Time
}

// ConversationSummary คือมุมมองของ user คนหนึ่งต่อบทสนทนาหนึ่งอัน — merge ของ
// สถานะหลายฝั่ง (ข้อความล่าสุด, ยอดที่ยังไม่อ่านของฉัน, อีกฝั่งอ่านถึงไหน)
// ประกอบโดย repository ด้วย query เดียว ไม่ใช่ caller เอาหลายผลมาต่อกันเอง
type ConversationSummary struct {
	Conversation
	Self              UserID
	Peer              UserID
	PeerName          string
	LastMessage       *Message
	UnreadCount       int
	PeerReadThroughAt *time.Time
}
