package eamqp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReconnectPolicy_Backoff(t *testing.T) {
	policy := ReconnectPolicy{
		Enabled:    true,
		Initial:    1 * time.Second,
		Max:        30 * time.Second,
		Multiplier: 2.0,
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},   // Initial (attempt 0)
		{1, 2 * time.Second},   // Initial * 2^1 = 2
		{2, 4 * time.Second},   // Initial * 2^2 = 4
		{3, 8 * time.Second},   // Initial * 2^3 = 8
		{4, 16 * time.Second},  // Initial * 2^4 = 16
		{5, 30 * time.Second},  // capped at Max
		{10, 30 * time.Second}, // still capped
	}

	for _, tt := range tests {
		got := policy.Backoff(tt.attempt)
		assert.Equal(t, tt.want, got, "attempt=%d", tt.attempt)
	}
}

func TestReconnectManager(t *testing.T) {
	t.Run("next increments attempt", func(t *testing.T) {
		policy := DefaultReconnectPolicy()
		rm := newReconnectManager(policy, nil, nil)

		attempt, proceed := rm.next()
		assert.Equal(t, 0, attempt)
		assert.True(t, proceed)

		attempt, proceed = rm.next()
		assert.Equal(t, 1, attempt)
		assert.True(t, proceed)
	})

	t.Run("respects max attempts", func(t *testing.T) {
		policy := ReconnectPolicy{Enabled: true, MaxAttempts: 2}
		rm := newReconnectManager(policy, nil, nil)

		_, proceed := rm.next()
		assert.True(t, proceed)

		_, proceed = rm.next()
		assert.True(t, proceed)

		_, proceed = rm.next()
		assert.False(t, proceed)
	})

	t.Run("disabled returns false immediately", func(t *testing.T) {
		policy := ReconnectPolicy{Enabled: false}
		rm := newReconnectManager(policy, nil, nil)

		_, proceed := rm.next()
		assert.False(t, proceed)
	})

	t.Run("reset clears attempt", func(t *testing.T) {
		policy := DefaultReconnectPolicy()
		rm := newReconnectManager(policy, nil, nil)

		rm.next()
		rm.next()
		rm.reset()

		attempt, proceed := rm.next()
		assert.Equal(t, 0, attempt)
		assert.True(t, proceed)
	})

	t.Run("backoff with zero multiplier", func(t *testing.T) {
		policy := ReconnectPolicy{Enabled: true, Initial: 1 * time.Second, Multiplier: 0}
		rm := newReconnectManager(policy, nil, nil)

		// 0 multiplier means constant delay
		delay0 := rm.delay(0)
		delay1 := rm.delay(1)
		assert.Equal(t, 1*time.Second, delay0)
		assert.Equal(t, 1*time.Second, delay1) // stays at initial
	})
}

func TestDefaultReconnectPolicy(t *testing.T) {
	policy := DefaultReconnectPolicy()

	assert.True(t, policy.Enabled)
	assert.Equal(t, 1*time.Second, policy.Initial)
	assert.Equal(t, 30*time.Second, policy.Max)
	assert.Equal(t, 2.0, policy.Multiplier)
	assert.Equal(t, 0, policy.MaxAttempts)
}
