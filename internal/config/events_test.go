package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventsConfigDefaultsAndLoad(t *testing.T) {
	defaults := NewDefaultConfig().Events
	assert.False(t, defaults.Enabled)
	assert.Equal(t, "2s", defaults.Timeout)
	assert.Equal(t, "nats", defaults.Transport)
	assert.Equal(t, "msgvault", defaults.NATS.SubjectPrefix)
	cfg := loadConfigText(t, `
[events]
enabled = true
transport = "nats"
timeout = "3s"
[events.nats]
url = "nats://localhost:4222"
subject_prefix = "archive.events"
credentials_file = "events.creds"
`)
	assert.True(t, cfg.Events.Enabled)
	assert.Equal(t, "archive.events", cfg.Events.NATS.SubjectPrefix)
	assert.Equal(t, filepath.Join(cfg.HomeDir, "events.creds"), cfg.Events.NATS.CredentialsFile)
}

func TestEventsConfigRejectsInvalidSettings(t *testing.T) {
	for _, test := range []struct {
		name string
		text string
	}{
		{"transport", `[events]
enabled = true
transport = "unsupported"`},
		{"timeout", `[events]
enabled = true
timeout = "0s"
[events.nats]
url = "nats://localhost:4222"`},
		{"url", `[events]
enabled = true
[events.nats]
url = "http://localhost:4222"`},
		{"credentials in url", `[events]
enabled = true
[events.nats]
url = "nats://user:password@localhost:4222"`},
		{"wildcard subject", `[events]
enabled = true
[events.nats]
url = "nats://localhost:4222"
subject_prefix = "archive.*"`},
		{"empty token", `[events]
enabled = true
[events.nats]
url = "nats://localhost:4222"
subject_prefix = "archive..events"`},
		{"unknown key", `[events]
enabled = false
unknown_key = true`},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, loadConfigTextError(t, test.text))
		})
	}
}
