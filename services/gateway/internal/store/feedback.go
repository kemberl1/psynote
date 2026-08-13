// Package store — отзывы врачей на сгенерированные дневники.
package store

import (
	"context"
	"time"
)

// GenerationFeedback is one doctor's rating/comment on a generation.
type GenerationFeedback struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	DoctorID  string    `json:"doctor_id"`
	Rating    int       `json:"rating"`
	Comment   string    `json:"comment"`
	Quote     string    `json:"quote"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AdminFeedbackItem is a feedback row for the admin inbox (joins doctor + diary).
type AdminFeedbackItem struct {
	GenerationFeedback
	DoctorEmail  string `json:"doctor_email"`
	DoctorName   string `json:"doctor_name"`
	TitleSafe    string `json:"title_safe"`
	DocumentType string `json:"document_type"`
}

// FeedbackRepository persists generation ratings and comments.
type FeedbackRepository interface {
	UpsertFeedback(ctx context.Context, rec GenerationFeedback) (*GenerationFeedback, error)
	GetFeedback(ctx context.Context, requestID, doctorID string) (*GenerationFeedback, error)
	ListFeedback(ctx context.Context, limit, offset int) ([]AdminFeedbackItem, int, error)
}
