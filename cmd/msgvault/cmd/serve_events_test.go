package cmd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/msgvault/internal/config"
	"go.kenn.io/msgvault/internal/events"
	"go.kenn.io/msgvault/internal/store"
	"go.kenn.io/msgvault/internal/testutil/storetest"
)

func TestConfigureArchiveEvents(t *testing.T) {
	f := storetest.New(t)
	closeEvents, err := configureArchiveEvents(t.Context(), f.Store, config.EventsConfig{})
	require.NoError(t, err)
	closeEvents()

	s, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { s.Shutdown(); s.WaitForShutdown() })
	go s.Start()
	require.True(t, s.ReadyForConnections(5*time.Second))
	connection, err := nats.Connect(s.ClientURL())
	require.NoError(t, err)
	t.Cleanup(connection.Close)
	js, err := jetstream.New(connection)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: "ARCHIVE_EVENTS", Subjects: []string{"msgvault.>"},
	})
	require.NoError(t, err)
	closeEvents, err = configureArchiveEvents(t.Context(), f.Store, config.EventsConfig{
		Enabled: true, NATS: config.NATSConfig{URL: s.ClientURL()},
	})
	require.NoError(t, err)
	t.Cleanup(closeEvents)
	message := storetest.NewMessage(f.Source.ID, f.ConvID).WithSourceMessageID("daemon-email").Build()
	id, err := f.Store.PersistMessage(&store.MessagePersistData{Message: message})
	require.NoError(t, err)
	stored, err := stream.GetMsg(ctx, 1)
	require.NoError(t, err)
	var event events.Event
	require.NoError(t, json.Unmarshal(stored.Data, &event))
	var data events.ArchivedMessage
	require.NoError(t, json.Unmarshal(event.Data, &data))
	assert.Equal(t, events.EmailArchived, event.Type)
	assert.Equal(t, id, data.MessageID)
	assert.Equal(t, "daemon-email", data.SourceMessageID)
}
