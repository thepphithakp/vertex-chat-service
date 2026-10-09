package bootstrap

import (
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"

	"github.com/vertex/chat-service/internal/adapter/handler"
	"github.com/vertex/chat-service/internal/adapter/notifier"
	"github.com/vertex/chat-service/internal/adapter/petlink"
	"github.com/vertex/chat-service/internal/adapter/repository"
	"github.com/vertex/chat-service/internal/adapter/wshub"
	"github.com/vertex/chat-service/internal/application"
	"github.com/vertex/chat-service/internal/config"
	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/pkg/middleware"
)

const bodyLimit = 128 << 10 // 128KB — ข้อความยาวสุด 4000 ตัวอักษร ยังเหลือเผื่อเยอะ

// NewApp ประกอบ HTTP layer + WebSocket hub ทั้งหมด
func NewApp(db *gorm.DB, cfg config.Config, auth middleware.AuthConfig) (*fiber.App, *Health) {
	app := fiber.New(fiber.Config{
		BodyLimit:             bodyLimit,
		ErrorHandler:          middleware.ErrorHandler,
		DisableStartupMessage: true,
	})

	app.Use(recover.New())
	app.Use(middleware.NewRequestID())
	app.Use(middleware.NewMetrics())
	app.Use(middleware.NewAccessLog())
	app.Use(cors.New())

	health := NewHealth(db)
	app.Get("/livez", health.Liveness)
	app.Get("/readyz", health.Readiness)
	app.Get("/health", health.Liveness)
	app.Get("/metrics", middleware.MetricsHandler())

	hub := wshub.NewHub()

	conversations := repository.NewGORMConversationRepository(db)
	messages := repository.NewGORMMessageRepository(db)
	reads := repository.NewGORMReadStateRepository(db)
	profiles := repository.NewGORMProfileRepository(db)
	petLink := petlink.NewHTTPPetLink(cfg.PetService.ServiceURL, cfg.PetService.Token)
	notify := notifier.NewHTTPNotifier(cfg.Notification.ServiceURL, cfg.Notification.Token)

	svc := application.NewChatService(conversations, messages, reads, profiles, petLink, hub, notify)
	hub.SetPresenceNotifier(svc) // circular ที่ระดับค่า ไม่ใช่ import — ดู hub.go
	tickets := application.NewTicketService()

	chatH := handler.NewChatHandler(svc)
	wsH := handler.NewWSHandler(tickets, cfg.WS.AllowedOrigins, cfg.WS.PublicBaseURL)

	authed := app.Group("/api/v1/chat", middleware.NewAuth(auth))
	chatH.RegisterRoutes(authed)
	authed.Post("/ws-ticket", wsH.MintTicket)

	// WS upgrade แยก group ต่างหาก ใช้ ticket แทน JWT middleware
	//
	// ⚠️ ห้ามตั้งชื่อ path ที่ขึ้นต้นด้วย "/api/v1/chat" (เช่น "/api/v1/chatws"
	// หรือ "/api/v1/chat-ws") — Fiber ผูก middleware ของ group ด้วย
	// string-prefix match ไม่ใช่ตาม path segment "/api/v1/chatws" ขึ้นต้น
	// ด้วยตัวอักษรเดียวกับ "/api/v1/chat" ทุกตัว จึงโดน JWT middleware ของ
	// REST group ด้านบนครอบไปด้วย เจอบั๊กเดียวกันนี้มาแล้วจริงตอนทำ
	// notification-service (ดู vertex-notification-service bootstrap/app.go)
	// "/api/v1/realtime/chat" ปลอดภัยเพราะอักษรตัวที่ 9 ต่างกัน (r ≠ c)
	app.Use("/api/v1/realtime/chat", wsH.PreUpgrade)
	app.Get("/api/v1/realtime/chat", websocket.New(func(ws *websocket.Conn) {
		userID, _ := ws.Locals("chatUserID").(domain.UserID)
		jwtExp, _ := ws.Locals("chatJWTExp").(time.Time)
		wshub.Handle(hub, userID, jwtExp, svc.ListPeerIDs)(ws)
	}))

	return app, health
}
