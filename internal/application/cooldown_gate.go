package application

import (
	"sync"
	"time"
)

// cooldownGate จำกัดไม่ให้ key เดียวกัน "ผ่าน" ถี่กว่า window ที่กำหนด —
// ใช้กันยิง push ซ้ำ ไม่ใช่กลไกที่ต้องถูกต้องเป๊ะข้ามการ restart
type cooldownGate struct {
	mu     sync.Mutex
	window time.Duration
	last   map[string]time.Time
}

func newCooldownGate(window time.Duration) *cooldownGate {
	return &cooldownGate{window: window, last: make(map[string]time.Time)}
}

func (g *cooldownGate) allow(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	if t, ok := g.last[key]; ok && now.Sub(t) < g.window {
		return false
	}
	g.last[key] = now

	// กันแผนที่โตไม่จำกัด — ล้าง entry เก่ากว่า window*10 แบบขี้เกียจตอนมีการเรียก
	if len(g.last) > 1000 {
		for k, t := range g.last {
			if now.Sub(t) > g.window*10 {
				delete(g.last, k)
			}
		}
	}
	return true
}
