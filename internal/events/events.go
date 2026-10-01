// Package events defines archive events without a delivery transport.
package events

import (
	"context"
	"encoding/json"
	"time"
)

const EmailArchived = "email.archived"

// Event is a versioned JSON envelope. Data uses the schema selected by Type.
type Event struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Time    time.Time       `json:"time"`
	Source  string          `json:"source"`
	Data    json.RawMessage `json:"data"`
}

// ArchivedMessage identifies a saved message. It contains no email content.
type ArchivedMessage struct {
	MessageID       int64  `json:"message_id"`
	SourceID        int64  `json:"source_id"`
	SourceMessageID string `json:"source_message_id"`
}

// Publisher sends one event before returning. Implementations must honor ctx
// and be safe for concurrent calls. An error does not undo the archive write.
type Publisher interface {
	Publish(ctx context.Context, event Event) error
}
