package application

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
	"github.com/vertex/chat-service/internal/port"
)

// pushCooldown กันยิง push ซ้ำถี่ๆ ตอนอีกฝั่งส่งข้อความรัวๆ ขณะเราไม่ได้ต่อ
// socket อยู่ — ยอมให้ข้อความแรกของ burst มาถึงเร็ว ที่เหลือรอ badge ในแอป
const pushCooldown = 60 * time.Second

type ChatService struct {
	conversations port.ConversationRepository
	messages      port.MessageRepository
	reads         port.ReadStateRepository
	profiles      port.ProfileRepository
	petLink       port.PetLink
	delivery      port.Delivery
	notifier      port.Notifier
	now           func() time.Time

	// pushGate กันยิง push ซ้ำ — in-process เท่านั้น หายได้ตอน restart แต่
	// ผลคือแค่อาจมี push เกินจำเป็นหนึ่งครั้ง ไม่ใช่ความถูกต้องของข้อมูล
	pushGate *cooldownGate
}

func NewChatService(
	conversations port.ConversationRepository,
	messages port.MessageRepository,
	reads port.ReadStateRepository,
	profiles port.ProfileRepository,
	petLink port.PetLink,
	delivery port.Delivery,
	notifier port.Notifier,
) *ChatService {
	return &ChatService{
		conversations: conversations,
		messages:      messages,
		reads:         reads,
		profiles:      profiles,
		petLink:       petLink,
		delivery:      delivery,
		notifier:      notifier,
		now:           time.Now,
		pushGate:      newCooldownGate(pushCooldown),
	}
}

func (s *ChatService) OpenConversation(ctx context.Context, self, peerID domain.UserID, petID uuid.UUID) (domain.ConversationSummary, error) {
	if petID == uuid.Nil {
		return domain.ConversationSummary{}, &ValidationError{Field: "petId", Reason: "ต้องไม่ว่าง"}
	}
	pair, err := domain.NewPair(self, peerID)
	if err != nil {
		return domain.ConversationSummary{}, &ValidationError{Field: "peerUserId", Reason: err.Error()}
	}

	shared, err := s.petLink.SharePet(ctx, self, peerID, petID)
	if err != nil {
		return domain.ConversationSummary{}, fmt.Errorf("ตรวจสิทธิ์ไม่สำเร็จ: %w", err)
	}
	if !shared {
		return domain.ConversationSummary{}, &ForbiddenError{Reason: "ต้องมีสัตว์เลี้ยงร่วมกันถึงจะแชทกันได้"}
	}

	conv, err := s.conversations.EnsureForPair(ctx, pair, petID)
	if err != nil {
		return domain.ConversationSummary{}, err
	}

	summaries, err := s.conversations.ListSummaries(ctx, self)
	if err != nil {
		return domain.ConversationSummary{}, err
	}
	for _, sum := range summaries {
		if sum.ID == conv.ID {
			return sum, nil
		}
	}
	// บทสนทนาที่เพิ่งสร้าง ยังไม่มีข้อความเลย — ประกอบ summary เปล่าตรงนี้แทน
	// ไม่ต้องยิง query ซ้ำ
	return domain.ConversationSummary{Conversation: conv, Self: self, Peer: peerID}, nil
}

func (s *ChatService) ListConversations(ctx context.Context, self domain.UserID) ([]domain.ConversationSummary, error) {
	return s.conversations.ListSummaries(ctx, self)
}

