package application

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/internal/port"
)

type fakeConvRepo struct {
	byPair map[domain.Pair]domain.Conversation
	byID   map[domain.ConversationID]domain.Conversation
}

func newFakeConvRepo() *fakeConvRepo {
	return &fakeConvRepo{byPair: map[domain.Pair]domain.Conversation{}, byID: map[domain.ConversationID]domain.Conversation{}}
}

func (r *fakeConvRepo) EnsureForPair(ctx context.Context, p domain.Pair, petID uuid.UUID) (domain.Conversation, error) {
	if c, ok := r.byPair[p]; ok {
		return c, nil
	}
	c := domain.Conversation{ID: domain.ConversationID(uuid.New()), Pair: p, OriginPetID: petID, CreatedAt: time.Now()}
	r.byPair[p] = c
	r.byID[c.ID] = c
	return c, nil
}

func (r *fakeConvRepo) Get(ctx context.Context, id domain.ConversationID) (domain.Conversation, error) {
	c, ok := r.byID[id]
	if !ok {
		return domain.Conversation{}, &NotFoundStub{}
	}
	return c, nil
}

func (r *fakeConvRepo) ListSummaries(ctx context.Context, self domain.UserID) ([]domain.ConversationSummary, error) {
	return nil, nil
}

type NotFoundStub struct{}

func (e *NotFoundStub) Error() string { return "not found" }

type fakeMsgRepo struct {
	byKey map[string]domain.Message
}

func newFakeMsgRepo() *fakeMsgRepo { return &fakeMsgRepo{byKey: map[string]domain.Message{}} }

func (r *fakeMsgRepo) Append(ctx context.Context, m domain.Message) (domain.Message, bool, error) {
	key := string(m.ConversationID[:]) + string(m.SenderID) + string(m.ClientMsgID)
	if existing, ok := r.byKey[key]; ok {
		return existing, false, nil
	}
	r.byKey[key] = m
	return m, true, nil
}

func (r *fakeMsgRepo) Page(ctx context.Context, convID domain.ConversationID, before *time.Time, limit int) ([]domain.Message, error) {
	return nil, nil
}

type fakeReadRepo struct{ marked []domain.UserID }

func (r *fakeReadRepo) MarkRead(ctx context.Context, convID domain.ConversationID, user domain.UserID, through time.Time) error {
	r.marked = append(r.marked, user)
	return nil
}

type fakeProfileRepo struct{ names map[domain.UserID]string }

func (r *fakeProfileRepo) Touch(ctx context.Context, user domain.UserID, displayName string) error {
	return nil
}

func (r *fakeProfileRepo) GetDisplayName(ctx context.Context, user domain.UserID) (string, error) {
	return r.names[user], nil
}

type fakePetLink struct{ shared bool }

func (p *fakePetLink) SharePet(ctx context.Context, a, b domain.UserID, petID uuid.UUID) (bool, error) {
	return p.shared, nil
}

type fakeDelivery struct {
	reached map[domain.UserID]int
	frames  []port.ServerFrame
}

func newFakeDelivery(reached map[domain.UserID]int) *fakeDelivery {
	return &fakeDelivery{reached: reached}
}

func (d *fakeDelivery) Deliver(user domain.UserID, frame port.ServerFrame) int {
	d.frames = append(d.frames, frame)
	return d.reached[user]
}

func (d *fakeDelivery) DeliverExcept(user domain.UserID, exceptSocket string, frame port.ServerFrame) int {
	return d.Deliver(user, frame)
}

// fakeNotifier นับจำนวนครั้งที่ถูกเรียก — ใช้ atomic เพราะ SendMessage ยิง
// push ผ่าน goroutine แยก (fire-and-forget ไม่บล็อก request หลัก) เทสต้อง
// รอผลด้วย waitForCalls แทนที่จะอ่านค่าทันทีหลัง SendMessage คืนค่า
type fakeNotifier struct{ calls atomic.Int32 }

