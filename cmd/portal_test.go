package main

import (
	"strings"
	"testing"
	"time"

	pmodels "github.com/abhinavxd/libredesk/internal/portal/models"
)

func TestRenderPortalTranscript(t *testing.T) {
	conv := pmodels.Conversation{
		Subject:          "Cannot pay",
		ReferenceNumber:  "100",
		Status:           pmodels.Status{Name: "Open", Category: "open"},
		InboxName:        "Support",
		CreatedAt:        time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		TimeSpentSeconds: 5400,
		Messages: []pmodels.Message{
			{
				CreatedAt:   time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
				Content:     "Hello, my card is declined",
				ContentType: "text",
				SenderType:  "contact",
				AuthorName:  "Ivan Petrov",
			},
			{
				CreatedAt:   time.Date(2026, 11, 1, 10, 5, 0, 0, time.UTC),
				Content:     "<p>Please use another card</p>",
				ContentType: "html",
				SenderType:  "agent",
				AuthorName:  "System",
				Attachments: []pmodels.Attachment{{Name: "receipt.pdf", Size: 42, URL: "https://example.com/receipt"}},
			},
		},
	}

	out := renderPortalTranscript(conv)
	for _, want := range []string{
		"Subject: Cannot pay",
		"Reference: 100",
		"Total time spent: 1h30m0s",
		"[2026-11-01 10:00] Customer",
		"Hello, my card is declined",
		"[2026-11-01 10:05] System",
		"Please use another card",
		"Attachment: receipt.pdf (42 bytes) https://example.com/receipt",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("transcript does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "<p>") {
		t.Fatalf("transcript still contains HTML tags:\n%s", out)
	}
}
