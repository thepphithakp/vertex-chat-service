package domain

import "fmt"

// UserID คือ sub จาก JWT — text ธรรมดา ไม่ใช่ FK ข้าม schema (auth อยู่คนละ schema)
type UserID string

// Pair คือคู่ผู้ใช้แบบไม่มีลำดับ บังคับ invariant: a < b, a != b, ไม่ว่าง
//
// การบังคับผ่าน constructor แทนที่จะปล่อยให้ที่ไหนก็สร้าง Pair ได้เอง ทำให้
// "หนึ่งคู่มีได้แค่หนึ่งบทสนทนา" เป็นจริงโดยโครงสร้าง ไม่ใช่กฎที่ต้องจำไว้ทำตาม
// (ซ้ำกับ CHECK + unique index ในฐานข้อมูลอีกชั้น กันกรณีมีทางเขียนอื่นเข้ามา)
type Pair struct {
	a, b UserID
}

func NewPair(x, y UserID) (Pair, error) {
	if x == "" || y == "" {
		return Pair{}, fmt.Errorf("userId ต้องไม่ว่างทั้งคู่")
	}
	if x == y {
		return Pair{}, fmt.Errorf("คุยกับตัวเองไม่ได้")
	}
	if x < y {
		return Pair{a: x, b: y}, nil
	}
	return Pair{a: y, b: x}, nil
}

func (p Pair) A() UserID { return p.a }
func (p Pair) B() UserID { return p.b }

// Other คืนอีกฝั่งของคู่ เทียบกับ self — false ถ้า self ไม่ใช่สมาชิกของคู่นี้
func (p Pair) Other(self UserID) (UserID, bool) {
	switch self {
	case p.a:
		return p.b, true
	case p.b:
		return p.a, true
	default:
		return "", false
	}
}

// Has บอกว่า self เป็นสมาชิกของคู่นี้ไหม
func (p Pair) Has(self UserID) bool {
	return self == p.a || self == p.b
}
