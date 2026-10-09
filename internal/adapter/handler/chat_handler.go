package handler

import (
	"errors"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/application"
	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/internal/port"
	"github.com/vertex/chat-service/pkg/middleware"
)

type ChatHandler struct {
	useCase port.ChatUseCase
}

func NewChatHandler(useCase port.ChatUseCase) *ChatHandler {
	return &ChatHandler{useCase: useCase}
}

func (h *ChatHandler) RegisterRoutes(r fiber.Router) {
	r.Post("/conversations", h.OpenConversation)
	r.Get("/conversations", h.ListConversations)
	r.Get("/conversations/:id/messages", h.ListMessages)
	r.Post("/conversations/:id/messages", h.SendMessage)
	r.Post("/conversations/:id/read", h.MarkRead)
}

// touchProfile อัปเดตชื่อที่โชว์ของผู้ใช้จาก JWT claim ของเขาเอง — เรียกทุก
// request ที่ยืนยันตัวตนแล้ว เพื่อให้ peer เห็นชื่อล่าสุดเสมอโดยไม่ต้องพึ่ง
// auth-service เลย (ดู rationale ใน vertex-migrations/chat/migration/V1)
//
// ย้าย logic จริงไป ChatService.TouchProfile แล้ว — handler แค่ส่งต่อ
func (h *ChatHandler) touchProfile(c *fiber.Ctx, actor middleware.Actor) {
	h.useCase.TouchProfile(c.UserContext(), domain.UserID(actor.UserID), actor.Name)
}

type openConversationRequest struct {
	PeerUserID string `json:"peerUserId"`
	PetID      string `json:"petId"`
}

func (h *ChatHandler) OpenConversation(c *fiber.Ctx) error {
	actor, ok := middleware.ActorFrom(c)
	if !ok {
		return unauthorized(c)
	}
	h.touchProfile(c, actor)

	var req openConversationRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "อ่าน request body ไม่ได้")
	}
	petID, err := uuid.Parse(req.PetID)
	if err != nil {
		return badRequest(c, "petId ต้องเป็น UUID")
	}

	summary, err := h.useCase.OpenConversation(c.UserContext(), domain.UserID(actor.UserID), domain.UserID(req.PeerUserID), petID)
	if err != nil {
		return handleUseCaseError(c, err)
	}
	return c.JSON(toConversationDTO(summary))
}

func (h *ChatHandler) ListConversations(c *fiber.Ctx) error {
	actor, ok := middleware.ActorFrom(c)
	if !ok {
		return unauthorized(c)
	}
	h.touchProfile(c, actor)

	summaries, err := h.useCase.ListConversations(c.UserContext(), domain.UserID(actor.UserID))
	if err != nil {
		return handleUseCaseError(c, err)
	}

	items := make([]conversationDTO, len(summaries))
	totalUnread := 0
	for i, s := range summaries {
		items[i] = toConversationDTO(s)
		totalUnread += s.UnreadCount
	}
	return c.JSON(fiber.Map{"items": items, "totalUnread": totalUnread})
}

func (h *ChatHandler) ListMessages(c *fiber.Ctx) error {
	actor, ok := middleware.ActorFrom(c)
	if !ok {
		return unauthorized(c)
	}
	h.touchProfile(c, actor)

	convID, err := parseConvID(c)
	if err != nil {
		return badRequest(c, err.Error())
	}

	var before *time.Time
	if raw := c.Query("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return badRequest(c, "before ต้องเป็นรูปแบบ RFC3339")
		}
		before = &t
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			limit = n
		}
	}

	messages, err := h.useCase.ListMessages(c.UserContext(), domain.UserID(actor.UserID), convID, before, limit)
	if err != nil {
		return handleUseCaseError(c, err)
	}

	items := make([]messageDTO, len(messages))
	for i, m := range messages {
		items[i] = toMessageDTO(m)
	}
	return c.JSON(fiber.Map{"items": items})
}

type sendMessageRequest struct {
	ClientMsgID string `json:"clientMsgId"`
	Body        string `json:"body"`
}

func (h *ChatHandler) SendMessage(c *fiber.Ctx) error {
	actor, ok := middleware.ActorFrom(c)
	if !ok {
		return unauthorized(c)
	}
	h.touchProfile(c, actor)

	convID, err := parseConvID(c)
	if err != nil {
		return badRequest(c, err.Error())
	}
	var req sendMessageRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "อ่าน request body ไม่ได้")
	}

	msg, err := h.useCase.SendMessage(c.UserContext(), domain.UserID(actor.UserID), convID,
		domain.ClientMsgID(req.ClientMsgID), req.Body)
	if err != nil {
		return handleUseCaseError(c, err)
	}
	return c.JSON(fiber.Map{"message": toMessageDTO(msg)})
}

type markReadRequest struct {
	ThroughMessageAt string `json:"throughMessageAt"`
}

func (h *ChatHandler) MarkRead(c *fiber.Ctx) error {
	actor, ok := middleware.ActorFrom(c)
	if !ok {
		return unauthorized(c)
	}
	h.touchProfile(c, actor)

	convID, err := parseConvID(c)
	if err != nil {
		return badRequest(c, err.Error())
	}
	var req markReadRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, "อ่าน request body ไม่ได้")
	}
	through, err := time.Parse(time.RFC3339, req.ThroughMessageAt)
	if err != nil {
		return badRequest(c, "throughMessageAt ต้องเป็นรูปแบบ RFC3339")
	}

	if err := h.useCase.MarkRead(c.UserContext(), domain.UserID(actor.UserID), convID, through); err != nil {
		return handleUseCaseError(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func parseConvID(c *fiber.Ctx) (domain.ConversationID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return domain.ConversationID{}, errors.New("conversation id ต้องเป็น UUID")
	}
	return domain.ConversationID(id), nil
}

func handleUseCaseError(c *fiber.Ctx, err error) error {
	var ve *application.ValidationError
	if errors.As(err, &ve) {
		return badRequest(c, ve.Error())
	}
	var fe *application.ForbiddenError
	if errors.As(err, &fe) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": fe.Error(), "requestId": c.Get(middleware.HeaderRequestID),
		})
	}
	// เดิมไม่มี branch นี้ — บทสนทนาที่ไม่มีจริงตกไปที่ 500 ข้างล่างเสมอ
	// (repository คืน error type ของตัวเองที่ไม่มีใครเช็ค) แก้ที่ root cause
	// แล้วด้วยให้ repository คืน domain.ErrConversationNotFound ตรงๆ
	if errors.Is(err, domain.ErrConversationNotFound) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(), "requestId": c.Get(middleware.HeaderRequestID),
		})
	}
	return fiber.NewError(fiber.StatusInternalServerError, err.Error())
}

func badRequest(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"error": msg, "requestId": c.Get(middleware.HeaderRequestID),
	})
}

func unauthorized(c *fiber.Ctx) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
		"error": "ยังไม่ได้ยืนยันตัวตน", "requestId": c.Get(middleware.HeaderRequestID),
	})
}
