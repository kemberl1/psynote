// Package store — чат поддержки (врач ↔ разработчик) и админ-инбокс.
package store

import (
	"context"
	"time"
)

// SupportThread is one conversation between a doctor and support.
type SupportThread struct {
	ID                 string    `json:"thread_id"`
	DoctorID           string    `json:"doctor_id"`
	Status             string    `json:"status"`
	LastMessageAt      time.Time `json:"last_message_at"`
	LastMessagePreview string    `json:"last_message_preview"`
	UnreadByAdmin      int       `json:"unread_by_admin"`
	UnreadByUser       int       `json:"unread_by_user"`
	CreatedAt          time.Time `json:"created_at"`
}

// SupportThreadListItem is a row in the admin inbox.
type SupportThreadListItem struct {
	SupportThread
	DoctorEmail string `json:"doctor_email"`
	DoctorName  string `json:"doctor_name"`
}

// SupportMessage is one chat message.
type SupportMessage struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"thread_id"`
	SenderID   string    `json:"sender_id"`
	SenderRole string    `json:"sender_role"` // user | support
	SenderName string    `json:"sender_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

// SupportSummary is a compact unread counter for the admin badge.
type SupportSummary struct {
	UnreadMessages int `json:"unread_messages"`
	UnreadThreads  int `json:"unread_threads"`
}

// SupportRepository persists support threads and messages.
type SupportRepository interface {
	GetThreadByDoctor(ctx context.Context, doctorID string) (*SupportThread, error)
	GetThreadByID(ctx context.Context, threadID string) (*SupportThread, error)
	GetThreadInboxItem(ctx context.Context, threadID string) (*SupportThreadListItem, error)
	GetOrCreateThread(ctx context.Context, doctorID string) (*SupportThread, error)
	ListThreads(ctx context.Context, limit, offset int) ([]SupportThreadListItem, int, error)
	ListMessages(ctx context.Context, threadID string) ([]SupportMessage, error)
	AddMessage(ctx context.Context, threadID, senderID, senderRole, body string) (*SupportMessage, error)
	MarkRead(ctx context.Context, threadID, who string) error
	SupportSummary(ctx context.Context) (*SupportSummary, error)
}
