package middleware

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
)

// ErrorHandler เป็นทางออกเดียวของ error ทุกตัวที่หลุดจาก handler
//
// หน้าที่สำคัญคือ "ไม่ส่งรายละเอียดภายในกลับไปให้ผู้เรียก"
// ข้อความจาก database มักมีชื่อตาราง ชื่อคอลัมน์ และค่าที่ทำให้ล้ม
// ซึ่งช่วยคนที่พยายามโจมตี — เก็บไว้ใน log ฝั่งเราแทน
func ErrorHandler(c *fiber.Ctx, err error) error {
	reqID := c.Get(HeaderRequestID)

	var fe *fiber.Error
	if errors.As(err, &fe) && fe.Code != fiber.StatusInternalServerError {
		// error ที่ตั้งใจให้ผู้เรียกเห็น เช่น 404 route ไม่มีอยู่
		return c.Status(fe.Code).JSON(fiber.Map{
			"error":     fe.Message,
			"requestId": reqID,
		})
	}

	slog.ErrorContext(c.UserContext(), "request ล้มเหลว",
		slog.String("request_id", reqID),
		slog.String("method", c.Method()),
		slog.String("path", c.Path()),
		slog.Any("error", err),
	)

	return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
		"error":     "เกิดข้อผิดพลาดภายในระบบ",
		"requestId": reqID,
	})
}
