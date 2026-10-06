package wshub

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/internal/port"
)

const (
	// เดิม 25s/60s คิดถึงแค่ไม่ให้ cloudflared/Envoy ตัด connection ก่อนเวลา
	// (ยังเหลือเวลาเกิน) แต่ไม่ได้คิดถึงอีกด้าน: ค่านี้คือเวลาสูงสุดที่ hub
	// จะยัง "เข้าใจผิด" ว่า client ตายแล้วยังต่ออยู่ — ซึ่งทำให้ SendMessage
	// เห็น reached > 0 แล้วข้าม push fallback ทั้งที่ client (เช่น iOS Safari
	// ที่ถูกพับแอป/ล็อกหน้าจอ) ไม่ได้รับอะไรจริงมาสักพักแล้ว ผลคือข้อความ
	// เงียบหายจนกว่าผู้รับจะเปิดแอปเอง — บั๊กที่เจอจริงจาก log (ไม่มี push
	// attempt ถูกยิงเลยทั้งที่คู่สนทนาไม่ได้เปิดแอปดูอยู่)
	//
	// ลดเหลือ 10s/25s: ยังมีช่องว่างเหลือเฟือก่อนชน infra idle timeout
	// (cloudflared/Envoy default ~5 นาที ไม่มี BackendTrafficPolicy กำหนดเอง)
	// แต่ลดเวลาที่ hub จะ "เข้าใจผิด" ลงจากสูงสุด ~85s เหลือ ~35s
	pingInterval = 10 * time.Second
	// readDeadline ต้องนานกว่า pingInterval พอให้ client ตอบ pong ทัน
	readDeadline = 25 * time.Second
)

// conn คือ socket หนึ่งเส้นของ user หนึ่งคน (คนเดียวเปิดได้หลายแท็บ/อุปกรณ์
// จึงมีได้หลาย conn ต่อหนึ่ง user) เขียนลง conn จากหลาย goroutine ไม่ปลอดภัย
// (fasthttp/websocket ไม่รับประกัน) จึงมี mutex ของตัวเองคุมการเขียน
type conn struct {
	socketID string
	userID   domain.UserID
	ws       *websocket.Conn
	writeMu  sync.Mutex
}

func (c *conn) writeJSON(v any) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.ws.WriteMessage(websocket.TextMessage, payload)
}

// Hub คือ connection registry ในหน่วยความจำของ pod นี้ — v1 รัน replica เดียว
// (ดู values.yaml) จึงไม่ต้องมี fanout ข้าม pod ด้วย Postgres NOTIFY หรือ
// Redis สิ่งที่ scale-out ในอนาคตต้องแก้คือ port.Delivery ตัวเดียว ไม่ใช่
// ไล่แก้ทุกจุดที่เรียกมัน
type Hub struct {
	mu     sync.RWMutex
	byUser map[domain.UserID]map[string]*conn
}

func NewHub() *Hub {
	return &Hub{byUser: make(map[domain.UserID]map[string]*conn)}
}

func (h *Hub) register(userID domain.UserID, ws *websocket.Conn) *conn {
	c := &conn{socketID: uuid.NewString(), userID: userID, ws: ws}

	h.mu.Lock()
	if h.byUser[userID] == nil {
		h.byUser[userID] = make(map[string]*conn)
	}
	h.byUser[userID][c.socketID] = c
	h.mu.Unlock()

	return c
}

func (h *Hub) unregister(c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if socks, ok := h.byUser[c.userID]; ok {
		delete(socks, c.socketID)
		if len(socks) == 0 {
			delete(h.byUser, c.userID)
		}
	}
}

func (h *Hub) Deliver(userID domain.UserID, frame port.ServerFrame) int {
	return h.DeliverExcept(userID, "", frame)
}

func (h *Hub) DeliverExcept(userID domain.UserID, exceptSocket string, frame port.ServerFrame) int {
	h.mu.RLock()
	targets := make([]*conn, 0, len(h.byUser[userID]))
	for id, c := range h.byUser[userID] {
		if id == exceptSocket {
			continue
		}
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	reached := 0
	for _, c := range targets {
		if err := c.writeJSON(frame); err != nil {
			slog.Warn("ส่ง frame ไม่สำเร็จ ปล่อยให้ read loop ของ conn นั้นปิดตัวเอง",
				"user_id", userID, "socket_id", c.socketID, "error", err)
			continue
		}
		reached++
	}
	return reached
}
