package eamqp

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"sync/atomic"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ChannelPool manages a pool of AMQP channels for efficient reuse.
type ChannelPool struct {
	conn   *amqp.Connection
	config *Config
	opts   *Options
	logger Logger

	channels chan *amqp.Channel
	mu      sync.RWMutex
	closed  bool

	acquired  int64
	returned  int64
	created   int64
}

// newChannelPool creates a new channel pool.
func newChannelPool(conn *amqp.Connection, cfg *Config, opts *Options, log Logger) (*ChannelPool, error) {
	poolSize := cfg.ChannelPoolSize
	if poolSize <= 0 {
		poolSize = 1
	}

	maxIdle := cfg.ChannelPoolMaxIdle
	if maxIdle <= 0 {
		maxIdle = 2
	}

	return &ChannelPool{
		conn:     conn,
		config:   cfg,
		opts:     opts,
		logger:   log,
		channels: make(chan *amqp.Channel, maxIdle),
	}, nil
}

// Acquire gets a channel from the pool.
func (p *ChannelPool) Acquire(ctx context.Context) (*amqp.Channel, func(), error) {
	if p.closed {
		return nil, nil, fmt.Errorf("eamqp: channel pool is closed")
	}

	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	case ch := <-p.channels:
		if ch != nil && !ch.IsClosed() {
			atomic.AddInt64(&p.acquired, 1)
			return ch, func() { p.release(ch) }, nil
		}
	}

	// Create a new channel.
	ch, err := p.conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	atomic.AddInt64(&p.acquired, 1)
	atomic.AddInt64(&p.created, 1)

	// Apply channel options if set.
	if p.opts != nil && p.opts.ChannelOptions != nil {
		if err := p.opts.ChannelOptions(ch); err != nil {
			ch.Close()
			return nil, nil, fmt.Errorf("eamqp: channel options failed: %w", err)
		}
	}

	return ch, func() { p.release(ch) }, nil
}

// release returns a channel to the pool.
func (p *ChannelPool) release(ch *amqp.Channel) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		ch.Close()
		return
	}

	if ch.IsClosed() {
		return
	}

	select {
	case p.channels <- ch:
		atomic.AddInt64(&p.returned, 1)
	default:
		// Pool is full, close the channel.
		ch.Close()
	}
}

// Close closes all channels in the pool.
func (p *ChannelPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	if p.channels != nil {
		close(p.channels)
		for ch := range p.channels {
			if err := ch.Close(); err != nil {
				// Ignore close errors
			}
		}
	}

	return nil
}

// Stats returns pool statistics.
func (p *ChannelPool) Stats() PoolStats {
	return PoolStats{
		ChannelsActive:   len(p.channels),
		ChannelsAcquired: atomic.LoadInt64(&p.acquired),
		ChannelsReturned: atomic.LoadInt64(&p.returned),
	}
}

// IsClosed returns true if the pool is closed.
func (p *ChannelPool) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// ConnectionPool manages multiple AMQP connections for high availability.
type ConnectionPool struct {
	config  *Config
	opts    *Options
	logger  Logger
	uris    []amqp.URI

	mu           sync.RWMutex
	connections  []*amqp.Connection
	channelPools []*ChannelPool
	closed       bool

	current     int32 // Round-robin index
	total       int64
	errors      int64
	reconnects  int64
}

// newConnectionPool creates a new connection pool.
func newConnectionPool(cfg *Config, opts *Options, log Logger, uris []amqp.URI) *ConnectionPool {
	pool := &ConnectionPool{
		config:   cfg,
		opts:     opts,
		logger:   log,
		uris:    uris,
	}

	poolSize := cfg.PoolSize
	if poolSize <= 0 {
		poolSize = len(uris)
	}
	if poolSize > len(uris) && len(uris) > 0 {
		poolSize = len(uris)
	}

	// Pre-allocate connection slots.
	if poolSize > 0 {
		pool.connections = make([]*amqp.Connection, poolSize)
		pool.channelPools = make([]*ChannelPool, poolSize)
	}

	return pool
}

// dial connects to an AMQP URI.
func (p *ConnectionPool) dial(uri amqp.URI) (*amqp.Connection, error) {
	amqpCfg := amqp.Config{
		Vhost:      uri.Vhost,
		Heartbeat:  p.config.Heartbeat,
		ChannelMax: int(p.config.ChannelMax),
		FrameSize:  p.config.FrameSize,
		Locale:     p.config.Locale,
		Properties: amqp.Table{},
	}

	if p.config.ClientName != "" {
		amqpCfg.Properties["connection_name"] = p.config.ClientName
	}

	// Build TLS config.
	var tlsConfig *tls.Config
	if p.config.TLSConfig != nil {
		tlsConfig = p.config.TLSConfig
	} else if p.config.TLSCertFile != "" {
		serverName := p.config.TLSServerName
		if serverName == "" {
			serverName = uri.Host
		}
		tc, err := p.config.buildTLSConfig(serverName)
		if err != nil {
			return nil, err
		}
		tlsConfig = tc
	}
	amqpCfg.TLSClientConfig = tlsConfig

	// SASL auth.
	if p.config.Username != "" || p.config.Password != "" {
		amqpCfg.SASL = []amqp.Authentication{
			&amqp.PlainAuth{
				Username: p.config.Username,
				Password: p.config.Password,
			},
		}
	}

	// Dial with config.
	conn, err := amqp.DialConfig(uri.String(), amqpCfg)
	if err != nil {
		return nil, fmt.Errorf("eamqp: failed to dial %s: %w", uri, err)
	}

	return conn, nil
}

