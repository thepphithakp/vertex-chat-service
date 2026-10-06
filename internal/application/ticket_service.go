package application

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/vertex/chat-service/internal/domain"
)

const ticketTTL = 30 * time.Second

// ticket คือสิทธิ์เปิด WebSocket ครั้งเดียว ผูกกับ user ที่ผ่าน JWT มาแล้ว
//
// เหตุผลที่มี ticket แยกจาก JWT: browser ตั้ง custom header บน
// new WebSocket(url) ไม่ได้ จึงส่ง Authorization: Bearer ตอน upgrade ไม่ได้
// ticket จึงเป็นทางเดียวที่เหลือ — ต้องสั้นและใช้ครั้งเดียวเพื่อลดผลถ้าหลุด
// ไปอยู่ใน log (เช่น query string ของ access log)
type ticket struct {
	user   domain.UserID
	expAt  time.Time
	jwtExp time.Time
}

// TicketService ออกและแลก ticket — เก็บในหน่วยความจำล้วน ไม่ลงฐานข้อมูล
// เพราะอายุ 30 วินาทีแลกในโปรเซสเดียวกับที่ออกให้ ไม่มีเหตุผลต้อง persist
// (pod restart ระหว่างออกกับแลกแปลว่า ticket นั้นตาย — client มีทาง retry
// ออกใหม่อยู่แล้วตอน handshake ล้ม)
type TicketService struct {
	mu      sync.Mutex
	tickets map[string]ticket
}

func NewTicketService() *TicketService {
	s := &TicketService{tickets: make(map[string]ticket)}
	go s.janitor()
	return s
}

// Mint ออก ticket ใหม่ให้ user ที่ผ่าน JWT มาแล้ว — jwtExp คือเวลาหมดอายุของ
// JWT เดิม ใช้เป็นเพดานอายุของ socket ที่จะเปิดด้วย ticket นี้ (socket ไม่ควร
// มีอายุยืนกว่า credential ที่ใช้เปิดมันเลย)
func (s *TicketService) Mint(user domain.UserID, jwtExp time.Time) (string, time.Time, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("สร้าง ticket ไม่สำเร็จ: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(raw)
	expAt := time.Now().Add(ticketTTL)

	s.mu.Lock()
	s.tickets[hashTicket(id)] = ticket{user: user, expAt: expAt, jwtExp: jwtExp}
	s.mu.Unlock()

	return id, expAt, nil
}

// Redeem ใช้ ticket ได้ครั้งเดียว — ลบออกก่อนตรวจเสมอ กัน replay ตอนมีสอง
// request แลก ticket เดียวกันพร้อมกันพอดี
func (s *TicketService) Redeem(id string) (domain.UserID, time.Time, bool) {
	h := hashTicket(id)

	s.mu.Lock()
	t, ok := s.tickets[h]
	delete(s.tickets, h)
	s.mu.Unlock()

	if !ok || time.Now().After(t.expAt) {
		return "", time.Time{}, false
	}
	return t.user, t.jwtExp, true
}

func (s *TicketService) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, t := range s.tickets {
			if now.After(t.expAt) {
				delete(s.tickets, k)
			}
		}
		s.mu.Unlock()
	}
}

func hashTicket(id string) string {
	sum := sha256.Sum256([]byte(id))
	return string(sum[:])
}
