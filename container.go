package eamqp

import (
	"fmt"

	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/core/elog"
)

// ContainerOption configures the Container.
type ContainerOption func(c *Container)

// Container wraps the Config and ego logging for configuration-driven initialization.
type Container struct {
	config *Config
	name   string
	logger *elog.Component
	onFail string
}

// DefaultContainer returns a Container with default configuration.
func DefaultContainer() *Container {
	cfg := DefaultConfig()
	return &Container{
		config: &cfg,
		logger: elog.EgoLogger.With(elog.FieldComponent(PackageName)),
		onFail: "panic",
	}
}

// Load loads configuration from ego's config manager and returns a Container.
// The key parameter corresponds to the config section name (e.g., "eamqp").
func Load(key string) *Container {
	c := DefaultContainer()
	if err := econf.UnmarshalKey(key, c.config); err != nil {
		c.logger.Panic("parse config error", elog.FieldErr(err), elog.FieldKey(key))
		return c
	}

	// Use OnFail from config if set.
	if c.config.OnFail != "" {
		c.onFail = c.config.OnFail
	}

	c.logger = c.logger.With(elog.FieldComponentName(key))
	c.name = key
	return c
}

// Build constructs the AMQP Client from the loaded configuration.
// It applies all configured interceptors and handles connection errors according to OnFail.
func (c *Container) Build(options ...ContainerOption) *Client {
	client, err := c.BuildE(options...)
	if err != nil {
		if c.onFail == "panic" {
			c.logger.Panic("start amqp", elog.FieldErr(err), elog.FieldKey(c.name))
		}
		return nil
	}
	return client
}

// BuildE constructs the AMQP Client and returns startup errors to the caller.
func (c *Container) BuildE(options ...ContainerOption) (*Client, error) {
	for _, opt := range options {
		opt(c)
	}

	// Validate configuration.
	if err := c.config.Validate(); err != nil {
		return nil, fmt.Errorf("eamqp[%s]: invalid config: %w", c.name, err)
	}

	redactedAddr := redactAMQPAddress(c.config.Addr)
	c.logger = c.logger.With(elog.FieldAddr(redactedAddr))
	c.logger.Info("start amqp", elog.FieldAddr(redactedAddr))

	client, err := New(*c.config, c.buildClientOptions()...)
	if err != nil {
		return nil, fmt.Errorf("eamqp[%s]: start amqp: %w", c.name, err)
	}

	instances.Store(c.name, client)
	return client, nil
}

func (c *Container) buildClientOptions() []Option {
	options := make([]Option, 0, 2)
	if c.config.EnableAccessInterceptor {
		options = append(options, WithLogger(NewEgoLogger(c.logger)))
	}
	if c.config.EnableMetricInterceptor {
		options = append(options, WithMetrics(NewEgoMetrics(c.name, redactAMQPAddress(c.config.Addr))))
	}
	return options
}

// WithOnFail sets the failure behavior: "panic" (default) or "error".
func WithOnFail(onFail string) ContainerOption {
	return func(c *Container) {
		c.onFail = onFail
	}
}