// dialAll establishes connections to all URIs.
func (p *ConnectionPool) dialAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, uri := range p.uris[:len(p.connections)] {
		conn, err := p.dial(uri)
		if err != nil {
			// Close any connections we've already established.
			for j := 0; j < i; j++ {
				p.connections[j].Close()
			}
			return fmt.Errorf("eamqp: failed to connect to %s: %w", uri, err)
		}

		p.connections[i] = conn
		atomic.AddInt64(&p.total, 1)

		// Create channel pool for this connection.
		chanPool, err := newChannelPool(conn, p.config, p.opts, p.logger)
		if err != nil {
			conn.Close()
			return fmt.Errorf("eamqp: failed to create channel pool: %w", err)
		}
		p.channelPools[i] = chanPool
	}

	return nil
}

// AcquireChannel gets a channel from the next connection (round-robin).
func (p *ConnectionPool) AcquireChannel(ctx context.Context) (*amqp.Channel, func(), error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, nil, fmt.Errorf("eamqp: connection pool is closed")
	}

	n := len(p.connections)
	if n == 0 {
		return nil, nil, fmt.Errorf("eamqp: no connections available")
	}

	// Round-robin.
	idx := int(atomic.AddInt32(&p.current, 1) % int32(n))
	conn := p.connections[idx]
	if conn == nil || conn.IsClosed() {
		return nil, nil, fmt.Errorf("eamqp: connection %d is not available", idx)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, err
	}

	// Apply channel options if set.
	if p.opts != nil && p.opts.ChannelOptions != nil {
		if err := p.opts.ChannelOptions(ch); err != nil {
			ch.Close()
			return nil, nil, fmt.Errorf("eamqp: channel options failed: %w", err)
		}
	}

	return ch, func() { ch.Close() }, nil
}

// AcquireFromPool gets a channel from the pool of the next connection.
func (p *ConnectionPool) AcquireFromPool(ctx context.Context) (*amqp.Channel, func(), error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return nil, nil, fmt.Errorf("eamqp: connection pool is closed")
	}

	n := len(p.channelPools)
	if n == 0 {
		return nil, nil, fmt.Errorf("eamqp: no channel pools available")
	}

	// Round-robin.
	idx := int(atomic.AddInt32(&p.current, 1) % int32(n))
	chanPool := p.channelPools[idx]
	if chanPool == nil {
		return nil, nil, fmt.Errorf("eamqp: channel pool %d is not available", idx)
	}

	return chanPool.Acquire(ctx)
}

// NotifyClose returns a channel that receives connection close notifications.
func (p *ConnectionPool) NotifyClose() <-chan *amqp.Error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return the first connection's notify channel.
	if len(p.connections) > 0 && p.connections[0] != nil {
		errChan := make(chan *amqp.Error, 1)
		p.connections[0].NotifyClose(errChan)
		return errChan
	}

	// Return a closed channel if no connection.
	ch := make(chan *amqp.Error)
	close(ch)
	return ch
}

// Stats returns connection pool statistics.
func (p *ConnectionPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	active := 0
	for _, conn := range p.connections {
		if conn != nil && !conn.IsClosed() {
			active++
		}
	}

	return PoolStats{
		ConnectionsActive: active,
		ConnectionsTotal:   int(atomic.LoadInt64(&p.total)),
		Reconnects:        atomic.LoadInt64(&p.reconnects),
	}
}

// Close closes all connections.
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	var lastErr error
	for _, conn := range p.connections {
		if conn != nil {
			if err := conn.Close(); err != nil {
				lastErr = err
			}
		}
	}

	return lastErr
}

// IsClosed returns true if the pool is closed.
func (p *ConnectionPool) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// incrementReconnects increments the reconnect counter.
func (p *ConnectionPool) incrementReconnects() {
	atomic.AddInt64(&p.reconnects, 1)
}

// incrementErrors increments the error counter.
func (p *ConnectionPool) incrementErrors() {
	atomic.AddInt64(&p.errors, 1)
}

// GetConnection returns the connection at the given index.
func (p *ConnectionPool) GetConnection(idx int) *amqp.Connection {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if idx >= 0 && idx < len(p.connections) {
		return p.connections[idx]
	}
	return nil
}

// Len returns the number of connections in the pool.
func (p *ConnectionPool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.connections)
}
