package eamqp

import (
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultVhost = "/"

// Config holds all configuration for the AMQP client.
type Config struct {
	// Addr is the AMQP URI(s). Multiple URIs separated by comma enable
	// basic load balancing. Use amqps:// for TLS.
	Addr string

	// Vhost overrides the virtual host from Addr.
	Vhost string

	// TLSConfig is the TLS configuration. When set, TLS is enabled.
	// Alternative to file-based TLS config (TLSCertFile, TLSKeyFile, TLSCACert).
	TLSConfig *tls.Config

	// TLSFileConfig enables file-based TLS. Alternative to TLSConfig.
	TLSCertFile   string // Client certificate file
	TLSKeyFile    string // Client private key file
	TLSCACert     string // CA certificate file
	TLSServerName string // Server name for SNI

	// Auth overrides credentials from Addr.
	Username string
	Password string

	// Tuning parameters.
	Heartbeat time.Duration // Connection heartbeat (default: 10s)
	ChannelMax uint16      // Max channels per connection (0 = server default)
	FrameSize  int         // Max frame bytes (0 = server default)
	Locale     string      // Connection locale (default: "en_US")

	// Connection pool (0 = single connection, N = pool size).
	PoolSize    int
	PoolMaxIdle int           // Max idle connections
	PoolMaxLife time.Duration // Max connection lifetime

	// Channel pool (0 = single channel, N = pool size per connection).
	ChannelPoolSize     int
	ChannelPoolMaxIdle int           // Max idle channels per connection
	ChannelPoolMaxLife time.Duration // Max channel lifetime

	// Reconnection.
	Reconnect            bool          // Enable auto-reconnect (default: true)
	ReconnectInterval   time.Duration // Initial reconnect interval (default: 5s)
	ReconnectMaxAttempts int          // Max reconnect attempts, 0 = infinite

	// Observability.
	EnableLogger  bool // Enable ego logger (default: true)
	EnableMetrics bool // Enable metrics collection (default: true)

	// Debug.
	ClientName string // Client connection name (RabbitMQ management UI)
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Vhost:               defaultVhost,
		Heartbeat:           10 * time.Second,
		ChannelMax:          0,
		FrameSize:           0,
		Locale:              "en_US",
		PoolSize:            1,
		PoolMaxIdle:         2,
		PoolMaxLife:         time.Hour,
		ChannelPoolSize:     1,
		ChannelPoolMaxIdle:  2,
		ChannelPoolMaxLife:  5 * time.Minute,
		Reconnect:            true,
		ReconnectInterval:    5 * time.Second,
		ReconnectMaxAttempts: 0,
		EnableLogger:        true,
		EnableMetrics:       true,
	}
}

// Validate checks the config for common issues.
func (c *Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("eamqp config: Addr is required")
	}
	if c.ReconnectInterval <= 0 {
		c.ReconnectInterval = 5 * time.Second
	}
	return nil
}

// parseURIs parses one or more AMQP URIs from Addr.
// Multiple URIs separated by comma enable basic load balancing.
func (c *Config) parseURIs() ([]amqp.URI, error) {
	addrs := strings.Split(c.Addr, ",")
	uris := make([]amqp.URI, 0, len(addrs))

	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		uri, err := amqp.ParseURI(addr)
		if err != nil {
			return nil, fmt.Errorf("eamqp: failed to parse URI %q: %w", addr, err)
		}

		// Apply overrides from config.
		if c.Vhost != "" && c.Vhost != defaultVhost {
			uri.Vhost = c.Vhost
		}
		if c.Username != "" {
			uri.Username = c.Username
		}
		if c.Password != "" {
			uri.Password = c.Password
		}

		uris = append(uris, uri)
	}

	if len(uris) == 0 {
		return nil, fmt.Errorf("eamqp: no valid URIs found in Addr")
	}

	return uris, nil
}

// buildTLSConfig builds a *tls.Config from file-based TLS settings.
func (c *Config) buildTLSConfig(serverName string) (*tls.Config, error) {
	if c.TLSCertFile == "" || c.TLSKeyFile == "" {
		return nil, fmt.Errorf("eamqp: TLS cert and key files are required for file-based TLS")
	}

	cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
	if err != nil {
		return nil, fmt.Errorf("eamqp: failed to load TLS cert/key: %w", err)
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ServerName:   serverName,
	}

	return config, nil
}

// buildAmqpConfig builds an amqp.Config from this Config.
func (c *Config) buildAmqpConfig(uri amqp.URI) (amqp.Config, error) {
	var tlsConfig *tls.Config
	if c.TLSConfig != nil {
		tlsConfig = c.TLSConfig
	} else if c.TLSCertFile != "" {
		serverName := c.TLSServerName
		if serverName == "" {
			serverName = uri.Host
		}
		var err error
		tlsConfig, err = c.buildTLSConfig(serverName)
		if err != nil {
			return amqp.Config{}, err
		}
	}

	config := amqp.Config{
		Vhost:      uri.Vhost,
		Heartbeat:  c.Heartbeat,
		ChannelMax: int(c.ChannelMax),
		FrameSize:  c.FrameSize,
		Locale:     c.Locale,
		TLSClientConfig: tlsConfig,
		Properties: amqp.Table{},
	}

	if c.ClientName != "" {
		config.Properties["connection_name"] = c.ClientName
	}

	// SASL auth.
	if c.Username != "" || c.Password != "" {
		config.SASL = []amqp.Authentication{
			&amqp.PlainAuth{
				Username: c.Username,
				Password: c.Password,
			},
		}
	}

	return config, nil
}
