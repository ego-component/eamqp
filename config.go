package eamqp

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Authentication holds TLS and credentials configuration.
type Authentication struct {
	// TLS configuration
	TLS *TLSConfig
	// SASL mechanism: "plain", "external", or empty for no SASL.
	SASLMechanism string `json:"saslMechanism" toml:"saslMechanism"`
}

// TLSConfig holds file-based TLS configuration.
type TLSConfig struct {
	CertFile string `json:"certFile" toml:"certFile"`
	KeyFile  string `json:"keyFile" toml:"keyFile"`
	CACert   string `json:"caCert" toml:"caCert"`
	Insecure bool   `json:"insecure" toml:"insecure"`
}

// LoadTLSConfig builds a *tls.Config from file-based settings.
func (c *TLSConfig) LoadTLSConfig() (*tls.Config, error) {
	if c == nil || (c.CertFile == "" && c.KeyFile == "" && c.CACert == "") {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.Insecure,
	}

	if c.CertFile != "" || c.KeyFile != "" {
		if c.CertFile == "" || c.KeyFile == "" {
			return nil, fmt.Errorf("eamqp: TLS cert and key files must be configured together")
		}
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("eamqp: failed to load TLS cert/key: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if c.CACert != "" {
		certPool, err := loadCertPool(c.CACert)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = certPool
	}

	return tlsConfig, nil
}

// Config holds all configuration for the AMQP client.
type Config struct {
	// Addr is the AMQP URI(s). Multiple URIs separated by comma enable
	// basic load balancing. Use amqps:// for TLS.
	// Environment variable: EGO_CONFIG_AMQP_ADDR
	Addr string `json:"addr" toml:"addr"`

	// Vhost overrides the virtual host from Addr. Empty keeps the URI vhost.
	Vhost string `json:"vhost" toml:"vhost"`

	// TLS configuration.
	TLSConfig *tls.Config `json:"-" toml:"-"`

	// TLSFileConfig enables file-based TLS. Alternative to TLSConfig.
	TLSCertFile   string `json:"tlsCertFile" toml:"tlsCertFile"`
	TLSKeyFile    string `json:"tlsKeyFile" toml:"tlsKeyFile"`
	TLSCACert     string `json:"tlsCaCert" toml:"tlsCaCert"`
	TLSServerName string `json:"tlsServerName" toml:"tlsServerName"`

	// Auth overrides credentials from Addr.
	Username string `json:"username" toml:"username"`
	Password string `json:"password" toml:"password"`

	// Tuning parameters.
	Heartbeat  time.Duration `json:"heartbeat" toml:"heartbeat"`
	ChannelMax uint16        `json:"channelMax" toml:"channelMax"`
	FrameSize  int           `json:"frameSize" toml:"frameSize"`
	Locale     string        `json:"locale" toml:"locale"`

	// Connection pool (0 = single connection, N = pool size).
	PoolSize    int           `json:"poolSize" toml:"poolSize"`
	PoolMaxIdle int           `json:"poolMaxIdle" toml:"poolMaxIdle"`
	PoolMaxLife time.Duration `json:"poolMaxLife" toml:"poolMaxLife"`

	// Channel pool (0 = single channel, N = pool size per connection).
	ChannelPoolSize    int           `json:"channelPoolSize" toml:"channelPoolSize"`
	ChannelPoolMaxIdle int           `json:"channelPoolMaxIdle" toml:"channelPoolMaxIdle"`
	ChannelPoolMaxLife time.Duration `json:"channelPoolMaxLife" toml:"channelPoolMaxLife"`

	// Manual reconnect policy. These fields configure helper policies only;
	// the component does not run background reconnect or topology recovery.
	ReconnectInterval    time.Duration `json:"reconnectInterval" toml:"reconnectInterval"`
	ReconnectMaxAttempts int           `json:"reconnectMaxAttempts" toml:"reconnectMaxAttempts"`

	// Observability.
	EnableAccessInterceptor bool `json:"enableAccessInterceptor" toml:"enableAccessInterceptor"`
	EnableMetricInterceptor bool `json:"enableMetricInterceptor" toml:"enableMetricInterceptor"`
	EnableTraceInterceptor  bool `json:"enableTraceInterceptor" toml:"enableTraceInterceptor"`

	// Debug.
	ClientName string `json:"clientName" toml:"clientName"`

	// OnFail controls startup failure behavior: "panic" or "error".
	OnFail string `json:"onFail" toml:"onFail"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Heartbeat:               10 * time.Second,
		ChannelMax:              0,
		FrameSize:               0,
		Locale:                  "en_US",
		PoolSize:                1,
		PoolMaxIdle:             2,
		PoolMaxLife:             time.Hour,
		ChannelPoolSize:         1,
		ChannelPoolMaxIdle:      2,
		ChannelPoolMaxLife:      5 * time.Minute,
		ReconnectInterval:       5 * time.Second,
		ReconnectMaxAttempts:    0,
		EnableAccessInterceptor: false,
		EnableMetricInterceptor: true,
		EnableTraceInterceptor:  true,
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

// ReconnectPolicy returns the explicit reconnect helper policy derived from Config.
func (c Config) ReconnectPolicy() ReconnectPolicy {
	if c.ReconnectInterval <= 0 {
		c.ReconnectInterval = 5 * time.Second
	}
	return ReconnectPolicy{
		Enabled:     true,
		Initial:     c.ReconnectInterval,
		Max:         30 * time.Second,
		Multiplier:  2,
		MaxAttempts: c.ReconnectMaxAttempts,
	}
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
			return nil, fmt.Errorf("eamqp: failed to parse URI %q: %s", redactAMQPURI(addr), redactAMQPURI(err.Error()))
		}

		// Apply overrides from config.
		if c.Vhost != "" {
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
	config := &tls.Config{
		ServerName: serverName,
	}
	if c.TLSServerName != "" {
		config.ServerName = c.TLSServerName
	}

	if c.TLSCertFile != "" || c.TLSKeyFile != "" {
		if c.TLSCertFile == "" || c.TLSKeyFile == "" {
			return nil, fmt.Errorf("eamqp: TLS cert and key files must be configured together")
		}
		cert, err := tls.LoadX509KeyPair(c.TLSCertFile, c.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("eamqp: failed to load TLS cert/key: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}

	if c.TLSCACert != "" {
		certPool, err := loadCertPool(c.TLSCACert)
		if err != nil {
			return nil, err
		}
		config.RootCAs = certPool
	}

	return config, nil
}

// buildAmqpConfig builds an amqp.Config from this Config.
func (c *Config) buildAmqpConfig(uri amqp.URI, opts *Options) (amqp.Config, error) {
	var tlsConfig *tls.Config
	if c.TLSConfig != nil {
		tlsConfig = c.TLSConfig
	} else if c.hasTLSFileConfig() {
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

	properties := amqp.NewConnectionProperties()
	connectionName := c.ClientName
	if opts != nil && opts.ConnectionName != "" {
		connectionName = opts.ConnectionName
	}
	if connectionName != "" {
		properties.SetClientConnectionName(connectionName)
	}

	config := amqp.Config{
		Vhost:           uri.Vhost,
		Heartbeat:       c.Heartbeat,
		ChannelMax:      int(c.ChannelMax),
		FrameSize:       c.FrameSize,
		Locale:          c.Locale,
		TLSClientConfig: tlsConfig,
		Properties:      properties,
	}

	if opts != nil && opts.Dial != nil {
		config.Dial = opts.Dial
	}

	if opts != nil && len(opts.Auth) > 0 {
		config.SASL = opts.Auth
	} else if c.Username != "" || c.Password != "" {
		config.SASL = []amqp.Authentication{
			&amqp.PlainAuth{
				Username: c.Username,
				Password: c.Password,
			},
		}
	}

	return config, nil
}

func (c *Config) hasTLSFileConfig() bool {
	return c.TLSCertFile != "" || c.TLSKeyFile != "" || c.TLSCACert != "" || c.TLSServerName != ""
}

func loadCertPool(caFile string) (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("eamqp: failed to read TLS CA cert: %w", err)
	}

	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(caPEM); !ok {
		return nil, fmt.Errorf("eamqp: failed to parse TLS CA cert %q", caFile)
	}
	return certPool, nil
}

func redactAMQPAddress(addr string) string {
	parts := strings.Split(addr, ",")
	for i, part := range parts {
		parts[i] = redactAMQPURI(strings.TrimSpace(part))
	}
	return strings.Join(parts, ",")
}

func redactAMQPURI(raw string) string {
	if raw == "" {
		return raw
	}

	parsed, err := url.Parse(raw)
	if err == nil && parsed.User != nil {
		username := parsed.User.Username()
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(username, "xxxxx")
		} else {
			parsed.User = url.User(username)
		}
		return parsed.String()
	}

	schemeEnd := strings.Index(raw, "://")
	at := strings.LastIndex(raw, "@")
	if schemeEnd >= 0 && at > schemeEnd {
		userInfo := raw[schemeEnd+3 : at]
		if colon := strings.Index(userInfo, ":"); colon >= 0 {
			return raw[:schemeEnd+3] + userInfo[:colon] + ":xxxxx@" + raw[at+1:]
		}
		return raw[:schemeEnd+3] + "xxxxx@" + raw[at+1:]
	}
	return raw
}
