package eamqp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClient_Config(t *testing.T) {
	cfg := Config{Addr: "amqp://localhost"}
	client := &Client{config: &cfg}

	got := client.Config()
	assert.Equal(t, &cfg, got)
}

func TestClient_IsClosed(t *testing.T) {
	t.Run("not closed", func(t *testing.T) {
		client := &Client{closed: false}
		assert.False(t, client.IsClosed())
	})

	t.Run("closed", func(t *testing.T) {
		client := &Client{closed: true}
		assert.True(t, client.IsClosed())
	})
}

func TestClient_Close(t *testing.T) {
	t.Run("already closed", func(t *testing.T) {
		client := &Client{closed: true, closeChan: make(chan struct{})}
		close(client.closeChan)

		err := client.Close()
		assert.NoError(t, err)
	})
}

func TestNew_InvalidConfig(t *testing.T) {
	_, err := New(Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Addr is required")
}
