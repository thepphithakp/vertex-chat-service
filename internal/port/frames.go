package port

// ServerFrame คือข้อความที่ยิงลง WebSocket หา client — ชุดเล็กเพราะ WS ใน
// v1 นี้มีหน้าที่เดียวคือ "แจ้งสด" ส่วนการเขียน (ส่งข้อความ/มาร์คอ่าน) เป็น
// REST ปกติ ไม่ผ่าน socket เลย — ดู figure-it-out decision trail
//
// ประกาศเป็น interface ว่าง + marker method เพราะ Go ไม่มี sum type จริง
// ฝั่ง PWA มี type คู่กันเป็น discriminated union ที่ src/lib/chat/protocol.ts
// (เขียนมือ ไม่ใช่ codegen — พื้นผิวเล็กพอที่ความเสี่ยง schema สองฝั่งเพี้ยน
// กันต่ำ ต่างจาก signaling protocol เดิมที่ถูกตัดออกไปพร้อมฟีเจอร์โทร)
type ServerFrame interface {
	FrameType() string
}

type ReadyFrame struct {
	T            string `json:"t"`
	UserID       string `json:"userId"`
	ServerTimeMs int64  `json:"serverTimeMs"`
}

func (ReadyFrame) FrameType() string { return "ready" }

type PingFrame struct {
	T string `json:"t"`
}

func (PingFrame) FrameType() string { return "ping" }

type WireMessage struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	SenderID       string `json:"senderId"`
	ClientMsgID    string `json:"clientMsgId"`
	Body           string `json:"body"`
	CreatedAt      string `json:"createdAt"`
}

type ChatMessageFrame struct {
	T       string      `json:"t"`
	Message WireMessage `json:"message"`
}

func (ChatMessageFrame) FrameType() string { return "chat.message" }

type ChatReadFrame struct {
	T              string `json:"t"`
	ConversationID string `json:"conversationId"`
	UserID         string `json:"userId"`
	ReadThroughAt  string `json:"readThroughAt"`
}

func (ChatReadFrame) FrameType() string { return "chat.read" }

type ErrorFrame struct {
	T       string `json:"t"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (ErrorFrame) FrameType() string { return "error" }
