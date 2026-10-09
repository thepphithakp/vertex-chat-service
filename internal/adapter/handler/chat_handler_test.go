package handler

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/vertex/chat-service/internal/application"
	"github.com/vertex/chat-service/internal/domain"
)

// newErrDispatchApp โยง error ที่ส่งเข้ามาตรงเข้า handleUseCaseError เลย
// ไม่ต้องสร้าง ChatHandler/ChatUseCase เต็มตัว เพราะ handleUseCaseError เป็น
// pure dispatch function ที่ไม่ขึ้นกับ use case จริง
func newErrDispatchApp(errToDispatch error) *fiber.App {
	app := fiber.New()
	app.Get("/x", func(c *fiber.Ctx) error {
		return handleUseCaseError(c, errToDispatch)
	})
	return app
}

// TestHandleUseCaseError_ConversationNotFoundBecomes404 pin บั๊กที่เจอจาก
// architecture audit: เดิม repository คืน error type ของตัวเอง
// (*repository.NotFoundError) ซึ่ง handleUseCaseError ไม่รู้จัก ตกไปที่ 500
// เสมอ แม้ที่จริงคือ "ไม่พบบทสนทนา" ซึ่งควรเป็น 404
func TestHandleUseCaseError_ConversationNotFoundBecomes404(t *testing.T) {
	app := newErrDispatchApp(domain.ErrConversationNotFound)

	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d ต้องการ 404", resp.StatusCode)
	}
}

// TestHandleUseCaseError_WrappedConversationNotFoundBecomes404 ยืนยันว่าเช็ค
// ด้วย errors.Is ไม่ใช่ == ตรงๆ — error ที่ถูก wrap ด้วย %w ต้องถูกจับได้เหมือนกัน
func TestHandleUseCaseError_WrappedConversationNotFoundBecomes404(t *testing.T) {
	wrapped := errWrap{cause: domain.ErrConversationNotFound}
	app := newErrDispatchApp(wrapped)

	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("status = %d ต้องการ 404", resp.StatusCode)
	}
}

func TestHandleUseCaseError_ValidationBecomes400(t *testing.T) {
	app := newErrDispatchApp(&application.ValidationError{Field: "x", Reason: "ต้องไม่ว่าง"})

	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d ต้องการ 400", resp.StatusCode)
	}
}

func TestHandleUseCaseError_UnknownBecomes500(t *testing.T) {
	app := newErrDispatchApp(errors.New("db ล่ม"))

	resp, err := app.Test(httptest.NewRequest("GET", "/x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d ต้องการ 500", resp.StatusCode)
	}
}

type errWrap struct{ cause error }

func (e errWrap) Error() string { return "wrapped: " + e.cause.Error() }
func (e errWrap) Unwrap() error { return e.cause }
