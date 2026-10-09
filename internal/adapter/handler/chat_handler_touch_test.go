package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/pkg/middleware"
)

// fakeChatUseCase บันทึกว่า TouchProfile ถูกเรียกไหม — ใช้ยืนยัน coverage
// ของ touchProfile ข้าม handler ทั้ง 5 ตัว (เดิมขาด ListMessages/MarkRead)
type fakeChatUseCase struct{ touched []domain.UserID }

func (f *fakeChatUseCase) TouchProfile(_ context.Context, self domain.UserID, displayName string) {
	if displayName == "" {
		return
	}
	f.touched = append(f.touched, self)
}
func (f *fakeChatUseCase) OpenConversation(context.Context, domain.UserID, domain.UserID, uuid.UUID) (domain.ConversationSummary, error) {
	return domain.ConversationSummary{}, nil
}
func (f *fakeChatUseCase) ListConversations(context.Context, domain.UserID) ([]domain.ConversationSummary, error) {
	return nil, nil
}
func (f *fakeChatUseCase) ListMessages(context.Context, domain.UserID, domain.ConversationID, *time.Time, int) ([]domain.Message, error) {
	return nil, nil
}
func (f *fakeChatUseCase) SendMessage(context.Context, domain.UserID, domain.ConversationID, domain.ClientMsgID, string) (domain.Message, error) {
	return domain.Message{}, nil
}
func (f *fakeChatUseCase) MarkRead(context.Context, domain.UserID, domain.ConversationID, time.Time) error {
	return nil
}

func withActor(actor middleware.Actor) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("actor", actor)
		return c.Next()
	}
}

func newTouchTestApp(uc *fakeChatUseCase) *fiber.App {
	app := fiber.New()
	app.Use(withActor(middleware.Actor{UserID: "u1", Name: "คุณทดสอบ"}))
	NewChatHandler(uc).RegisterRoutes(app.Group("/"))
	return app
}

// TestTouchProfile_CalledFromListMessagesAndMarkRead pin บั๊กที่เจอจาก
// architecture audit: เดิม handler เรียก touchProfile ไม่ครบ ขาด
// ListMessages กับ MarkRead ทั้งที่ comment เดิมอ้างว่า "เรียกทุก request"
func TestTouchProfile_CalledFromListMessagesAndMarkRead(t *testing.T) {
	uc := &fakeChatUseCase{}
	app := newTouchTestApp(uc)
	convID := uuid.New().String()

	_, _ = app.Test(httptest.NewRequest("GET", "/conversations/"+convID+"/messages", nil))

	markReadReq := httptest.NewRequest("POST", "/conversations/"+convID+"/read",
		strings.NewReader(`{"throughMessageAt":"2026-01-01T00:00:00Z"}`))
	markReadReq.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(markReadReq)

	if len(uc.touched) != 2 {
		t.Fatalf("touchProfile ต้องถูกเรียกจากทั้ง ListMessages และ MarkRead ได้ %d ครั้ง", len(uc.touched))
	}
}