func (n *fakeNotifier) Notify(ctx context.Context, user domain.UserID, title, body, tag, url string) error {
	n.calls.Add(1)
	return nil
}

// waitForCalls รอจนกว่า n.calls ถึง want หรือหมดเวลา — กัน flaky จากการที่
// push ยิงผ่าน goroutine แยกซึ่งไม่มีจังหวะที่แน่นอนว่าทำงานเสร็จเมื่อไหร่
func waitForCalls(t *testing.T, n *fakeNotifier, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if n.calls.Load() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("รอ notifier ถูกเรียก %d ครั้งไม่ทัน ได้ %d ครั้ง", want, n.calls.Load())
}

func newService(t *testing.T, shared bool, reached map[domain.UserID]int) (*ChatService, *fakeConvRepo, *fakeNotifier) {
	t.Helper()
	conv := newFakeConvRepo()
	msg := newFakeMsgRepo()
	notif := &fakeNotifier{}
	svc := NewChatService(conv, msg, &fakeReadRepo{}, &fakeProfileRepo{},
		&fakePetLink{shared: shared}, newFakeDelivery(reached), notif)
	return svc, conv, notif
}

func TestOpenConversation_RejectsWhenNoSharedPet(t *testing.T) {
	svc, _, _ := newService(t, false, nil)
	_, err := svc.OpenConversation(context.Background(), "u1", "u2", uuid.New())
	if err == nil {
		t.Fatal("ต้อง reject ถ้าไม่มีสัตว์เลี้ยงร่วมกัน")
	}
}

func TestOpenConversation_IdempotentForSamePair(t *testing.T) {
	svc, _, _ := newService(t, true, nil)
	petID := uuid.New()
	a, err := svc.OpenConversation(context.Background(), "u1", "u2", petID)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.OpenConversation(context.Background(), "u2", "u1", petID)
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != b.ID {
		t.Fatalf("คู่เดียวกันต้องได้บทสนทนาเดียวกันไม่ว่าใครเปิดก่อน ได้ %v กับ %v", a.ID, b.ID)
	}
}

func TestSendMessage_RetryWithSameClientMsgIDDoesNotDoubleFanout(t *testing.T) {
	svc, conv, notifier := newService(t, true, map[domain.UserID]int{"peer": 0})
	c, err := conv.EnsureForPair(context.Background(), mustPair(t, "self", "peer"), uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.SendMessage(context.Background(), "self", c.ID, "client-1", "hello")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.SendMessage(context.Background(), "self", c.ID, "client-1", "hello")
	if err != nil {
		t.Fatal(err)
	}

	waitForCalls(t, notifier, 1)
}

func TestSendMessage_PushesOnlyWhenPeerNotConnected(t *testing.T) {
	svc, conv, notifier := newService(t, true, map[domain.UserID]int{"peer": 1})
	c, _ := conv.EnsureForPair(context.Background(), mustPair(t, "self", "peer"), uuid.New())

	if _, err := svc.SendMessage(context.Background(), "self", c.ID, "client-1", "hello"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond) // เผื่อ race ถ้า logic เปลี่ยนไปยิง goroutine โดยไม่ตั้งใจ
	if got := notifier.calls.Load(); got != 0 {
		t.Fatalf("peer ต่อ socket อยู่ (reached=1) ไม่ควร push เลย ได้ %d ครั้ง", got)
	}
}

func TestSendMessage_RejectsNonMember(t *testing.T) {
	svc, conv, _ := newService(t, true, nil)
	c, _ := conv.EnsureForPair(context.Background(), mustPair(t, "self", "peer"), uuid.New())

	_, err := svc.SendMessage(context.Background(), "stranger", c.ID, "client-1", "hello")
	if err == nil {
		t.Fatal("ต้อง reject คนที่ไม่ใช่สมาชิกของบทสนทนา")
	}
}

func mustPair(t *testing.T, a, b domain.UserID) domain.Pair {
	t.Helper()
	p, err := domain.NewPair(a, b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
