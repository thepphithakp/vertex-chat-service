package middleware

import (
	"encoding/json"
	"strings"
)

const maskedValue = "[HIDDEN]"

// sensitiveFields คือ denylist ของฟิลด์ที่ไม่ยอมให้หลุดเข้า log
//
// event-service เป็น event bus กลางที่รับ payload จากหลาย domain
// event.data จึงมีโครงสร้างที่คาดเดาล่วงหน้าไม่ได้ทั้งหมด — denylist นี้
// จึงเดินโครงสร้าง JSON จริงแบบ recursive (ไม่ใช่ regex ที่ระดับบนสุด
// อย่างเดียว) เพื่อจับได้แม้ฟิลด์ที่ต้องปิดซ้อนอยู่ใน event.data ชั้นใน
var sensitiveFields = []string{
	"token", "accesstoken", "access_token", "refreshtoken", "refresh_token",
	"secret", "authorization", "password", "passwordhash", "password_hash",
	"email", "avatardata", "avatar_data",
}

// maskBody ซ่อนค่าของฟิลด์ที่อยู่ใน denylist
//
// พอร์ตมาจาก vertex-pet-service/pkg/middleware/mask.go — logic เดียวกัน
// เพราะยังไม่มี shared library ระหว่าง service (แต่ละตัวเป็นคนละ Go module)
func maskBody(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		// ไม่ใช่ JSON — ไม่รู้โครงสร้าง จึงไม่กล้าปล่อยผ่าน
		return "[ไม่ใช่ JSON]"
	}

	masked, _ := json.Marshal(maskValue(v))
	return string(masked)
}

func maskValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isSensitive(k) {
				out[k] = maskedValue
				continue
			}
			out[k] = maskValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = maskValue(val)
		}
		return out
	default:
		return v
	}
}

func isSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveFields {
		if k == s {
			return true
		}
	}
	return false
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(ตัดทอน)"
}
