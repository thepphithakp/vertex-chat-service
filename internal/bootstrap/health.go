package bootstrap

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const dbPingTimeout = 2 * time.Second

// Health แยก liveness ออกจาก readiness — พอร์ตมาจาก vertex-event-service
type Health struct {
	db           *gorm.DB
	shuttingDown atomic.Bool
}

func NewHealth(db *gorm.DB) *Health { return &Health{db: db} }

func (h *Health) BeginShutdown() { h.shuttingDown.Store(true) }

// Liveness จงใจไม่เช็ค dependency ใดๆ — ถ้าเช็ค DB ด้วย พอ DB ล่ม
// k8s จะฆ่าทุก pod พร้อมกันแล้ววนรีสตาร์ตไม่จบ
func (h *Health) Liveness(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}

func (h *Health) Readiness(c *fiber.Ctx) error {
	if h.shuttingDown.Load() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "shutting_down"})
	}
	if h.db == nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "no_database"})
	}

	sqlDB, err := h.db.DB()
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "database_unavailable"})
	}

	ctx, cancel := context.WithTimeout(c.UserContext(), dbPingTimeout)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(fiber.Map{"status": "database_unreachable"})
	}

	return c.JSON(fiber.Map{"status": "ready"})
}
