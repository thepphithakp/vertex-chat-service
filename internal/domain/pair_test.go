package domain

import "testing"

func TestNewPair_RejectsSelf(t *testing.T) {
	if _, err := NewPair("u1", "u1"); err == nil {
		t.Fatal("ต้อง reject คุยกับตัวเอง")
	}
}

func TestNewPair_RejectsEmpty(t *testing.T) {
	if _, err := NewPair("", "u1"); err == nil {
		t.Fatal("ต้อง reject userId ว่าง")
	}
}

func TestNewPair_CanonicalOrderingIndependentOfInputOrder(t *testing.T) {
	p1, err := NewPair("u2", "u1")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := NewPair("u1", "u2")
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Fatalf("Pair ต้องเหมือนกันไม่ว่า input จะสลับลำดับยังไง ได้ %+v กับ %+v", p1, p2)
	}
	if p1.A() != "u1" || p1.B() != "u2" {
		t.Fatalf("A() ต้องเป็นตัวที่เรียงก่อนเสมอ ได้ A=%s B=%s", p1.A(), p1.B())
	}
}

func TestPair_Other(t *testing.T) {
	p, _ := NewPair("u1", "u2")
	other, ok := p.Other("u1")
	if !ok || other != "u2" {
		t.Fatalf("Other(u1) ต้องได้ u2 ได้ %s ok=%v", other, ok)
	}
	if _, ok := p.Other("u3"); ok {
		t.Fatal("u3 ไม่ใช่สมาชิกของคู่นี้ ต้องได้ ok=false")
	}
}
