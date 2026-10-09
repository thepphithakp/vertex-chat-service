package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vertex/chat-service/internal/domain"
)

type conversationRow struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;column:id"`
	UserAID     string    `gorm:"column:user_a_id"`
	UserBID     string    `gorm:"column:user_b_id"`
	OriginPetID uuid.UUID `gorm:"type:uuid;column:origin_pet_id"`
	CreatedAt   time.Time `gorm:"column:created_at"`
}

func (conversationRow) TableName() string { return "conversations" }

type GORMConversationRepository struct {
	db *gorm.DB
}

func NewGORMConversationRepository(db *gorm.DB) *GORMConversationRepository {
	return &GORMConversationRepository{db: db}
}

// EnsureForPair idempotent ตามชื่อ — ไม่ว่าจะเป็นคนสร้างแถวใหม่หรือมีอยู่
// แล้ว ต้อง SELECT กลับมาเสมอเพื่อได้ ID จริงของแถวที่มีอยู่ (ถ้าชน conflict
// ID ที่เพิ่ง generate ไว้จะไม่ใช่ตัวจริง)
func (r *GORMConversationRepository) EnsureForPair(ctx context.Context, p domain.Pair, petID uuid.UUID) (domain.Conversation, error) {
	candidate := conversationRow{
		ID: uuid.New(), UserAID: string(p.A()), UserBID: string(p.B()),
		OriginPetID: petID, CreatedAt: time.Now(),
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_a_id"}, {Name: "user_b_id"}},
		DoNothing: true,
	}).Create(&candidate).Error
	if err != nil {
		return domain.Conversation{}, err
	}

	var row conversationRow
	err = r.db.WithContext(ctx).
		Where("user_a_id = ? AND user_b_id = ?", candidate.UserAID, candidate.UserBID).
		First(&row).Error
	if err != nil {
		return domain.Conversation{}, err
	}
	return toDomainConversation(row), nil
}

func (r *GORMConversationRepository) Get(ctx context.Context, id domain.ConversationID) (domain.Conversation, error) {
	var row conversationRow
	err := r.db.WithContext(ctx).Where("id = ?", uuid.UUID(id)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Conversation{}, domain.ErrConversationNotFound
	}
	if err != nil {
		return domain.Conversation{}, err
	}
	return toDomainConversation(row), nil
}

// summaryRow คือผลลัพธ์ดิบของ query รวม — ฟิลด์ที่มาจาก LEFT JOIN เป็น
// pointer เพราะบทสนทนาที่ยังไม่มีข้อความเลย/อีกฝั่งยังไม่เคยอ่านเลยจะเป็น NULL
type summaryRow struct {
	ID                uuid.UUID
	UserAID           string
	UserBID           string
	OriginPetID       uuid.UUID
	CreatedAt         time.Time
	LastMsgID         *uuid.UUID
	LastMsgSender     *string
	LastMsgClientID   *string
	LastMsgBody       *string
	LastMsgCreatedAt  *time.Time
	UnreadCount       int
	PeerReadThroughAt *time.Time
	PeerName          *string
}

// ListSummaries คือ query เดียวที่ตอบทุกอย่างที่หน้ารายการบทสนทนาต้องใช้ —
// ข้อความล่าสุด, ยอดไม่อ่านของ self, อีกฝั่งอ่านถึงไหน, ชื่อที่โชว์
// ประกอบด้วย GORM/application layer ไม่ได้เลย เพราะไม่ใช่เจ้าของ "ข้อความ
// ล่าสุดคืออะไร" ของบทสนทนาแต่ละอัน
const listSummariesSQL = `
SELECT
  c.id, c.user_a_id, c.user_b_id, c.origin_pet_id, c.created_at,
  m.id AS last_msg_id, m.sender_id AS last_msg_sender,
  m.client_msg_id AS last_msg_client_id, m.body AS last_msg_body,
  m.created_at AS last_msg_created_at,
  COALESCE(unread.cnt, 0) AS unread_count,
  peer_read.read_through_at AS peer_read_through_at,
  peer_profile.display_name AS peer_name
FROM conversations c
LEFT JOIN LATERAL (
  SELECT id, sender_id, client_msg_id, body, created_at FROM messages
  WHERE conversation_id = c.id ORDER BY created_at DESC LIMIT 1
) m ON true
LEFT JOIN LATERAL (
  SELECT count(*) AS cnt FROM messages msg
  WHERE msg.conversation_id = c.id
    AND msg.sender_id <> $1
    AND msg.created_at > COALESCE(
      (SELECT read_through_at FROM conversation_reads
       WHERE conversation_id = c.id AND user_id = $1),
      'epoch'::timestamptz)
) unread ON true
LEFT JOIN conversation_reads peer_read
  ON peer_read.conversation_id = c.id
 AND peer_read.user_id = CASE WHEN c.user_a_id = $1 THEN c.user_b_id ELSE c.user_a_id END
LEFT JOIN user_profiles peer_profile
  ON peer_profile.user_id = CASE WHEN c.user_a_id = $1 THEN c.user_b_id ELSE c.user_a_id END
WHERE c.user_a_id = $1 OR c.user_b_id = $1
ORDER BY COALESCE(m.created_at, c.created_at) DESC
`

func (r *GORMConversationRepository) ListSummaries(ctx context.Context, self domain.UserID) ([]domain.ConversationSummary, error) {
	var rows []summaryRow
	if err := r.db.WithContext(ctx).Raw(listSummariesSQL, string(self)).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]domain.ConversationSummary, 0, len(rows))
	for _, row := range rows {
		conv := domain.Conversation{
			ID:          domain.ConversationID(row.ID),
			OriginPetID: row.OriginPetID,
			CreatedAt:   row.CreatedAt,
		}
		pair, err := domain.NewPair(domain.UserID(row.UserAID), domain.UserID(row.UserBID))
		if err != nil {
			continue // ข้อมูลเสีย (ไม่ควรเกิด เพราะ CHECK constraint กันไว้แล้ว) ข้ามแถวนี้ไป
		}
		conv.Pair = pair
		peer, _ := pair.Other(self)

		sum := domain.ConversationSummary{
			Conversation:      conv,
			Self:              self,
			Peer:              peer,
			UnreadCount:       row.UnreadCount,
			PeerReadThroughAt: row.PeerReadThroughAt,
		}
		if row.PeerName != nil {
			sum.PeerName = *row.PeerName
		}
		if row.LastMsgID != nil {
			sum.LastMessage = &domain.Message{
				ID:             domain.MessageID(*row.LastMsgID),
				ConversationID: conv.ID,
				SenderID:       domain.UserID(derefStr(row.LastMsgSender)),
				ClientMsgID:    domain.ClientMsgID(derefStr(row.LastMsgClientID)),
				Body:           derefStr(row.LastMsgBody),
				CreatedAt:      derefTime(row.LastMsgCreatedAt),
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

func toDomainConversation(row conversationRow) domain.Conversation {
	pair, _ := domain.NewPair(domain.UserID(row.UserAID), domain.UserID(row.UserBID))
	return domain.Conversation{
		ID: domain.ConversationID(row.ID), Pair: pair,
		OriginPetID: row.OriginPetID, CreatedAt: row.CreatedAt,
	}
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefTime(p *time.Time) time.Time {
	if p == nil {
		return time.Time{}
	}
	return *p
}
