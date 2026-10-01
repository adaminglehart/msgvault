package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.kenn.io/msgvault/internal/events"
)

type eventPublishing struct {
	publisher  events.Publisher
	archiveUID string
	timeout    time.Duration
}

// SetEventPublisher enables direct, best-effort events. Call it after schema
// initialization, before the store is used by workers. Sync-scoped views use
// the same publisher. The caller owns the publisher's lifetime.
func (s *Store) SetEventPublisher(ctx context.Context, publisher events.Publisher, timeout time.Duration) error {
	if publisher == nil || timeout <= 0 {
		return errors.New("event publisher and positive timeout are required")
	}
	uid, err := s.ArchiveUIDContext(ctx)
	if err != nil {
		return fmt.Errorf("read event archive identity: %w", err)
	}
	s.withoutSyncScope().eventPublishing = &eventPublishing{publisher: publisher, archiveUID: uid, timeout: timeout}
	return nil
}

func (s *Store) recordEmailInsert(tx *loggedTx, msg *Message) func(int64) {
	if s.withoutSyncScope().eventPublishing == nil || msg.MessageType != "email" || msg.DeletedAt.Valid {
		return nil
	}
	return func(id int64) {
		tx.archivedMessages = append(tx.archivedMessages, events.ArchivedMessage{
			MessageID: id, SourceID: msg.SourceID, SourceMessageID: msg.SourceMessageID,
		})
	}
}

// publishCommittedEvents runs outside the transaction. Neither a failed send
// nor a cancelled request can change a write that has already committed.
func (s *Store) publishCommittedEvents(ctx context.Context, tx *loggedTx) {
	publishing := s.withoutSyncScope().eventPublishing
	if publishing == nil {
		return
	}
	for _, message := range tx.archivedMessages {
		data, err := json.Marshal(message)
		if err != nil {
			slog.Warn("encode archive event failed", "message_id", message.MessageID, "error", err)
			continue
		}
		identity := fmt.Sprintf("%s:%d:%s", publishing.archiveUID, message.SourceID, message.SourceMessageID)
		event := events.Event{
			ID:   fmt.Sprintf("%s:%x", events.EmailArchived, sha256.Sum256([]byte(identity))),
			Type: events.EmailArchived, Version: 1, Time: time.Now().UTC(),
			Source: publishing.archiveUID, Data: data,
		}
		publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishing.timeout)
		err = publishing.publisher.Publish(publishCtx, event)
		cancel()
		if err != nil {
			slog.Warn("archive event publish failed", "event_id", event.ID, "error", err)
		}
	}
}
