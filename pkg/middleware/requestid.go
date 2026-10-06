package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// HeaderRequestID คือชื่อ header ที่ใช้ correlate log ข้าม service
const HeaderRequestID = "X-Request-Id"

// NewRequestID ใช้ค่าที่ผู้เรียกส่งมา ถ้าไม่มีก็สร้างใหม่
//
// ตั้งค่ากลับเข้า request header ด้วย เพื่อให้ handler และ middleware
// ตัวอื่นอ่านผ่าน c.Get(HeaderRequestID) ได้เหมือนกันหมด
func NewRequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		reqID := c.Get(HeaderRequestID)
		if reqID == "" {
			reqID = uuid.NewString()
			c.Request().Header.Set(HeaderRequestID, reqID)
		}
		c.Set(HeaderRequestID, reqID)
		return c.Next()
	}
}
