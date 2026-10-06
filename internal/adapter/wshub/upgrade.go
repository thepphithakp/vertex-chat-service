package wshub

import (
	"log/slog"
	"time"

	"github.com/gofiber/contrib/websocket"

	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/internal/port"
)

// Handle เป็น handler ของ websocket.New — เรียกหลังผ่าน pre-upgrade middleware
// แล้ว (ตรวจ Origin + แลก ticket เสร็จแล้ว ได้ userID + jwtExp มาจาก Locals)
//
// อายุของ socket นี้ไม่มีทางยืนกว่า JWT เดิมที่ใช้ขอ ticket มา — ถ้า socket
// เปิดค้างไว้นานกว่า JWT เดิมจะหมดอายุ ต้องปิดแล้วให้ client ไปขอ ticket ใหม่
// (ซึ่ง re-validate JWT อีกรอบ) ไม่ใช่ปล่อยให้ socket ที่เปิดจาก credential
// ที่หมดอายุไปแล้วยังใช้งานได้ต่อ
func Handle(h *Hub, userID domain.UserID, jwtExp time.Time) func(*websocket.Conn) {
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

		stop := make(chan struct{})
		defer close(stop)
		go pingLoop(c, jwtExp, stop)

		// read loop — v1 ไม่มี client frame ที่มีความหมายนอกจาก pong (ซึ่ง
		// SetPongHandler ข้างบนจัดการให้แล้วที่ชั้น protocol) อ่านทิ้งไปเรื่อยๆ
		// เพื่อให้ ReadDeadline/PongHandler ทำงาน และรู้ตัวตอน client ปิด
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
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
