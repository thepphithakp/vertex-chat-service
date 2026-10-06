package port

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
)

type ConversationRepository interface {
	// EnsureForPair เป็น idempotent — เรียกซ้ำกี่ครั้งก็ได้บทสนทนาเดิม
	// (INSERT ... ON CONFLICT DO NOTHING แล้ว SELECT) กัน race ตอนสองฝั่งเปิด
	// แชทกันพร้อมกันเป๊ะ
	EnsureForPair(ctx context.Context, p domain.Pair, originPetID uuid.UUID) (domain.Conversation, error)
	Get(ctx context.Context, id domain.ConversationID) (domain.Conversation, error)
	// ListSummaries คือ query เดียวที่ join ข้อความล่าสุด + ยอดไม่อ่านของ self +
	// read_through ของอีกฝั่ง + ชื่อที่โชว์ — caller ไม่ต้องมา merge เอง
	ListSummaries(ctx context.Context, self domain.UserID) ([]domain.ConversationSummary, error)
}

type MessageRepository interface {
	// Append คืน created=false พร้อมแถวเดิมถ้าชน client_msg_id เดิม — POST ที่
	// ถูก retry ตอบผลเหมือนเดิมทุกประการและไม่ fanout ซ้ำ
	Append(ctx context.Context, m domain.Message) (created domain.Message, isNew bool, err error)
	Page(ctx context.Context, convID domain.ConversationID, before *time.Time, limit int) ([]domain.Message, error)
}

type ReadStateRepository interface {
	// MarkRead เป็น monotonic (GREATEST) — ส่งค่าเก่ากว่าที่บันทึกไว้แล้วจะไม่ถอยหลัง
	MarkRead(ctx context.Context, convID domain.ConversationID, user domain.UserID, through time.Time) error
}

type ProfileRepository interface {
	// Touch อัปเดตชื่อของ user เอง — เรียกทุก request ที่ยืนยันตัวตนแล้ว แต่ gate
	// ด้วย TTL ในชั้น application ไม่ให้เขียนทุก request จริงๆ
	Touch(ctx context.Context, user domain.UserID, displayName string) error
}

// PetLink ตอบคำถามเดียวที่ chat-service เองตอบไม่ได้: สองคนนี้มีสัตว์เลี้ยง
// ร่วมกันไหม (เจ้าของ/ผู้ดูแล) เรียกแค่ตอนสร้างบทสนทนาใหม่ ไม่ใช่ทุกข้อความ
type PetLink interface {
	SharePet(ctx context.Context, a, b domain.UserID, petID uuid.UUID) (bool, error)
}

// Delivery คือ socket hub มองจาก application layer — คืนจำนวนที่ส่งถึงจริง
// แทนที่จะมี IsOnline แยกต่างหาก กัน check-then-act race และกันมีสองทางถาม
// คำถามเดียวกัน (reached==0 แปลว่า "ไม่มีใครต่ออยู่" ให้ fallback ไป push)
type Delivery interface {
	Deliver(user domain.UserID, frame ServerFrame) (reached int)
	DeliverExcept(user domain.UserID, exceptSocket string, frame ServerFrame) (reached int)
}

// Notifier ยิง push + in-app feed ผ่าน notification-service — รูปแบบเดียวกับ
// notifier.HTTPNotifier ของ ev-service
type Notifier interface {
	Notify(ctx context.Context, user domain.UserID, title, body, tag, url string) error
}

type ChatUseCase interface {
	OpenConversation(ctx context.Context, self domain.UserID, peerID domain.UserID, petID uuid.UUID) (domain.ConversationSummary, error)
	ListConversations(ctx context.Context, self domain.UserID) ([]domain.ConversationSummary, error)
	ListMessages(ctx context.Context, self domain.UserID, convID domain.ConversationID, before *time.Time, limit int) ([]domain.Message, error)
	SendMessage(ctx context.Context, self domain.UserID, convID domain.ConversationID, clientMsgID domain.ClientMsgID, body string) (domain.Message, error)
	MarkRead(ctx context.Context, self domain.UserID, convID domain.ConversationID, through time.Time) error
}
