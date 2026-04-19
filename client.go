package eamqp

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client is the main AMQP client.
type Client struct {
	config *Config
	opts   *Options

	conn *amqp.Connection
	pool *ConnectionPool

	mu     sync.RWMutex
	closed bool

	logger  Logger
	metrics MetricsCollector

	closeChan chan struct{}
}

// New creates a new AMQP client.
func New(config Config, opts ...Option) (*Client, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// Apply options.
	options := &Options{}
	for _, opt := range opts {
		opt(options)
	}

	// Setup logger.
	var log Logger
	if config.EnableLogger {
		log = &NopLogger{}
	}

	// Setup metrics.
	var metrics MetricsCollector
	if config.EnableMetrics {
		metrics = &NoOpMetrics{}
	}

	client := &Client{
		config:    &config,
		opts:      options,
		logger:    log,
		metrics:   metrics,
		closeChan: make(chan struct{}),
	}

	// Dial.
	if err := client.dial(); err != nil {
		return nil, err
	}

	return client, nil
}

// dial establishes the connection.
func (c *Client) dial() error {
	uris, err := c.config.parseURIs()
	if err != nil {
		return err
	}

	// Single connection mode.
	if c.config.PoolSize <= 1 && len(uris) == 1 {
		conn, err := c.dialConnection(uris[0])
		if err != nil {
			return err
		}
		c.conn = conn
		c.metrics.RecordConnection(true)
		return nil
	}

	// Connection pool mode.
	c.pool = newConnectionPool(c.config, c.opts, c.logger, uris)
	if err := c.pool.dialAll(); err != nil {
		return err
	}

	return nil
}

// dialConnection dials a single connection.
func (c *Client) dialConnection(uri amqp.URI) (*amqp.Connection, error) {
	amqpCfg := amqp.Config{
		Vhost:      uri.Vhost,
		Heartbeat:  c.config.Heartbeat,
		ChannelMax: int(c.config.ChannelMax),
		FrameSize:  c.config.FrameSize,
		Locale:     c.config.Locale,
		Properties: amqp.Table{},
	}

	if c.config.ClientName != "" {
		amqpCfg.Properties["connection_name"] = c.config.ClientName
	}

	// Build TLS config.
	var tlsConfig *tls.Config
	if c.config.TLSConfig != nil {
		tlsConfig = c.config.TLSConfig
	} else if c.config.TLSCertFile != "" {
		tc, err := c.config.buildTLSConfig(uri.Host)
		if err != nil {
			return nil, err
		}
		tlsConfig = tc
	}
	amqpCfg.TLSClientConfig = tlsConfig

	// SASL auth.
	if c.config.Username != "" || c.config.Password != "" {
		amqpCfg.SASL = []amqp.Authentication{
			&amqp.PlainAuth{
				Username: c.config.Username,
				Password: c.config.Password,
			},
		}
	}

	// Dial.
	conn, err := amqp.DialConfig(uri.String(), amqpCfg)
	if err != nil {
		return nil, fmt.Errorf("eamqp: failed to dial %s: %w", uri, err)
	}

	if c.logger != nil {
		c.logger.Info("eamqp connected", "addr", uri.String())
	}

	return conn, nil
}

