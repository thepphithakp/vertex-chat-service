package handler

import (
	"time"

	"github.com/google/uuid"

	"github.com/vertex/chat-service/internal/domain"
)

func formatMessageID(id domain.MessageID) string   { return uuid.UUID(id).String() }
func formatConvID(id domain.ConversationID) string { return uuid.UUID(id).String() }

// DTO ทั้งหมดอยู่ที่นี่ — ชนิดของ wire format ไม่รั่วเข้าไปใน domain เลย
// เวลาจะเปลี่ยนรูปแบบ JSON ที่ส่งออก แก้ที่ไฟล์เดียวนี้พอ

type messageDTO struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	SenderID       string `json:"senderId"`
	ClientMsgID    string `json:"clientMsgId"`
	Body           string `json:"body"`
	CreatedAt      string `json:"createdAt"`
}

func toMessageDTO(m domain.Message) messageDTO {
	return messageDTO{
		ID:             formatMessageID(m.ID),
		ConversationID: formatConvID(m.ConversationID),
		SenderID:       string(m.SenderID),
		ClientMsgID:    string(m.ClientMsgID),
		Body:           m.Body,
		CreatedAt:      m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type conversationDTO struct {
	ID                string      `json:"id"`
	PeerUserID        string      `json:"peerUserId"`
	PeerName          string      `json:"peerName"`
	LastMessage       *messageDTO `json:"lastMessage"`
	UnreadCount       int         `json:"unreadCount"`
	PeerReadThroughAt *string     `json:"peerReadThroughAt,omitempty"`
	CreatedAt         string      `json:"createdAt"`
}

func toConversationDTO(s domain.ConversationSummary) conversationDTO {
	dto := conversationDTO{
		ID:          formatConvID(s.ID),
		PeerUserID:  string(s.Peer),
		PeerName:    s.PeerName,
		UnreadCount: s.UnreadCount,
		CreatedAt:   s.CreatedAt.UTC().Format(time.RFC3339),
	}
	if s.LastMessage != nil {
		m := toMessageDTO(*s.LastMessage)
		dto.LastMessage = &m
	}
	if s.PeerReadThroughAt != nil {
		t := s.PeerReadThroughAt.UTC().Format(time.RFC3339)
		dto.PeerReadThroughAt = &t
	}
	return dto
}
