package middleware

import (
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// KeyID คำนวณ kid จากตัว public key เอง (SHA-256 thumbprint ของ DER)
//
// การคำนวณจากตัวคีย์ทำให้ผู้เซ็นและผู้ตรวจได้ค่าเดียวกันเสมอ
// โดยไม่ต้องตกลงชื่อ kid กันล่วงหน้าและไม่มีทางตั้งค่าไม่ตรงกัน
func KeyID(pub *rsa.PublicKey) string {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ParsePublicKeys แยก PEM หลายบล็อกที่ต่อกันออกจากกันแล้ว parse ทีละใบ
//
// รับหลายใบเพื่อให้ rotate key ได้โดยไม่มี downtime — ระหว่างเปลี่ยนผ่าน
// ต้องยอมรับทั้งใบเก่า (สำหรับ token ที่ยังไม่หมดอายุ) และใบใหม่พร้อมกัน
func ParsePublicKeys(raw string) ([]*rsa.PublicKey, error) {
	var keys []*rsa.PublicKey
	rest := []byte(raw)

	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		pub, err := jwt.ParseRSAPublicKeyFromPEM(pem.EncodeToMemory(block))
		if err != nil {
			return nil, fmt.Errorf("parse public key ไม่สำเร็จ: %w", err)
		}
		keys = append(keys, pub)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("ไม่พบ PEM block ที่ใช้ได้เลย")
	}
	return keys, nil
}

// keySet เก็บ public key ทุกใบที่ยอมรับได้ พร้อม index ตาม kid
type keySet struct {
	byKID map[string]*rsa.PublicKey
	all   []jwt.VerificationKey
}

func newKeySet(keys []*rsa.PublicKey) *keySet {
	ks := &keySet{byKID: make(map[string]*rsa.PublicKey, len(keys))}
	for _, k := range keys {
		if k == nil {
			continue
		}
		ks.byKID[KeyID(k)] = k
		ks.all = append(ks.all, k)
	}
	return ks
}

// keyfunc เลือก key ที่จะใช้ตรวจลายเซ็น
//
//   - token ใหม่มี kid → เลือกใบที่ตรงได้ทันที
//   - token เก่าไม่มี kid → คืนทั้งชุดให้ไลบรารีลองทีละใบ
func (ks *keySet) keyfunc(t *jwt.Token) (interface{}, error) {
	if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
	}
	if len(ks.all) == 0 {
		return nil, fmt.Errorf("ไม่ได้ตั้งค่า public key ไว้เลย")
	}

	if kid, ok := t.Header["kid"].(string); ok && kid != "" {
		if key, found := ks.byKID[kid]; found {
			return key, nil
		}
		return nil, fmt.Errorf("ไม่รู้จัก kid %q — ตรวจว่า JWT_PUBLIC_KEYS มีคีย์ใบนี้แล้วหรือยัง", kid)
	}

	return jwt.VerificationKeySet{Keys: ks.all}, nil
}