// Close closes the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closeChan)

	var lastErr error

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			lastErr = err
		}
		c.metrics.RecordConnection(false)
	}

	if c.pool != nil {
		if err := c.pool.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// IsClosed returns true if the client is closed.
func (c *Client) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// NewChannel creates a new channel.
func (c *Client) NewChannel() (*Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, fmt.Errorf("eamqp: client is closed")
	}

	// Single connection mode.
	if c.conn != nil {
		amqpCh, err := c.conn.Channel()
		if err != nil {
			return nil, fmt.Errorf("eamqp: failed to open channel: %w", err)
		}

		if c.opts != nil && c.opts.ChannelOptions != nil {
			if err := c.opts.ChannelOptions(amqpCh); err != nil {
				amqpCh.Close()
				return nil, fmt.Errorf("eamqp: channel options failed: %w", err)
			}
		}

		return newChannel(amqpCh, c), nil
	}

	// Pool mode.
	if c.pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		amqpCh, release, err := c.pool.AcquireChannel(ctx)
		if err != nil {
			return nil, fmt.Errorf("eamqp: failed to acquire channel: %w", err)
		}

		c.metrics.RecordChannelAcquired()

		// Apply channel options.
		if c.opts != nil && c.opts.ChannelOptions != nil {
			if err := c.opts.ChannelOptions(amqpCh); err != nil {
				release()
				return nil, fmt.Errorf("eamqp: channel options failed: %w", err)
			}
		}

		wrapped := &ChannelWithRelease{
			Channel: Channel{amqpCh: amqpCh, client: c},
			release: release,
		}

		return &wrapped.Channel, nil
	}

	return nil, fmt.Errorf("eamqp: no connection available")
}

// ChannelWithRelease wraps Channel with a release function for pooled channels.
type ChannelWithRelease struct {
	Channel
	release func()
}

// Close returns the channel to the pool.
func (c *ChannelWithRelease) Close() error {
	c.release()
	if c.Channel.client != nil {
		if m := c.Channel.client.GetMetrics(); m != nil {
			m.RecordChannelReturned()
		}
	}
	return nil
}

// AcquireChannel acquires a channel from the pool (pool mode only).
func (c *Client) AcquireChannel(ctx context.Context) (*Channel, func(), error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return nil, nil, fmt.Errorf("eamqp: client is closed")
	}

	if c.pool == nil {
		return nil, nil, fmt.Errorf("eamqp: connection pool not enabled")
	}

	amqpCh, release, err := c.pool.AcquireChannel(ctx)
	if err != nil {
		return nil, nil, err
	}

	c.metrics.RecordChannelAcquired()

	wrapped := newChannel(amqpCh, c)

	wrappedRelease := func() {
		release()
		c.metrics.RecordChannelReturned()
	}

	return wrapped, wrappedRelease, nil
}

// NotifyClose returns a channel that receives close notifications.
func (c *Client) NotifyClose() <-chan *Error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn != nil {
		errChan := c.conn.NotifyClose(make(chan *amqp.Error, 1))
		wrapped := make(chan *Error, 1)

		go func() {
			for err := range errChan {
				wrapped <- &Error{amqpErr: err, Component: "connection", Op: "close"}
			}
			close(wrapped)
		}()

		return wrapped
	}

	if c.pool != nil {
		errChan := c.pool.NotifyClose()
		wrapped := make(chan *Error, 1)

		go func() {
			for err := range errChan {
				wrapped <- &Error{amqpErr: err, Component: "connection", Op: "close"}
			}
			close(wrapped)
		}()

		return wrapped
	}

	ch := make(chan *Error)
	close(ch)
	return ch
}

// Config returns the client configuration.
func (c *Client) Config() *Config {
	return c.config
}

// Stats returns pool statistics.
func (c *Client) Stats() PoolStats {
	if c.pool != nil {
		return c.pool.Stats()
	}
	return PoolStats{}
}

// GetLogger returns the logger.
func (c *Client) GetLogger() Logger {
	return c.logger
}

// GetMetrics returns the metrics collector.
func (c *Client) GetMetrics() MetricsCollector {
	return c.metrics
}

// Reconnect attempts to reconnect the client.
func (c *Client) Reconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("eamqp: client is closed")
	}

	// Close existing connection.
	if c.conn != nil {
		c.conn.Close()
		c.metrics.RecordConnection(false)
	}

	// Re-dial.
	return c.dial()
}

// Option configures the client.
type Option func(*Options)

// WithOptions sets the options.
func WithOptions(opts *Options) Option {
	return func(o *Options) {
		*o = *opts
	}
}

// WithLogger sets a custom logger.
func WithLogger(log Logger) Option {
	return func(o *Options) {}
}

// WithMetrics sets a custom metrics collector.
func WithMetrics(m MetricsCollector) Option {
	return func(o *Options) {}
}
