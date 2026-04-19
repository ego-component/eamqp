// Package eamqp provides RabbitMQ integration for Ego framework.
package eamqp

// Exchange types.
const (
	ExchangeDirect  = "direct"
	ExchangeFanout  = "fanout"
	ExchangeTopic   = "topic"
	ExchangeHeaders = "headers"
)

// Delivery modes.
const (
	// Transient messages are lost on broker restart.
	Transient uint8 = 1
	// Persistent messages survive broker restart.
	Persistent uint8 = 2
)

// Queue argument keys.
const (
	QueueTypeArg                 = "x-queue-type"
	QueueMaxLenArg               = "x-max-length"
	QueueMaxLenBytesArg          = "x-max-length-bytes"
	QueueOverflowArg             = "x-overflow"
	QueueMessageTTLArg           = "x-message-ttl"
	QueueTTLArg                  = "x-expires"
	StreamMaxAgeArg              = "x-max-age"
	StreamMaxSegmentSizeBytesArg = "x-stream-max-segment-size-bytes"
	QueueVersionArg              = "x-queue-version"
	ConsumerTimeoutArg           = "x-consumer-timeout"
	SingleActiveConsumerArg      = "x-single-active-consumer"
	QueueExclusiveArg            = "x-exclusive"
)

// Queue type values.
const (
	QueueTypeClassic = "classic"
	QueueTypeQuorum  = "quorum"
	QueueTypeStream  = "stream"
)

// Overflow behavior values.
const (
	QueueOverflowDropHead         = "drop-head"
	QueueOverflowRejectPublish    = "reject-publish"
	QueueOverflowRejectPublishDLX = "reject-publish-dlx"
)

// Expiration constants.
const (
	NeverExpire       = ""
	ImmediatelyExpire = "0"
)

// PackageName is the component identifier used by Ego logging and metrics.
const PackageName = "component.eamqp"

// ComponentName is kept as a short component type label for AMQP metrics.
const ComponentName = "eamqp"
