package repository

import (
	"context"
	"errors"
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

// GetDisplayName คืนค่าว่างเมื่อยังไม่เคยมีแถว (ErrRecordNotFound) แทนที่จะ
// โยน error ออกไป — ผู้เรียก (push notification title) ต้อง fallback เองอยู่แล้ว
// ไม่ถือเป็นเหตุขัดข้อง
func (r *GORMProfileRepository) GetDisplayName(ctx context.Context, user domain.UserID) (string, error) {
	var row profileRow
	err := r.db.WithContext(ctx).Where("user_id = ?", string(user)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return row.DisplayName, nil
}
