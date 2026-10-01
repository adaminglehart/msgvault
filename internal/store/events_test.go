package store_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/msgvault/internal/events"
	"go.kenn.io/msgvault/internal/store"
	"go.kenn.io/msgvault/internal/testutil/storetest"
)

type eventPublisherFunc func(context.Context, events.Event) error

func (f eventPublisherFunc) Publish(ctx context.Context, event events.Event) error {
	return f(ctx, event)
}

func TestArchiveEventsPublishAfterCommit(t *testing.T) {
	f := storetest.New(t)
	f.Store.DB().SetMaxOpenConns(1)
	uid, err := f.Store.ArchiveUID()
	require.NoError(t, err)
	var delivered []events.Event
	publisher := eventPublisherFunc(func(ctx context.Context, event events.Event) error {
		// One connection proves that publishing does not hold the transaction.
		var message events.ArchivedMessage
		require.NoError(t, json.Unmarshal(event.Data, &message))
		var body string
		require.NoError(t, f.Store.DB().QueryRowContext(ctx, f.Store.Rebind(
			`SELECT body_text FROM message_bodies WHERE message_id = ?`), message.MessageID).Scan(&body))
		assert.Equal(t, "saved body", body)
		delivered = append(delivered, event)
		return nil
	})
	require.NoError(t, f.Store.SetEventPublisher(t.Context(), publisher, 3*time.Second))
	message := storetest.NewMessage(f.Source.ID, f.ConvID).WithSourceMessageID("event-email").Build()
	data := &store.MessagePersistData{Message: message, BodyText: sql.NullString{String: "saved body", Valid: true}}
	id, err := f.Store.PersistMessage(data)
	require.NoError(t, err)
	require.Len(t, delivered, 1)
	assert.Equal(t, events.EmailArchived, delivered[0].Type)
	assert.Equal(t, 1, delivered[0].Version)
	assert.Equal(t, uid, delivered[0].Source)
	identity := fmt.Sprintf("%s:%d:event-email", uid, f.Source.ID)
	assert.Equal(t, fmt.Sprintf("email.archived:%x", sha256.Sum256([]byte(identity))), delivered[0].ID)
	assert.False(t, delivered[0].Time.IsZero())
	assert.JSONEq(t, fmt.Sprintf(`{"message_id":%d,"source_id":%d,"source_message_id":"event-email"}`, id, f.Source.ID), string(delivered[0].Data))

	_, err = f.Store.PersistMessage(data)
	require.NoError(t, err)
	assert.Len(t, delivered, 1, "updates do not send another archive event")
}

func TestArchiveEventsIgnoreRollbackAndNonEmail(t *testing.T) {
	f := storetest.New(t)
	var delivered []events.Event
	require.NoError(t, f.Store.SetEventPublisher(t.Context(), eventPublisherFunc(func(_ context.Context, event events.Event) error {
		delivered = append(delivered, event)
		return nil
	}), time.Second))
	message := storetest.NewMessage(f.Source.ID, f.ConvID).WithSourceMessageID("rollback-email").Build()
	_, err := f.Store.PersistMessage(&store.MessagePersistData{Message: message, LabelIDs: []int64{999999}})
	require.Error(t, err)
	assert.Empty(t, delivered)
	message.MessageType = "chat"
	_, err = f.Store.PersistMessage(&store.MessagePersistData{Message: message})
	require.NoError(t, err)
	assert.Empty(t, delivered)
}

func TestArchiveEventsPublishFailureDoesNotFailWrite(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprintf("timeout=%t", timeout), func(t *testing.T) {
			f := storetest.New(t)
			require.NoError(t, f.Store.SetEventPublisher(t.Context(), eventPublisherFunc(func(ctx context.Context, _ events.Event) error {
				_, hasDeadline := ctx.Deadline()
				assert.True(t, hasDeadline)
				if timeout {
					<-ctx.Done()
					return ctx.Err()
				}
				return errors.New("test publisher unavailable")
			}), time.Millisecond))
			message := storetest.NewMessage(f.Source.ID, f.ConvID).WithSourceMessageID("failed-send").Build()
			id, err := f.Store.PersistMessage(&store.MessagePersistData{Message: message})
			require.NoError(t, err)
			assert.Positive(t, id)
			found, err := f.Store.MessageExistsBatch(f.Source.ID, []string{"failed-send"})
			require.NoError(t, err)
			assert.Len(t, found, 1)
		})
	}
}

func TestArchiveEventsDoNotBackfillExistingEmail(t *testing.T) {
	f := storetest.New(t)
	message := storetest.NewMessage(f.Source.ID, f.ConvID).WithSourceMessageID("existing-email").Build()
	_, err := f.Store.UpsertMessage(message)
	require.NoError(t, err)
	calls := 0
	require.NoError(t, f.Store.SetEventPublisher(t.Context(), eventPublisherFunc(func(context.Context, events.Event) error {
		calls++
		return nil
	}), time.Second))
	_, err = f.Store.UpsertMessage(message)
	require.NoError(t, err)
	assert.Zero(t, calls)
}

func TestArchiveEventsConcurrentInsertAndSyncScope(t *testing.T) {
	for _, syncScoped := range []bool{false, true} {
		t.Run(fmt.Sprintf("sync_scoped=%t", syncScoped), func(t *testing.T) {
			f := storetest.New(t)
			var mu sync.Mutex
			var delivered []events.Event
			require.NoError(t, f.Store.SetEventPublisher(t.Context(), eventPublisherFunc(func(_ context.Context, event events.Event) error {
				mu.Lock()
				defer mu.Unlock()
				delivered = append(delivered, event)
				return nil
			}), time.Second))
			writer := f.Store
			if syncScoped {
				runID, err := f.Store.StartSync(f.Source.ID, "full")
				require.NoError(t, err)
				writer = f.Store.ScopedToSync(f.Source.ID, runID)
			}
			start := make(chan struct{})
			results := make(chan error, 4)
			for range 4 {
				go func() {
					<-start
					message := storetest.NewMessage(f.Source.ID, f.ConvID).WithSourceMessageID("concurrent-email").Build()
					_, err := writer.PersistMessage(&store.MessagePersistData{Message: message})
					results <- err
				}()
			}
			close(start)
			for range 4 {
				assert.NoError(t, <-results)
			}
			assert.Len(t, delivered, 1)
		})
	}
}
