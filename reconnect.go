package eamqp

import (
	"math"
	"sync/atomic"
	"time"
)

// ReconnectPolicy defines the reconnection strategy.
type ReconnectPolicy struct {
	Enabled     bool
	Initial     time.Duration // Initial backoff interval
	Max         time.Duration // Maximum backoff interval
	Multiplier  float64       // Backoff multiplier
	MaxAttempts int           // 0 = infinite
}

// DefaultReconnectPolicy returns the default reconnection policy.
func DefaultReconnectPolicy() ReconnectPolicy {
	return ReconnectPolicy{
		Enabled:     true,
		Initial:     1 * time.Second,
		Max:         30 * time.Second,
		Multiplier:  2.0,
		MaxAttempts: 0,
	}
}

// Backoff calculates the next reconnect delay.
func (p ReconnectPolicy) Backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return p.Initial
	}

	// Prevent division-by-zero when multiplier is 0
	if p.Multiplier == 0 {
		return p.Initial
	}

	delay := float64(p.Initial) * math.Pow(p.Multiplier, float64(attempt))
	if delay > float64(p.Max) {
		delay = float64(p.Max)
	}
	return time.Duration(delay)
}

// reconnectManager manages the reconnection lifecycle.
type reconnectManager struct {
	policy        ReconnectPolicy
	attempt       int32
	onReconnect  func(attempt int)
	onDisconnect func(err error)
	logger       Logger
}

// newReconnectManager creates a new reconnect manager.
func newReconnectManager(policy ReconnectPolicy, opts *Options, log Logger) *reconnectManager {
	rm := &reconnectManager{
		policy:        policy,
		attempt:       -1,
		onReconnect:   func(int) {},
		onDisconnect: func(error) {},
		logger:        log,
	}

	if opts != nil {
		if opts.OnReconnect != nil {
			rm.onReconnect = opts.OnReconnect
		}
		if opts.OnDisconnect != nil {
			rm.onDisconnect = opts.OnDisconnect
		}
	}

	return rm
}

// reset resets the attempt counter.
func (rm *reconnectManager) reset() {
	atomic.StoreInt32(&rm.attempt, -1)
}

// next returns the next attempt number and whether to proceed.
func (rm *reconnectManager) next() (int, bool) {
	if !rm.policy.Enabled {
		return 0, false
	}

	attempt := atomic.AddInt32(&rm.attempt, 1)
	if rm.policy.MaxAttempts > 0 && attempt >= int32(rm.policy.MaxAttempts) {
		return int(attempt), false
	}
	return int(attempt), true
}

// delay returns the delay for the given attempt number.
func (rm *reconnectManager) delay(attempt int) time.Duration {
	return rm.policy.Backoff(attempt)
}

// onSuccessfulReconnect is called after successful reconnection.
func (rm *reconnectManager) onSuccessfulReconnect() {
	rm.reset()
}

// onLostConnection is called when the connection is lost.
func (rm *reconnectManager) onLostConnection(err error) {
	rm.onDisconnect(err)
}

// ReconnectLoop runs the reconnection loop, calling dial for each attempt.
// It returns when reconnection is no longer possible or enabled.
func (rm *reconnectManager) ReconnectLoop(dial func() error) error {
	for {
		attempt, proceed := rm.next()
		if !proceed {
			return nil
		}

		delay := rm.delay(attempt)
		if attempt > 0 && delay > 0 {
			if rm.logger != nil {
				rm.logger.Warn("eamqp reconnecting", "attempt", attempt+1, "delay", delay)
			}
			time.Sleep(delay)
		}

		if err := dial(); err != nil {
			if rm.logger != nil {
				rm.logger.Error("eamqp reconnect failed", "attempt", attempt+1, "err", err)
			}
			continue
		}

		rm.onSuccessfulReconnect()
		rm.onReconnect(attempt)
		return nil
	}
}

// Logger interface for structured logging.
type Logger interface {
	Debug(msg string, keyvals ...any)
	Info(msg string, keyvals ...any)
	Warn(msg string, keyvals ...any)
	Error(msg string, keyvals ...any)
}

// NopLogger is a no-op logger.
type NopLogger struct{}

func (NopLogger) Debug(msg string, keyvals ...any) {}
func (NopLogger) Info(msg string, keyvals ...any)  {}
func (NopLogger) Warn(msg string, keyvals ...any)  {}
func (NopLogger) Error(msg string, keyvals ...any) {}
