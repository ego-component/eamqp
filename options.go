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

	// ChannelOptions is called after each raw channel is opened.
	// In pooled mode it runs when the raw channel is created, not on every checkout.
	ChannelOptions func(ch *amqp.Channel) error

	// OnReconnect is reserved for a future lifecycle supervisor.
	// Client.Reconnect is explicit and does not invoke this callback today.
	OnReconnect func(attempt int)

	// OnDisconnect is reserved for a future lifecycle supervisor.
	// Use NotifyClose today to observe connection close events.
	OnDisconnect func(err error)

	// OnChannelError is reserved for a future lifecycle supervisor.
	// Use Channel.NotifyClose today to observe channel close events.
	OnChannelError func(channelID uint16, err error)

	// Logger overrides the component logger.
	Logger Logger

	// Metrics overrides the component metrics collector.
	Metrics MetricsCollector
}
