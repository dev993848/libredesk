// Package models holds the customer portal DTOs.
package models

import (
	"time"

	"github.com/volatiletech/null/v9"
)

// Contact is the portal user profile.
type Contact struct {
	ID        int    `db:"id" json:"id"`
	FirstName string `db:"first_name" json:"first_name"`
	LastName  string `db:"last_name" json:"last_name"`
	Email     string `db:"email" json:"email"`
	AvatarURL string `db:"avatar_url" json:"avatar_url"`
}

// Status is a conversation status exposed to the customer.
type Status struct {
	Name     string `db:"name" json:"name"`
	Category string `db:"category" json:"category"`
}

// ConversationListItem is a conversation in the customer's ticket list.
type ConversationListItem struct {
	UUID               string    `db:"uuid" json:"uuid"`
	ReferenceNumber    string    `db:"reference_number" json:"reference_number"`
	Subject            string    `db:"subject" json:"subject"`
	Status             Status    `db:"status" json:"status"`
	InboxName          string    `db:"inbox_name" json:"inbox_name"`
	InboxChannel       string    `db:"inbox_channel" json:"inbox_channel"`
	CreatedAt          time.Time `db:"created_at" json:"created_at"`
	LastMessageAt      null.Time `db:"last_message_at" json:"last_message_at"`
	LastMessagePreview string    `db:"last_message_preview" json:"last_message_preview"`
	UnreadCount        int       `db:"unread_count" json:"unread_count"`
	TimeSpentSeconds   int       `db:"time_spent_seconds" json:"time_spent_seconds"`
	Total              int       `db:"total" json:"-"`
}

// Conversation is a single ticket with its public messages.
type Conversation struct {
	UUID             string    `db:"uuid" json:"uuid"`
	ReferenceNumber  string    `db:"reference_number" json:"reference_number"`
	Subject          string    `db:"subject" json:"subject"`
	Status           Status    `db:"status" json:"status"`
	InboxName        string    `db:"inbox_name" json:"inbox_name"`
	InboxChannel     string    `db:"inbox_channel" json:"inbox_channel"`
	CreatedAt        time.Time `db:"created_at" json:"created_at"`
	LastMessageAt    null.Time `db:"last_message_at" json:"last_message_at"`
	ResolvedAt       null.Time `db:"resolved_at" json:"resolved_at"`
	ClosedAt         null.Time `db:"closed_at" json:"closed_at"`
	TimeSpentSeconds int       `db:"time_spent_seconds" json:"time_spent_seconds"`
	Messages         []Message `db:"-" json:"messages"`
}

// Message is a public message shown to the customer.
type Message struct {
	UUID         string       `json:"uuid"`
	CreatedAt    time.Time    `json:"created_at"`
	Content      string       `json:"content"`
	ContentType  string       `json:"content_type"`
	SenderType   string       `json:"sender_type"`
	AuthorName   string       `json:"author_name"`
	AuthorAvatar string       `json:"author_avatar"`
	Attachments  []Attachment `json:"attachments"`
}

// Attachment is a downloadable file attached to a message.
type Attachment struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Size        int    `json:"size"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}
