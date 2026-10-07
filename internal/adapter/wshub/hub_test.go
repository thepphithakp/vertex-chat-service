package wshub

import (
	"testing"
	"time"

	"github.com/vertex/chat-service/internal/domain"
)

func TestReachable_NoConnAtAll(t *testing.T) {
	h := NewHub()
	if h.Reachable("u1") {
		t.Fatal("ไม่มี conn เลย ต้องไม่ reachable")
	}
}

func TestReachable_NeverReportedVisibility(t *testing.T) {
	h := NewHub()
	u := domain.UserID("u1")
	c := &conn{socketID: "s1", userID: u}
	h.byUser[u] = map[string]*conn{"s1": c}

	if h.Reachable(u) {
		t.Fatal("conn เพิ่ง register ยังไม่เคยรายงาน visibility ต้องไม่ reachable (fail-safe default)")
	}
}

func TestReachable_ForegroundFresh(t *testing.T) {
	h := NewHub()
	u := domain.UserID("u1")
	c := &conn{socketID: "s1", userID: u}
	h.byUser[u] = map[string]*conn{"s1": c}

	h.setForeground(c, true)
	if !h.Reachable(u) {
		t.Fatal("รายงาน foreground สดๆ ต้อง reachable")
	}
}

func TestReachable_BackgroundReported(t *testing.T) {
	h := NewHub()
	u := domain.UserID("u1")
	c := &conn{socketID: "s1", userID: u}
	h.byUser[u] = map[string]*conn{"s1": c}

	h.setForeground(c, false)
	if h.Reachable(u) {
		t.Fatal("รายงาน background ต้องไม่ reachable")
	}
}

func TestReachable_StaleForegroundExpires(t *testing.T) {
	h := NewHub()
	u := domain.UserID("u1")
	c := &conn{socketID: "s1", userID: u}
	h.byUser[u] = map[string]*conn{"s1": c}

	h.setForeground(c, true)
	h.mu.Lock()
	c.lastVisibilityAt = time.Now().Add(-foregroundTTL - time.Second)
	h.mu.Unlock()

	if h.Reachable(u) {
		t.Fatal("ป้าย foreground เก่าเกิน foregroundTTL ต้องถือว่าไม่รู้ (ไม่ reachable)")
	}
}

func TestReachable_OneOfSeveralConnsForeground(t *testing.T) {
	h := NewHub()
	u := domain.UserID("u1")
	background := &conn{socketID: "s1", userID: u}
	foreground := &conn{socketID: "s2", userID: u}
	h.byUser[u] = map[string]*conn{"s1": background, "s2": foreground}

	h.setForeground(background, false)
	h.setForeground(foreground, true)

	if !h.Reachable(u) {
		t.Fatal("มีอย่างน้อยหนึ่งสายอยู่ foreground (หลายแท็บ/อุปกรณ์) ต้อง reachable")
	}
}

func TestParseClientFrame_Visibility(t *testing.T) {
	f := parseClientFrame([]byte(`{"t":"client.visibility","hidden":true}`))
	if f == nil || !f.Hidden {
		t.Fatalf("ต้อง parse ผ่านและได้ hidden=true ได้ %+v", f)
	}
}

func TestParseClientFrame_UnknownTypeIgnored(t *testing.T) {
	if f := parseClientFrame([]byte(`{"t":"something-else"}`)); f != nil {
		t.Fatalf("t ไม่รู้จัก ต้องได้ nil ได้ %+v", f)
	}
}

func TestParseClientFrame_InvalidJSONIgnored(t *testing.T) {
	if f := parseClientFrame([]byte(`not json`)); f != nil {
		t.Fatalf("parse ไม่ผ่าน ต้องได้ nil ได้ %+v", f)
	}
}
