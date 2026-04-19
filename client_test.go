package eamqp

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestClient_RawConnection(t *testing.T) {
	client := &Client{}

	assert.Nil(t, client.RawConnection())
}

func TestClient_NotifyBlockedWithoutConnection(t *testing.T) {
	client := &Client{}

	blocked := client.NotifyBlocked()
	_, ok := <-blocked

	assert.False(t, ok)
}

func TestClientReconnectClosesExistingPoolBeforeRedial(t *testing.T) {
	pool := newConnectionPool(&Config{PoolSize: 1}, nil, nil, nil)
	client := &Client{
		config: &Config{Addr: "invalid-uri"},
		pool:   pool,
	}

	err := client.Reconnect()

	require.Error(t, err)
	assert.True(t, pool.IsClosed())
	assert.Nil(t, client.pool)
}

func TestNewRedactsPasswordInDialError(t *testing.T) {
	dialErr := errors.New("dial failed")
	customDial := func(network, addr string) (net.Conn, error) {
		return nil, dialErr
	}

	_, err := New(Config{Addr: "amqp://guest:secret@127.0.0.1:5672/"}, WithOptions(&Options{
		Dial: customDial,
	}))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
	assert.Contains(t, err.Error(), "guest:xxxxx@127.0.0.1")
}

func TestNewRedactsPasswordInPoolDialError(t *testing.T) {
	dialErr := errors.New("dial failed")
	customDial := func(network, addr string) (net.Conn, error) {
		return nil, dialErr
	}

	_, err := New(Config{
		Addr:     "amqp://guest:secret@127.0.0.1:5672/,amqp://admin:topsecret@127.0.0.1:5673/",
		PoolSize: 2,
	}, WithOptions(&Options{
		Dial: customDial,
	}))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "topsecret")
	assert.Contains(t, err.Error(), "guest:xxxxx@127.0.0.1")
}