func (s *ChatService) ListMessages(ctx context.Context, self domain.UserID, convID domain.ConversationID, before *time.Time, limit int) ([]domain.Message, error) {
	if err := s.assertMember(ctx, self, convID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.messages.Page(ctx, convID, before, limit)
}

func (s *ChatService) SendMessage(ctx context.Context, self domain.UserID, convID domain.ConversationID, clientMsgID domain.ClientMsgID, rawBody string) (domain.Message, error) {
	conv, err := s.conversations.Get(ctx, convID)
	if err != nil {
		return domain.Message{}, err
	}
	peer, ok := conv.Pair.Other(self)
	if !ok {
		return domain.Message{}, &ForbiddenError{Reason: "ไม่ใช่สมาชิกของบทสนทนานี้"}
	}
	if clientMsgID == "" {
		return domain.Message{}, &ValidationError{Field: "clientMsgId", Reason: "ต้องไม่ว่าง"}
	}
	body, err := domain.NewMessageBody(rawBody)
	if err != nil {
		return domain.Message{}, &ValidationError{Field: "body", Reason: err.Error()}
	}

	msg, isNew, err := s.messages.Append(ctx, domain.Message{
		ID:             domain.MessageID(uuid.New()),
		ConversationID: convID,
		SenderID:       self,
		ClientMsgID:    clientMsgID,
		Body:           body,
		CreatedAt:      s.now(),
	})
	if err != nil {
		return domain.Message{}, err
	}
	if !isNew {
		// ถูก retry ด้วย clientMsgId เดิม — คืนผลเดิม ไม่ fanout ซ้ำ
		return msg, nil
	}

	frame := port.ChatMessageFrame{T: "chat.message", Message: toWireMessage(msg)}
	s.delivery.DeliverExcept(self, "", frame) // อุปกรณ์อื่นของผู้ส่งเอง (ถ้ามี)
	reached := s.delivery.Deliver(peer, frame)

	if reached == 0 && s.pushGate.allow(string(peer)+":"+convIDStr(convID)) {
		go func() {
			ctx := context.WithoutCancel(ctx)
			// title เดิมเป็น "ข้อความใหม่" เฉยๆ ไม่บอกว่าใครส่ง — ผู้ใช้เปิด
			// แจ้งเตือนมาแล้วไม่รู้ว่าต้องรีบเปิดอ่านไหม จึงดึงชื่อผู้ส่งมาใส่
			// title แทน ชื่อว่างได้ (ยังไม่เคย Touch) จึง fallback เป็น
			// "มีข้อความใหม่" คำเดิมไว้เผื่อ
			senderName, err := s.profiles.GetDisplayName(ctx, self)
			if err != nil {
				slog.ErrorContext(ctx, "หาชื่อผู้ส่งสำหรับ push ไม่สำเร็จ ใช้ชื่อสำรองแทน", "error", err)
			}
			title := "มีข้อความใหม่"
			if senderName != "" {
				title = senderName
			}
			if err := s.notifier.Notify(ctx, peer, title, preview(body, 120),
				"chat-conv-"+convIDStr(convID), "/chat/"+convIDStr(convID)); err != nil {
				slog.ErrorContext(ctx, "ส่ง push แจ้งข้อความใหม่ไม่สำเร็จ", "error", err)
			}
		}()
	}

	return msg, nil
}

func (s *ChatService) MarkRead(ctx context.Context, self domain.UserID, convID domain.ConversationID, through time.Time) error {
	if err := s.assertMember(ctx, self, convID); err != nil {
		return err
	}
	if err := s.reads.MarkRead(ctx, convID, self, through); err != nil {
		return err
	}

	// อ่านสำเร็จแล้ว แค่ fanout ไม่ได้ ไม่ควรทำให้ request นี้ fail จึง log แล้วปล่อยผ่าน
	if conv, err := s.conversations.Get(ctx, convID); err != nil {
		slog.ErrorContext(ctx, "fanout read receipt ไม่ได้: หาบทสนทนาไม่เจอ", "error", err)
	} else if peer, ok := conv.Pair.Other(self); ok {
		s.delivery.Deliver(peer, port.ChatReadFrame{
			T: "chat.read", ConversationID: convIDStr(convID), UserID: string(self),
			ReadThroughAt: through.UTC().Format(time.RFC3339),
		})
	}
	return nil
}

// NotifyPresenceChange ทำให้ ChatService เป็น port.PresenceNotifier ของ hub —
// hub เรียกเข้ามาตอนจำนวน connection ของ user เปลี่ยนจาก 0 เป็น 1 หรือ
// กลับกัน แจ้งเฉพาะคู่สนทนาที่มีอยู่แล้ว (ListSummaries) ไม่มี peer list
// แยกต่างหากให้ดึง จึงยืมคิวรีเดิมที่มีอยู่แล้วแทนที่จะเขียน SQL ใหม่
//
// 🔴 ไม่กัน flicker ตอนสายหลุดๆ ติดๆ (เช่น เน็ตมือถือไม่นิ่ง) ด้วยเจตนา —
//
//	debounce เพิ่มความซับซ้อนที่ยังไม่เห็นว่าจำเป็นจริง ถ้าพบว่ากระพริบถี่
//	จนรำคาญจริงค่อยเพิ่มทีหลัง ไม่ใช่เผื่อไว้ล่วงหน้า
func (s *ChatService) NotifyPresenceChange(ctx context.Context, user domain.UserID, online bool) {
	summaries, err := s.conversations.ListSummaries(ctx, user)
	if err != nil {
		slog.ErrorContext(ctx, "broadcast presence ไม่ได้: หารายชื่อคู่สนทนาไม่สำเร็จ",
			"user_id", user, "error", err)
		return
	}
	frame := port.PresenceFrame{T: "presence", UserID: string(user), Online: online}
	for _, c := range summaries {
		s.delivery.Deliver(c.Peer, frame)
	}
}

func (s *ChatService) assertMember(ctx context.Context, self domain.UserID, convID domain.ConversationID) error {
	conv, err := s.conversations.Get(ctx, convID)
	if err != nil {
		return err
	}
	if !conv.Pair.Has(self) {
		return &ForbiddenError{Reason: "ไม่ใช่สมาชิกของบทสนทนานี้"}
	}
	return nil
}

func convIDStr(id domain.ConversationID) string { return uuid.UUID(id).String() }

func toWireMessage(m domain.Message) port.WireMessage {
	return port.WireMessage{
		ID:             uuid.UUID(m.ID).String(),
		ConversationID: convIDStr(m.ConversationID),
		SenderID:       string(m.SenderID),
		ClientMsgID:    string(m.ClientMsgID),
		Body:           m.Body,
		CreatedAt:      m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func preview(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "…"
}

type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("%s %s", e.Field, e.Reason) }

type ForbiddenError struct{ Reason string }

func (e *ForbiddenError) Error() string { return e.Reason }
