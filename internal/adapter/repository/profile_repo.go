package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/vertex/chat-service/internal/domain"
)

type profileRow struct {
	UserID      string    `gorm:"primaryKey;column:user_id"`
	DisplayName string    `gorm:"column:display_name"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (profileRow) TableName() string { return "user_profiles" }

type GORMProfileRepository struct {
	db *gorm.DB
}

func NewGORMProfileRepository(db *gorm.DB) *GORMProfileRepository {
	return &GORMProfileRepository{db: db}
}

// Touch เป็นเจ้าของแถวเดียว (user เขียนแค่ของตัวเอง) — เรียกจาก request
// ที่ยืนยันตัวตนแล้วของ user คนนั้นเสมอ ไม่มีทางมีคนอื่นเขียนทับชื่อเราได้
func (r *GORMProfileRepository) Touch(ctx context.Context, user domain.UserID, displayName string) error {
	row := profileRow{UserID: string(user), DisplayName: displayName, UpdatedAt: time.Now()}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"display_name", "updated_at"}),
	}).Create(&row).Error
}
