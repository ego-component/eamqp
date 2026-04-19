package eamqp

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Error wraps amqp.Error with component context.
type Error struct {
	amqpErr *amqp.Error
	Component string // "connection", "channel", "publish", "consume"
	Op       string // Operation name
}

// Error implements error.
func (e *Error) Error() string {
	if e.amqpErr != nil {
		return fmt.Sprintf("eamqp[%s.%s]: %s (code=%d, server=%v, recoverable=%v)",
			e.Component, e.Op, e.amqpErr.Reason, e.amqpErr.Code, e.amqpErr.Server, e.amqpErr.Recover)
	}
	return fmt.Sprintf("eamqp[%s.%s]: unknown error", e.Component, e.Op)
}

// IsRetryable returns true if the error is temporary and can be retried.
func (e *Error) IsRetryable() bool {
	if e.amqpErr == nil {
		return false
	}
	return e.amqpErr.Recover
}

// Unwrap returns the underlying amqp.Error.
func (e *Error) Unwrap() error {
	return e.amqpErr
}

// wrapError wraps an amqp.Error with context.
func wrapError(err *amqp.Error, component, op string) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		amqpErr:   err,
		Component: component,
		Op:       op,
	}
}
