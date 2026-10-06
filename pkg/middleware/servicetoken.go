package middleware

import (
	"crypto/sha256"
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"
)

// HeaderServiceToken คือ header ที่ service ต้นทางใช้ยืนยันตัวตน
const HeaderServiceToken = "X-Service-Token"

// NewServiceToken ปล่อยผ่านเฉพาะผู้เรียกที่ถือ token ที่ตรงกัน
//
// ใช้กับ POST /events ซึ่งถูกเรียกโดย service อื่น ไม่ใช่โดยผู้ใช้
// จึงไม่เหมาะจะใช้ JWT ของผู้ใช้ด้วยสองเหตุผล:
//   - การส่ง event เป็น fire-and-forget และต่อไปจะเป็น outbox ที่ส่งทีหลัง
//     token ของผู้ใช้อาจหมดอายุไปแล้วตอนที่ event ถูกส่งจริง
//   - "ใครทำ" (actorId) เป็นข้อมูลใน event ไม่ใช่ตัวยืนยันสิทธิ์ของผู้เรียก
//
// เทียบแบบ constant-time เพื่อไม่ให้เดา token ทีละตัวอักษรจากเวลาที่ใช้ตอบได้
// และ hash ก่อนเทียบเพื่อให้ความยาวไม่ต่างกันจนเป็นข้อมูลรั่ว
func NewServiceToken(expected string) fiber.Handler {
	want := sha256.Sum256([]byte(expected))

	return func(c *fiber.Ctx) error {
		got := sha256.Sum256([]byte(c.Get(HeaderServiceToken)))
		if subtle.ConstantTimeCompare(want[:], got[:]) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error":     "service token ไม่ถูกต้อง",
				"requestId": c.Get(HeaderRequestID),
			})
		}
		return c.Next()
	}
}
