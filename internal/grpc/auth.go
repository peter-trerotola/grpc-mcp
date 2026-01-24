// Package grpc provides gRPC client functionality including connection management,
// reflection-based service discovery, and dynamic RPC invocation.
package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grpc-mcp/grpc-mcp/internal/config"
)

// PerRPCCredentials is a type alias for grpc.PerRPCCredentials.
type PerRPCCredentials = credentials.PerRPCCredentials

// bearerTokenCredentials implements PerRPCCredentials for bearer token auth.
type bearerTokenCredentials struct {
	token    string
	insecure bool
}

func (b *bearerTokenCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b *bearerTokenCredentials) RequireTransportSecurity() bool {
	return !b.insecure
}

// apiKeyCredentials implements PerRPCCredentials for API key auth.
type apiKeyCredentials struct {
	header   string
	value    string
	insecure bool
}

func (a *apiKeyCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		a.header: a.value,
	}, nil
}

func (a *apiKeyCredentials) RequireTransportSecurity() bool {
	return !a.insecure
}

// BuildDialOptions creates gRPC dial options from endpoint configuration.
func BuildDialOptions(cfg config.EndpointConfig) ([]grpc.DialOption, error) {
	var opts []grpc.DialOption

	// Build transport credentials
	transportCreds, err := buildTransportCredentials(cfg)
	if err != nil {
		return nil, fmt.Errorf("building transport credentials: %w", err)
	}
	opts = append(opts, transportCreds)

	// Build per-RPC credentials for authentication
	perRPCCreds, err := buildPerRPCCredentials(cfg)
	if err != nil {
		return nil, fmt.Errorf("building per-RPC credentials: %w", err)
	}
	if perRPCCreds != nil {
		opts = append(opts, grpc.WithPerRPCCredentials(perRPCCreds))
	}

	return opts, nil
}

// buildTransportCredentials creates the transport credentials (TLS or insecure).
func buildTransportCredentials(cfg config.EndpointConfig) (grpc.DialOption, error) {
	// mTLS has its own certificate handling
	if cfg.Auth.Type == "mtls" {
		tlsConfig, err := buildMTLSConfig(cfg.Auth.MTLS)
		if err != nil {
			return nil, err
		}
		return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
	}

	// Standard TLS
	if cfg.TLS.Enabled {
		tlsConfig, err := buildTLSConfig(cfg.TLS)
		if err != nil {
			return nil, err
		}
		return grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)), nil
	}

	// Insecure (plaintext)
	return grpc.WithTransportCredentials(insecure.NewCredentials()), nil
}

// buildTLSConfig creates a TLS configuration from the config.
func buildTLSConfig(cfg config.TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}

	// Load custom CA if specified
	if cfg.CAFile != "" {
		caCert, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("reading CA file: %w", err)
		}
		caPool := x509.NewCertPool()
		if !caPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caPool
	}

	return tlsConfig, nil
}

// buildMTLSConfig creates a mutual TLS configuration.
func buildMTLSConfig(cfg config.MTLSConfig) (*tls.Config, error) {
	// Load client certificate and key
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA file: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
	}, nil
}

// buildPerRPCCredentials creates per-RPC credentials for authentication.
func buildPerRPCCredentials(cfg config.EndpointConfig) (PerRPCCredentials, error) {
	switch cfg.Auth.Type {
	case "none", "mtls":
		// No per-RPC credentials needed
		return nil, nil

	case "bearer":
		return &bearerTokenCredentials{
			token:    cfg.Auth.BearerToken,
			insecure: !cfg.TLS.Enabled,
		}, nil

	case "api-key":
		return &apiKeyCredentials{
			header:   cfg.Auth.APIKey.Header,
			value:    cfg.Auth.APIKey.Value,
			insecure: !cfg.TLS.Enabled,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported auth type: %s", cfg.Auth.Type)
	}
}
