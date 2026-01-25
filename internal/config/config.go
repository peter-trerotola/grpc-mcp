// Package config provides configuration types and loading for grpc-mcp.
package config

import (
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Config represents the top-level configuration for grpc-mcp.
type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Endpoints []EndpointConfig `yaml:"endpoints"`
}

// ServerConfig contains server-level settings.
type ServerConfig struct {
	Name      string `yaml:"name"`
	Version   string `yaml:"version"`
	Transport string `yaml:"transport"` // "stdio" or "sse"
	Address   string `yaml:"address"`   // Only for SSE transport
}

// EndpointConfig represents a single gRPC endpoint configuration.
type EndpointConfig struct {
	Name        string            `yaml:"name"`
	Address     string            `yaml:"address"`
	Auth        AuthConfig        `yaml:"auth"`
	TLS         TLSConfig         `yaml:"tls"`
	Include     []string          `yaml:"include"`
	Exclude     []string          `yaml:"exclude"`
	HealthCheck HealthCheckConfig `yaml:"healthCheck"`
}

// AuthConfig specifies authentication settings.
type AuthConfig struct {
	Type        string       `yaml:"type"` // "none", "bearer", "api-key", "mtls"
	BearerToken string       `yaml:"bearerToken"`
	APIKey      APIKeyConfig `yaml:"apiKey"`
	MTLS        MTLSConfig   `yaml:"mtls"`
}

// APIKeyConfig holds API key authentication settings.
type APIKeyConfig struct {
	Header string `yaml:"header"`
	Value  string `yaml:"value"`
}

// MTLSConfig holds mutual TLS settings.
type MTLSConfig struct {
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
	CAFile   string `yaml:"caFile"`
}

// TLSConfig specifies TLS settings for the connection.
type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
	CAFile             string `yaml:"caFile"`
}

// HealthCheckConfig specifies health check settings.
type HealthCheckConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
}

// Default values
const (
	DefaultServerName    = "grpc-mcp"
	DefaultServerVersion = "1.0.0"
	DefaultTransport     = "stdio"
	DefaultAuthType      = "none"
)

// Validation errors
var (
	ErrNoEndpoints           = errors.New("at least one endpoint is required")
	ErrEmptyEndpointName     = errors.New("endpoint name cannot be empty")
	ErrEmptyEndpointAddress  = errors.New("endpoint address cannot be empty")
	ErrDuplicateEndpointName = errors.New("duplicate endpoint name")
	ErrInvalidTransport      = errors.New("transport must be 'stdio' or 'sse'")
	ErrInvalidAuthType       = errors.New("auth type must be 'none', 'bearer', 'api-key', or 'mtls'")
	ErrMissingBearerToken    = errors.New("bearer token is required for bearer auth")
	ErrMissingAPIKeyHeader   = errors.New("api key header is required for api-key auth")
	ErrMissingAPIKeyValue    = errors.New("api key value is required for api-key auth")
	ErrMissingMTLSCert       = errors.New("cert file is required for mtls auth")
	ErrMissingMTLSKey        = errors.New("key file is required for mtls auth")
	ErrMissingMTLSCA         = errors.New("ca file is required for mtls auth")
	ErrSSEMissingAddress     = errors.New("address is required for SSE transport")
)

// validNameRegex matches valid endpoint names (alphanumeric, hyphens, underscores).
var validNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// Validate validates the configuration and returns all validation errors.
func (c *Config) Validate() error {
	var errs []error

	// Apply defaults
	c.applyDefaults()

	// Validate server config
	if err := c.Server.Validate(); err != nil {
		errs = append(errs, fmt.Errorf("server: %w", err))
	}

	// Validate endpoints
	if len(c.Endpoints) == 0 {
		errs = append(errs, ErrNoEndpoints)
	}

	names := make(map[string]bool)
	for i, ep := range c.Endpoints {
		if err := ep.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("endpoint[%d] (%s): %w", i, ep.Name, err))
		}
		if names[ep.Name] {
			errs = append(errs, fmt.Errorf("endpoint[%d]: %w: %s", i, ErrDuplicateEndpointName, ep.Name))
		}
		names[ep.Name] = true
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// applyDefaults sets default values for unset fields.
func (c *Config) applyDefaults() {
	if c.Server.Name == "" {
		c.Server.Name = DefaultServerName
	}
	if c.Server.Version == "" {
		c.Server.Version = DefaultServerVersion
	}
	if c.Server.Transport == "" {
		c.Server.Transport = DefaultTransport
	}

	for i := range c.Endpoints {
		if c.Endpoints[i].Auth.Type == "" {
			c.Endpoints[i].Auth.Type = DefaultAuthType
		}
	}
}

// Validate validates the server configuration.
func (s *ServerConfig) Validate() error {
	switch s.Transport {
	case "stdio", "sse":
		// Valid
	default:
		return ErrInvalidTransport
	}

	if s.Transport == "sse" && s.Address == "" {
		return ErrSSEMissingAddress
	}

	return nil
}

// Validate validates an endpoint configuration.
func (e *EndpointConfig) Validate() error {
	var errs []error

	if e.Name == "" {
		errs = append(errs, ErrEmptyEndpointName)
	} else if !validNameRegex.MatchString(e.Name) {
		errs = append(errs, fmt.Errorf("invalid endpoint name '%s': must start with letter and contain only alphanumeric, hyphens, or underscores", e.Name))
	}

	if e.Address == "" {
		errs = append(errs, ErrEmptyEndpointAddress)
	}

	if err := e.Auth.Validate(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Validate validates the auth configuration.
func (a *AuthConfig) Validate() error {
	switch a.Type {
	case "none":
		return nil
	case "bearer":
		if a.BearerToken == "" {
			return ErrMissingBearerToken
		}
	case "api-key":
		var errs []error
		if a.APIKey.Header == "" {
			errs = append(errs, ErrMissingAPIKeyHeader)
		}
		if a.APIKey.Value == "" {
			errs = append(errs, ErrMissingAPIKeyValue)
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
	case "mtls":
		var errs []error
		if a.MTLS.CertFile == "" {
			errs = append(errs, ErrMissingMTLSCert)
		}
		if a.MTLS.KeyFile == "" {
			errs = append(errs, ErrMissingMTLSKey)
		}
		if a.MTLS.CAFile == "" {
			errs = append(errs, ErrMissingMTLSCA)
		}
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
	default:
		return ErrInvalidAuthType
	}
	return nil
}

// Clone returns a deep copy of the config.
func (c *Config) Clone() *Config {
	if c == nil {
		return nil
	}
	clone := &Config{
		Server: c.Server,
	}
	clone.Endpoints = make([]EndpointConfig, len(c.Endpoints))
	for i, ep := range c.Endpoints {
		clone.Endpoints[i] = ep.Clone()
	}
	return clone
}

// Clone returns a deep copy of the endpoint config.
func (e EndpointConfig) Clone() EndpointConfig {
	clone := e
	clone.Include = append([]string(nil), e.Include...)
	clone.Exclude = append([]string(nil), e.Exclude...)
	return clone
}
