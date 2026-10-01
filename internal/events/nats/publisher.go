// Package nats delivers archive events through NATS JetStream.
package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.kenn.io/msgvault/internal/events"
)

type Publisher struct {
	connection    *nats.Conn
	jetstream     jetstream.JetStream
	subjectPrefix string
}

// Open connects to NATS. An unavailable server does not stop the daemon:
// the connection retries in the background, but failed events are not queued.
// The operator must create a stream for subjectPrefix + ".>".
func Open(serverURL, subjectPrefix, credentialsFile string, timeout time.Duration) (*Publisher, error) {
	options := []nats.Option{
		nats.Name("msgvault-events"), nats.Timeout(timeout),
		nats.RetryOnFailedConnect(true), nats.MaxReconnects(-1),
		nats.ReconnectWait(time.Second), nats.ReconnectBufSize(0),
	}
	if credentialsFile != "" {
		options = append(options, nats.UserCredentials(credentialsFile))
	}
	connection, err := nats.Connect(serverURL, options...)
	if err != nil {
		return nil, fmt.Errorf("connect archive event publisher: %w", err)
	}
	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		return nil, fmt.Errorf("create archive event publisher: %w", err)
	}
	return &Publisher{connection: connection, jetstream: js, subjectPrefix: subjectPrefix}, nil
}

func (p *Publisher) Publish(ctx context.Context, event events.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode archive event: %w", err)
	}
	_, err = p.jetstream.Publish(ctx, p.subjectPrefix+"."+event.Type, data,
		jetstream.WithMsgID(event.ID), jetstream.WithRetryAttempts(0))
	if err != nil {
		return fmt.Errorf("publish archive event: %w", err)
	}
	return nil
}

func (p *Publisher) Close() {
	p.connection.Close()
}
