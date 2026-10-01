package cmd

import (
	"context"
	"fmt"
	"time"

	"go.kenn.io/msgvault/internal/config"
	natsevents "go.kenn.io/msgvault/internal/events/nats"
	"go.kenn.io/msgvault/internal/store"
)

func configureArchiveEvents(ctx context.Context, s *store.Store, cfg config.EventsConfig) (func(), error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return func() {}, nil
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil {
		return nil, fmt.Errorf("parse event timeout: %w", err)
	}
	publisher, err := natsevents.Open(cfg.NATS.URL, cfg.NATS.SubjectPrefix, cfg.NATS.CredentialsFile, timeout)
	if err != nil {
		return nil, err
	}
	if err := s.SetEventPublisher(ctx, publisher, timeout); err != nil {
		publisher.Close()
		return nil, err
	}
	return publisher.Close, nil
}
