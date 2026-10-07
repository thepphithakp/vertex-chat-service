package wshub

import "encoding/json"

// clientFrame คือ frame เดียวที่ client ส่งเข้ามาได้ใน v1 — ก่อนหน้านี้ WS
// เป็น receive-only จากฝั่ง client โดยตั้งใจ (ดู port.ServerFrame) นี่คือ
// client→server frame แรกของ protocol นี้ ประกาศเป็น struct เดียวไม่ใช่
// sum type เพราะมีแค่ชนิดเดียวจริงๆ ถ้าจะเพิ่มชนิดที่สองค่อยทำ switch เต็ม
// รูปตอนนั้น ไม่เผื่อ abstraction ล่วงหน้า
//
// ไม่ขึ้นไปถึง port package — application layer ไม่ต้องรู้จัก frame ดิบนี้
// เลย ผลของ frame นี้คือการเปลี่ยน state ข้างใน hub เท่านั้น (ดู
// Hub.setForeground) ไม่มี use case ไหนใน ChatService ต้องอ่าน frame นี้ตรงๆ
type clientFrame struct {
	T      string `json:"t"`
	Hidden bool   `json:"hidden"`
}

// parseClientFrame คืน nil ถ้า parse ไม่ผ่านหรือ t ไม่รู้จัก — ผู้เรียกทิ้ง
// เงียบๆ ได้เลย ไม่มี error channel กลับไปหา client (ไม่มีใครรอ response ของ
// frame ที่ส่งไปอยู่แล้ว) และไม่คุ้มที่จะปิดทั้งสายเพราะ frame เดียวเสีย/schema
// เพี้ยน — unknown t ไว้เผื่อ client รุ่นใหม่ส่งชนิดที่ server รุ่นนี้ยังไม่รู้จัก
func parseClientFrame(data []byte) *clientFrame {
	var f clientFrame
	if json.Unmarshal(data, &f) != nil || f.T != "client.visibility" {
		return nil
	}
	return &f
}
