package eamqp

import (
	"context"
	"fmt"
)

// HealthStatus describes the lightweight client health state.
type HealthStatus string

const (
	// HealthStatusUp means all known AMQP connections are open.
	HealthStatusUp HealthStatus = "up"
	// HealthStatusDegraded means at least one pooled connection is open, but not all are healthy.
	HealthStatusDegraded HealthStatus = "degraded"
	// HealthStatusDown means the client has no usable AMQP connection.
	HealthStatusDown HealthStatus = "down"
	// HealthStatusClosed means the client or connection pool was explicitly closed.
	HealthStatusClosed HealthStatus = "closed"
)

// HealthInfo contains a lightweight health snapshot for readiness checks.
type HealthInfo struct {
	Status            HealthStatus
	Reason            string
	ConnectionsActive int
	ConnectionsTotal  int
}

// Health returns true when the client has at least one usable AMQP connection.
func (c *Client) Health() bool {
	status := c.HealthStatus().Status
	return status == HealthStatusUp || status == HealthStatusDegraded
}

// HealthStatus returns an in-memory health snapshot without opening a channel.
func (c *Client) HealthStatus() HealthInfo {
	if c == nil {
		return HealthInfo{Status: HealthStatusDown, Reason: "client is nil"}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return HealthInfo{Status: HealthStatusClosed, Reason: "client is closed"}
	}

	if c.conn != nil {
		if c.conn.IsClosed() {
			return HealthInfo{
				Status:            HealthStatusDown,
				Reason:            "connection is closed",
				ConnectionsTotal:  1,
				ConnectionsActive: 0,
			}
		}
		return HealthInfo{
			Status:            HealthStatusUp,
			ConnectionsActive: 1,
			ConnectionsTotal:  1,
		}
	}

	if c.pool != nil {
		return c.poolHealthStatus()
	}

	return HealthInfo{Status: HealthStatusDown, Reason: "no connection available"}
}

// Ping verifies that RabbitMQ accepts opening an AMQP channel.
func (c *Client) Ping(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("eamqp: ping canceled: %w", err)
	}
	if c == nil {
		return fmt.Errorf("eamqp: client is nil")
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("eamqp: client is closed")
	}

	if c.conn != nil {
		conn := c.conn
		c.mu.RUnlock()
		if conn.IsClosed() {
			return fmt.Errorf("eamqp: connection is closed")
		}
		ch, err := conn.Channel()
		if err != nil {
			return fmt.Errorf("eamqp: ping failed: %w", err)
		}
		return ch.Close()
	}

	if c.pool != nil {
		pool := c.pool
		c.mu.RUnlock()
		if pool.IsClosed() {
			return fmt.Errorf("eamqp: connection pool is closed")
		}
		ch, release, err := pool.AcquireChannel(ctx)
		if err != nil {
			return fmt.Errorf("eamqp: ping failed: %w", err)
		}
		if release != nil {
			release()
			return nil
		}
		if ch != nil {
			return ch.Close()
		}
		return nil
	}

	c.mu.RUnlock()
	return fmt.Errorf("eamqp: no connection available")
}

func (c *Client) poolHealthStatus() HealthInfo {
	if c.pool.IsClosed() {
		stats := c.pool.Stats()
		return HealthInfo{
			Status:            HealthStatusClosed,
			Reason:            "connection pool is closed",
			ConnectionsActive: stats.ConnectionsActive,
			ConnectionsTotal:  c.poolConnectionTotal(stats),
		}
	}

	stats := c.pool.Stats()
	total := c.poolConnectionTotal(stats)
	if total == 0 {
		return HealthInfo{Status: HealthStatusDown, Reason: "no connection available"}
	}
	if stats.ConnectionsActive == 0 {
		return HealthInfo{
			Status:            HealthStatusDown,
			Reason:            "no active connections",
			ConnectionsActive: stats.ConnectionsActive,
			ConnectionsTotal:  total,
		}
	}
	if stats.ConnectionsActive < total {
		return HealthInfo{
			Status:            HealthStatusDegraded,
			Reason:            "some connections are unavailable",
			ConnectionsActive: stats.ConnectionsActive,
			ConnectionsTotal:  total,
		}
	}

	return HealthInfo{
		Status:            HealthStatusUp,
		ConnectionsActive: stats.ConnectionsActive,
		ConnectionsTotal:  total,
	}
}

func (c *Client) poolConnectionTotal(stats PoolStats) int {
	total := stats.ConnectionsTotal
	if c.pool == nil {
		return total
	}
	if slots := c.pool.Len(); slots > total {
		total = slots
	}
	return total
}
