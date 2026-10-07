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
	// GetDisplayName คืนชื่อที่แสดงของ user ให้ชื่อว่างถ้ายังไม่เคย Touch เลย
	// (ยังไม่เคยยิง request ที่ยืนยันตัวตนผ่าน chat-service มาก่อน) ผู้เรียก
	// ต้อง fallback เองตอนชื่อว่าง
	GetDisplayName(ctx context.Context, user domain.UserID) (string, error)
}

// PetLink ตอบคำถามเดียวที่ chat-service เองตอบไม่ได้: สองคนนี้มีสัตว์เลี้ยง
// ร่วมกันไหม (เจ้าของ/ผู้ดูแล) เรียกแค่ตอนสร้างบทสนทนาใหม่ ไม่ใช่ทุกข้อความ
type PetLink interface {
	SharePet(ctx context.Context, a, b domain.UserID, petID uuid.UUID) (bool, error)
}

// Delivery คือ socket hub มองจาก application layer — reached คือจำนวนที่
// เขียนลง socket สำเร็จ ไม่ได้แปลว่าผู้รับเห็นแบบสด — พิสูจน์แล้วจาก
// production ว่า iOS Safari ที่ถูกพับแอปยังรับ write เข้า OS buffer ได้สำเร็จ
// (reached>0) ทั้งที่หน้าแอปถูก suspend ไม่ได้รับ event อะไรเลยจนกว่าจะเปิด
// แอปเอง จึงต้องมี Reachable แยกต่างหากตอบคำถามที่ SendMessage ต้องการจริงๆ:
// "มีสายไหนของ user นี้ที่เชื่อได้ว่า foreground อยู่ตอนนี้ไหม"
//
// reached กับ Reachable เป็นคนละคำถามโดยตั้งใจ ไม่รวมเป็นค่าเดียว — reached
// ยังมีประโยชน์เป็นตัวเลขดิบ ส่วน Reachable คือตัวตัดสินใจ push fallback
// เพียงอย่างเดียว ไม่ปนความหมายสองอย่างเข้าด้วยกัน
type Delivery interface {
	Deliver(user domain.UserID, frame ServerFrame) (reached int)
	DeliverExcept(user domain.UserID, exceptSocket string, frame ServerFrame) (reached int)

	// Reachable แทนที่ส่วน "reached == 0 แปลว่า fallback ไป push" เดิม —
	// สายเปิดอยู่ (reached>0 ได้) ไม่ได้แปลว่า reachable อีกต่อไป ถ้าทุกสาย
	// ของ user นั้นอยู่ background หรือยังไม่เคยรายงานสถานะเลย ต้องถือว่า
	// ไม่ reachable (fail-safe: push เกินจำเป็นถูกกว่าข้อความเงียบหาย)
	Reachable(user domain.UserID) bool
}

// PresenceNotifier คือทิศทางตรงข้ามของ Delivery — hub (adapter) เรียกเข้ามา
// หา application ตอนจำนวน connection ของ user คนหนึ่งเปลี่ยนจาก 0 เป็น 1
// (online) หรือ 1 เป็น 0 (offline) เท่านั้น ไม่ใช่ทุกครั้งที่ tab เปิด/ปิด
// เพราะ hub เองไม่รู้จัก conversation ของใครเลย (จงใจให้โง่ ไม่พึ่งพา DB)
// ฝั่งที่รู้ว่าต้องแจ้งใครบ้างคือ application ผ่าน ConversationRepository
//
// ตั้งค่าแบบ setter หลังสร้างทั้ง hub และ ChatService เสร็จแล้ว (ดู
// bootstrap/app.go) เพราะสองฝั่งพึ่งพากันเป็นวงกลมที่ระดับค่า (hub เป็น
// Delivery ของ ChatService, ChatService เป็น PresenceNotifier ของ hub)
// ไม่ใช่ที่ระดับ import — ไม่มี cycle จริง
type PresenceNotifier interface {
	NotifyPresenceChange(ctx context.Context, user domain.UserID, online bool)
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
