package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/vertex/chat-service/internal/config"
)

const (
	dbConnectAttempts = 5
	dbConnectBackoff  = 3 * time.Second
)

// NewDB ต่อฐานข้อมูลพร้อม retry — พอร์ตมาจาก vertex-event-service
// pod มักขึ้นก่อน postgres พร้อมรับ connection
func NewDB(cfg config.DBConfig) (*gorm.DB, error) {
	var (
		db  *gorm.DB
		err error
	)
	for i := 0; i < dbConnectAttempts; i++ {
		db, err = gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{})
		if err == nil {
			return db, nil
		}
		slog.Warn("เชื่อมต่อ DB ไม่สำเร็จ กำลังลองใหม่",
			"attempt", i+1, "max_attempts", dbConnectAttempts,
			"dsn", cfg.Redacted(), "retry_in", dbConnectBackoff, "error", err)
		time.Sleep(dbConnectBackoff)
	}
	return nil, fmt.Errorf("เชื่อมต่อฐานข้อมูลไม่สำเร็จหลังลอง %d ครั้ง: %w", dbConnectAttempts, err)
}

func CloseDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// requiredSchemaVersion คือเวอร์ชัน migration ต่ำสุดที่โค้ดชุดนี้ต้องการ
// ต้องเพิ่มค่านี้ทุกครั้งที่เขียน migration ใหม่ที่โค้ดพึ่งพา (ดู vertex-migrations/README.md)
const requiredSchemaVersion = 1

// AssertSchemaVersion ยืนยันว่า Flyway รันครบก่อนรับ request — ไม่มี AutoMigrate
func AssertSchemaVersion(ctx context.Context, db *gorm.DB) error {
	var version string
	err := db.WithContext(ctx).Raw(
		`SELECT version FROM flyway_schema_history
		 WHERE success AND version IS NOT NULL
		 ORDER BY installed_rank DESC LIMIT 1`).Scan(&version).Error
	if err != nil {
		return fmt.Errorf("อ่าน flyway_schema_history ไม่ได้ (migration รันหรือยัง): %w", err)
	}
	if version == "" {
		return fmt.Errorf("ยังไม่มี migration ที่สำเร็จเลย — รัน Flyway ก่อน")
	}

	var current int
	if _, err := fmt.Sscanf(version, "%d", &current); err != nil {
		return fmt.Errorf("อ่านเลขเวอร์ชัน %q ไม่ได้: %w", version, err)
	}
	if current < requiredSchemaVersion {
		return fmt.Errorf("schema เป็นเวอร์ชัน %d แต่โค้ดชุดนี้ต้องการอย่างน้อย %d",
			current, requiredSchemaVersion)
	}

	slog.Info("ตรวจ schema ผ่าน", "version", current, "required", requiredSchemaVersion)
	return nil
}
