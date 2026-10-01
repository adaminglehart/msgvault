package nats

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/msgvault/internal/events"
)

func startEventServer(t *testing.T) *server.Server {
	t.Helper()
	s, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { s.Shutdown(); s.WaitForShutdown() })
	go s.Start()
	require.True(t, s.ReadyForConnections(5*time.Second))
	return s
}

func TestPublishJetStreamEvent(t *testing.T) {
	s := startEventServer(t)
	publisher, err := Open(s.ClientURL(), "msgvault", "", time.Second)
	require.NoError(t, err)
	t.Cleanup(publisher.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stream, err := publisher.jetstream.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVENTS", Subjects: []string{"msgvault.>"}, Storage: jetstream.FileStorage,
	})
	require.NoError(t, err)
	event := events.Event{
		ID: "archive:email.archived:1", Type: events.EmailArchived, Version: 1,
		Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Source: "archive",
		Data: json.RawMessage(`{"message_id":1,"source_id":2,"source_message_id":"email-1"}`),
	}
	require.NoError(t, publisher.Publish(ctx, event))
	require.NoError(t, publisher.Publish(ctx, event))
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint64(1), info.State.Msgs, "the stable event ID suppresses duplicates")
	message, err := stream.GetMsg(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "msgvault.email.archived", message.Subject)
	var got events.Event
	require.NoError(t, json.Unmarshal(message.Data, &got))
	assert.Equal(t, event, got)
}

func TestPublishFailsWithoutStreamOrConnection(t *testing.T) {
	s := startEventServer(t)
	publisher, err := Open(s.ClientURL(), "msgvault", "", time.Second)
	require.NoError(t, err)
	t.Cleanup(publisher.Close)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	event := events.Event{ID: "missing-stream", Type: events.EmailArchived, Data: json.RawMessage(`{}`)}
	require.Error(t, publisher.Publish(ctx, event))
	publisher.Close()
	require.Error(t, publisher.Publish(ctx, event))
}

func TestReconnectAfterInitialFailure(t *testing.T) {
	s := startEventServer(t)
	serverURL := s.ClientURL()
	port := s.Addr().(*net.TCPAddr).Port
	s.Shutdown()
	s.WaitForShutdown()
	publisher, err := Open(serverURL, "msgvault", "", time.Second)
	require.NoError(t, err, "an unavailable server must not stop the daemon")
	t.Cleanup(publisher.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	event := events.Event{ID: "reconnect-email", Type: events.EmailArchived, Data: json.RawMessage(`{}`)}
	require.Error(t, publisher.Publish(ctx, event))

	restarted, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: port, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { restarted.Shutdown(); restarted.WaitForShutdown() })
	go restarted.Start()
	require.True(t, restarted.ReadyForConnections(5*time.Second))
	require.Eventually(t, publisher.connection.IsConnected, 5*time.Second, 10*time.Millisecond)
	stream, err := publisher.jetstream.CreateStream(ctx, jetstream.StreamConfig{
		Name: "EVENTS", Subjects: []string{"msgvault.>"},
	})
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	assert.Zero(t, info.State.Msgs, "the failed event was not buffered")
	require.NoError(t, publisher.Publish(ctx, event))
}
