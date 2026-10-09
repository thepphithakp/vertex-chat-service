package domain

import "errors"

// ErrConversationNotFound คือสิ่งที่ ConversationRepository.Get คืนเมื่อไม่พบ
// บทสนทนา — ประกาศที่นี่ (ไม่ใช่ private type ใน adapter/repository) เพื่อให้
// ชั้น application/handler เช็คได้โดยไม่ต้อง import adapter ตรงๆ ซึ่งผิด
// ทิศทางของ hexagonal (เดิมเป็น *repository.NotFoundError ซึ่งไม่มีใครเช็ค
// เลยนอกจาก error string ธรรมดา ทำให้บทสนทนาที่ไม่มีจริงตอบ 500 แทน 404)
var ErrConversationNotFound = errors.New("conversation ไม่พบ")
