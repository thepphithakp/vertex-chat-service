package handler

import (
	"net/url"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	"github.com/vertex/chat-service/internal/application"
	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/pkg/middleware"
)

type WSHandler struct {
	tickets         *application.TicketService
	allowedOrigins  map[string]bool
	publicWSBaseURL string
}

func NewWSHandler(tickets *application.TicketService, allowedOrigins []string, publicWSBaseURL string) *WSHandler {
	set := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		set[o] = true
	}
	return &WSHandler{tickets: tickets, allowedOrigins: set, publicWSBaseURL: publicWSBaseURL}
}

// MintTicket ออก ticket อายุ 30 วินาทีให้ user ที่ผ่าน JWT มาแล้ว — browser
// ตั้ง custom header บน new WebSocket() ไม่ได้ จึงส่ง Authorization: Bearer
// ตอน upgrade ไม่ได้เลย ticket คือทางที่เหลือ ดู figure-it-out decision trail
func (h *WSHandler) MintTicket(c *fiber.Ctx) error {
	actor, ok := middleware.ActorFrom(c)
	if !ok {
		return unauthorized(c)
	}

	id, expAt, err := h.tickets.Mint(domain.UserID(actor.UserID), actor.Exp)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	wsURL := h.publicWSBaseURL + "?ticket=" + url.QueryEscape(id)
	return c.JSON(fiber.Map{
		"ticket":           id,
		"wsUrl":            wsURL,
		"expiresInSeconds": int(time.Until(expAt).Seconds()),
	})
}

// PreUpgrade ตรวจ Origin + แลก ticket ก่อนยอม upgrade จริง — ต้องอยู่ "ก่อน"
// websocket.New เสมอ เพราะ Origin และ query string อ่านได้เฉพาะตอนเป็น
// Fiber request ธรรมดา ไม่ใช่หลัง upgrade ไปแล้ว
func (h *WSHandler) PreUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.NewError(fiber.StatusUpgradeRequired, "ต้องเป็น WebSocket upgrade request")
	}

	// Origin เป็น header ที่ browser ใส่ให้เองและ JS หน้าเว็บปลอมไม่ได้ —
	// ป้องกัน cross-site WebSocket hijacking (เว็บอื่นเปิด WS มาแอบใช้ cookie/
	// session ของผู้ใช้) ไม่ใช่ตัวยืนยันตัวตน (หน้าที่นั้นเป็นของ ticket)
	origin := c.Get(fiber.HeaderOrigin)
	if !h.allowedOrigins[origin] {
		return fiber.NewError(fiber.StatusForbidden, "origin ไม่ได้รับอนุญาต")
	}

	ticketID := c.Query("ticket")
	if ticketID == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "ต้องมี ticket")
	}
	userID, jwtExp, ok := h.tickets.Redeem(ticketID)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "ticket ไม่ถูกต้องหรือหมดอายุแล้ว")
	}

	c.Locals("chatUserID", userID)
	c.Locals("chatJWTExp", jwtExp)
	return c.Next()
}
