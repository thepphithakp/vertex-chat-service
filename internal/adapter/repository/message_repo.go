package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vertex/chat-service/internal/domain"
)

type messageRow struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;column:id"`
	ConversationID uuid.UUID `gorm:"type:uuid;column:conversation_id"`
	SenderID       string    `gorm:"column:sender_id"`
	ClientMsgID    string    `gorm:"column:client_msg_id"`
	Body           string    `gorm:"column:body"`
	CreatedAt      time.Time `gorm:"column:created_at"`
}

func (messageRow) TableName() string { return "messages" }

type GORMMessageRepository struct {
	db *gorm.DB
}

func NewGORMMessageRepository(db *gorm.DB) *GORMMessageRepository {
	return &GORMMessageRepository{db: db}
}

// Append เป็น idempotent ตาม (conversation_id, sender_id, client_msg_id) —
// ส่งซ้ำด้วย clientMsgId เดิม (เช่น retry หลัง reconnect) ได้แถวเดิมกลับมา
// isNew=false แทนที่จะสร้างข้อความซ้ำ
func (r *GORMMessageRepository) Append(ctx context.Context, m domain.Message) (domain.Message, bool, error) {
	row := messageRow{
		ID: uuid.UUID(m.ID), ConversationID: uuid.UUID(m.ConversationID),
		SenderID: string(m.SenderID), ClientMsgID: string(m.ClientMsgID),
		Body: m.Body, CreatedAt: m.CreatedAt,
	}
	tx := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "conversation_id"}, {Name: "sender_id"}, {Name: "client_msg_id"}},
		DoNothing: true,
	}).Create(&row)
	if tx.Error != nil {
		return domain.Message{}, false, tx.Error
	}
	if tx.RowsAffected > 0 {
		return toDomainMessage(row), true, nil
	}

	var existing messageRow
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND sender_id = ? AND client_msg_id = ?",
			row.ConversationID, row.SenderID, row.ClientMsgID).
		First(&existing).Error
	if err != nil {
		return domain.Message{}, false, err
	}
	return toDomainMessage(existing), false, nil
}

// Page คืนข้อความใหม่สุดก่อน (เหมือน feed ของ ListSummaries) — ฝั่ง client
// กลับลำดับเองตอนแสดงผลจากบนลงล่าง
func (r *GORMMessageRepository) Page(ctx context.Context, convID domain.ConversationID, before *time.Time, limit int) ([]domain.Message, error) {
	q := r.db.WithContext(ctx).Where("conversation_id = ?", uuid.UUID(convID))
	if before != nil {
		q = q.Where("created_at < ?", *before)
	}

	var rows []messageRow
	if err := q.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]domain.Message, len(rows))
	for i, row := range rows {
		out[i] = toDomainMessage(row)
	}
	return out, nil
}

func toDomainMessage(row messageRow) domain.Message {
	return domain.Message{
		ID: domain.MessageID(row.ID), ConversationID: domain.ConversationID(row.ConversationID),
		SenderID: domain.UserID(row.SenderID), ClientMsgID: domain.ClientMsgID(row.ClientMsgID),
		Body: row.Body, CreatedAt: row.CreatedAt,
	}
}
