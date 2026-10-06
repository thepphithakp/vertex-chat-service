package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vertex/chat-service/internal/domain"
)

type readRow struct {
	ConversationID uuid.UUID `gorm:"type:uuid;primaryKey;column:conversation_id"`
	UserID         string    `gorm:"primaryKey;column:user_id"`
	ReadThroughAt  time.Time `gorm:"column:read_through_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (readRow) TableName() string { return "conversation_reads" }

type GORMReadStateRepository struct {
	db *gorm.DB
}

func NewGORMReadStateRepository(db *gorm.DB) *GORMReadStateRepository {
	return &GORMReadStateRepository{db: db}
}

// MarkRead เป็น monotonic ด้วย GREATEST — ผู้ใช้คนเดียวเขียนแถวของตัวเอง
// เท่านั้น จึงไม่มี concurrent writer สองคนมาชนกันที่แถวเดียวกันเลย
// (ต่างจาก messages ที่ sender_id ต่างกันได้) replay หรือ out-of-order
// read receipt ที่มาถึงไม่ตามลำดับจึงไม่ทำให้ read_through_at ถอยหลัง
func (r *GORMReadStateRepository) MarkRead(ctx context.Context, convID domain.ConversationID, user domain.UserID, through time.Time) error {
	now := time.Now()
	row := readRow{
		ConversationID: uuid.UUID(convID), UserID: string(user),
		ReadThroughAt: through, UpdatedAt: now,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"read_through_at": gorm.Expr("GREATEST(conversation_reads.read_through_at, EXCLUDED.read_through_at)"),
			"updated_at":      now,
		}),
	}).Create(&row).Error
}
