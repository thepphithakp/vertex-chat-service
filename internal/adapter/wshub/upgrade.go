package wshub

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/contrib/websocket"

	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/internal/port"
)

// PeerLister คืนรายชื่อคู่สนทนาของ user คนหนึ่ง — ChatService.ListPeerIDs
// เข้ากับ signature นี้พอดีอยู่แล้ว ไม่ต้อง adapter เพิ่ม
type PeerLister func(ctx context.Context, self domain.UserID) ([]domain.UserID, error)

// Handle เป็น handler ของ websocket.New — เรียกหลังผ่าน pre-upgrade middleware
// แล้ว (ตรวจ Origin + แลก ticket เสร็จแล้ว ได้ userID + jwtExp มาจาก Locals)
//
// อายุของ socket นี้ไม่มีทางยืนกว่า JWT เดิมที่ใช้ขอ ticket มา — ถ้า socket
// เปิดค้างไว้นานกว่า JWT เดิมจะหมดอายุ ต้องปิดแล้วให้ client ไปขอ ticket ใหม่
// (ซึ่ง re-validate JWT อีกรอบ) ไม่ใช่ปล่อยให้ socket ที่เปิดจาก credential
// ที่หมดอายุไปแล้วยังใช้งานได้ต่อ
func Handle(h *Hub, userID domain.UserID, jwtExp time.Time, peersOf PeerLister) func(*websocket.Conn) {
	return func(ws *websocket.Conn) {
		c := h.register(userID, ws)
		defer h.unregister(c)

		_ = ws.SetReadDeadline(time.Now().Add(readDeadline))
		ws.SetPongHandler(func(string) error {
			return ws.SetReadDeadline(time.Now().Add(readDeadline))
		})

		if err := c.writeJSON(port.ReadyFrame{
			T: "ready", UserID: string(userID), ServerTimeMs: time.Now().UnixMilli(),
		}); err != nil {
			return
		}

		// sync สถานะ online ปัจจุบันให้ client ที่เพิ่งต่อสาย — presence frame
		// ปกติบอกแค่ "เปลี่ยนแปลง" ไม่งั้น client จะไม่รู้เลยว่าใคร online
		// อยู่ก่อนหน้าจนกว่าจะมีการเปลี่ยนแปลงเกิดขึ้นจริงหลังจากนี้ ยิงครั้งเดียว
		// ตอนต่อสายสำเร็จพอ ไม่ต้อง poll ซ้ำ เพราะหลังจากนี้ transition ทุกอัน
		// จะมาทาง presence frame ตามปกติ
		if peersOf != nil {
			if peers, err := peersOf(context.Background(), userID); err != nil {
				slog.Warn("sync สถานะ online เริ่มต้นไม่ได้ ไม่ร้ายแรง — รอ presence frame ปกติแทน",
					"user_id", userID, "error", err)
			} else {
				for _, peer := range peers {
					if h.IsOnline(peer) {
						_ = c.writeJSON(port.PresenceFrame{T: "presence", UserID: string(peer), Online: true})
					}
				}
			}
		}

		stop := make(chan struct{})
		defer close(stop)
		go pingLoop(c, jwtExp, stop)

		// read loop — frame เดียวที่มีความหมายคือ client.visibility (ดู
		// client_frame.go) นอกนั้นทิ้งเงียบๆ (parseClientFrame คืน nil)
		// ยังต้องอ่านวนอยู่ดีเพื่อให้ ReadDeadline/PongHandler ทำงาน และรู้ตัว
		// ตอน client ปิด
		for {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if f := parseClientFrame(data); f != nil {
				h.setForeground(c, !f.Hidden)
			}
		}
	}
}

func pingLoop(c *conn, jwtExp time.Time, stop <-chan struct{}) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-time.After(time.Until(jwtExp)):
			slog.Info("ปิด socket เพราะ JWT เดิมหมดอายุแล้ว — client ต้องขอ ticket ใหม่",
				"user_id", c.userID, "socket_id", c.socketID)
			_ = c.ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(4001, "token_expired"), time.Now().Add(5*time.Second))
			_ = c.ws.Close()
			return
		case <-ticker.C:
			c.writeMu.Lock()
			err := c.ws.WriteMessage(websocket.PingMessage, nil)
			c.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}
