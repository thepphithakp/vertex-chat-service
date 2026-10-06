package middleware

import (
	"errors"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// maxBodyLogBytes จำกัดขนาด body ที่เขียนลง log
// เท่ากับค่า default ของ vertex-pet-service เพื่อความสม่ำเสมอข้าม service
const maxBodyLogBytes = 4 << 10 // 4KB

// isListPath บอกว่า path นี้คืน collection ซึ่ง response อาจใหญ่มาก
//
// /conversations (ไม่มี id ต่อท้าย) และ /conversations/:id/messages คืนหลายแถว
// — ไม่ log response body เต็มๆ ของสอง path นี้แม้ตอน error
func isListPath(path string) bool {
	return path == "/api/v1/chat/conversations" || strings.HasSuffix(path, "/messages")
}

// infraPaths คือ endpoint ที่ถูกเรียกโดย k8s ไม่ใช่ผู้ใช้
//
// ไม่ log เพราะ probe ยิงทุกไม่กี่วินาทีตลอดเวลา ถ้า log ด้วยจะกลบ
// access log ของ request จริงจนหาไม่เจอ
var infraPaths = map[string]bool{
	"/livez":  true,
	"/readyz": true,
	"/health": true,
}

// IsInfraPath บอกว่า path นี้เป็นของ infrastructure ไม่ใช่ traffic ของผู้ใช้
func IsInfraPath(path string) bool { return infraPaths[path] }

// SetupLogger ตั้ง global logger เป็น JSON
//
// slog.SetDefault จะพา log ของ stdlib มาออกทาง handler นี้ด้วย
// ทำให้ทุกบรรทัดใน log parse เป็น JSON ได้เหมือนกันหมด
func SetupLogger(level string) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})))
}

// NewAccessLog เขียน access log หนึ่งบรรทัดต่อหนึ่ง request
//
// 🔴 log body เฉพาะตอน error (status >= 400) เท่านั้น ไม่ใช่ทุก request
//
// เดิม (comment เก่า) ตั้งใจไม่ log body เลยเพราะ payload ของ event เป็น
// ข้อมูลของผู้ใช้ กลัวว่า log จะกลายเป็นสำเนาข้อมูลส่วนตัวชุดที่สอง
// ยังคงหลักการนั้นไว้สำหรับ request ที่สำเร็จ — เปลี่ยนเฉพาะตอน error
// ที่ไม่มี body ให้ดูเลยจะสืบสาเหตุไม่ได้ (ปัญหาเดียวกับที่เจอใน VT-69)
//
// ผ่าน maskBody ก่อนเสมอ ตัดค่า token/email/password ออกแม้ตอน error
func NewAccessLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if IsInfraPath(c.Path()) {
			return c.Next()
		}

		start := time.Now()
		err := c.Next()

		// 🔴 อ่าน c.Response().StatusCode() ตรงๆ ไม่พอเมื่อ handler
		// return error object (fiber.NewError) แทนที่จะเรียก
		// c.Status().JSON() เอง — ErrorHandler กลางทำงาน "หลังจาก"
		// middleware chain นี้ unwind ไปแล้ว ไม่ใช่ระหว่าง c.Next()
		// ดูรายละเอียดเหตุผลเต็มที่ vertex-pet-service (โค้ดเดียวกัน)
		status := c.Response().StatusCode()
		if err != nil {
			status = resolveErrStatus(err)
		}

		// endpoint คือ path จริงที่แทน UUID ด้วย :id แล้ว
		//
		// ไม่ใช้ c.Route().Path เพราะ auth middleware ของ event-service
		// ผูกไว้ที่ระดับ group (app.Group("/api/v1/events", serviceToken))
		// พอถูกปฏิเสธก่อนถึง route ย่อย Fiber ยังไม่ resolve ไปถึง route
		// เต็ม — รายละเอียดเดียวกับที่เจอใน pet-service (ดู logging.go
		// ของ pet-service comment เต็ม)
		endpoint := normalizeEndpoint(c.Path())

		attrs := []any{
			slog.String("method", c.Method()),
			slog.String("path", c.Path()),
			slog.String("endpoint", endpoint),
			slog.Int("status", status),
			slog.Duration("latency", time.Since(start)),
			slog.String("request_id", c.Get(HeaderRequestID)),
			slog.String("ip", c.IP()),
		}
		if uid, ok := c.Locals("userId").(string); ok && uid != "" {
			attrs = append(attrs, slog.String("user_id", uid))
		}

		if status >= 400 {
			if b := truncate(maskBody(c.Body()), maxBodyLogBytes); b != "" {
				attrs = append(attrs, slog.String("req_body", b))
			}
			if !isListPath(c.Path()) {
				if b := truncate(maskBody(c.Response().Body()), maxBodyLogBytes); b != "" {
					attrs = append(attrs, slog.String("res_body", b))
				}
			}
		}

		if status >= 500 {
			slog.ErrorContext(c.UserContext(), "http_request", attrs...)
		} else {
			slog.InfoContext(c.UserContext(), "http_request", attrs...)
		}
		return err
	}
}

// resolveErrStatus เดา HTTP status ที่ ErrorHandler จะกำหนดให้ error นี้
// ต้องตรงกับ logic ใน error_handler.go ทุกประการ
func resolveErrStatus(err error) int {
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fe.Code
	}
	return fiber.StatusInternalServerError
}

// uuidSegment จับ UUID มาตรฐาน (8-4-4-4-12 hex) ไม่สนตัวพิมพ์ใหญ่เล็ก
var uuidSegment = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// normalizeEndpoint แทนที่ segment ที่เป็น UUID ด้วย ":id"
// ทำงานอิสระจากการ resolve route ของ Fiber โดยสิ้นเชิง
func normalizeEndpoint(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if uuidSegment.MatchString(p) {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}
