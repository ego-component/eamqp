package eamqp

import (
	"net"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Options holds optional configuration for the AMQP client.
type Options struct {
	// Dial is a custom dial function. If set, it is used instead of net.Dial.
	// The addr parameter is the host:port from the URI.
	Dial func(network, addr string) (net.Conn, error)

	// Auth specifies SASL authentication mechanisms.
	// If set, this overrides the default PLAIN auth.
	Auth []amqp.Authentication

	// ConnectionName sets the RabbitMQ connection name for management UI.
	ConnectionName string

	// ChannelOptions is called after each channel is opened.
	// Use it to apply channel-level settings (e.g., QoS, confirm mode).
	ChannelOptions func(ch *amqp.Channel) error

	// OnReconnect is called after a successful reconnection.
	// The argument is the number of reconnection attempts.
	OnReconnect func(attempt int)

	// OnDisconnect is called when the connection is lost.
	OnDisconnect func(err error)

	// OnChannelError is called when a channel encounters an error.
	// The Channel parameter is the eamqp.Channel wrapper.
	OnChannelError func(channelID uint16, err error)
}
