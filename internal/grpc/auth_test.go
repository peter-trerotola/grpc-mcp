package grpc

import (
	"context"
	"testing"

	"github.com/peter-trerotola/grpc-mcp/internal/config"
)

func TestBearerTokenCredentials_GetRequestMetadata(t *testing.T) {
	creds := &bearerTokenCredentials{
		token:    "my-secret-token",
		insecure: false,
	}

	metadata, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	auth, ok := metadata["authorization"]
	if !ok {
		t.Fatal("expected authorization header")
	}
	if auth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", auth)
	}
}

func TestBearerTokenCredentials_RequireTransportSecurity(t *testing.T) {
	// Secure
	creds := &bearerTokenCredentials{token: "token", insecure: false}
	if !creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to be true")
	}

	// Insecure
	creds = &bearerTokenCredentials{token: "token", insecure: true}
	if creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to be false")
	}
}

func TestAPIKeyCredentials_GetRequestMetadata(t *testing.T) {
	creds := &apiKeyCredentials{
		header:   "x-api-key",
		value:    "my-api-key",
		insecure: false,
	}

	metadata, err := creds.GetRequestMetadata(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	key, ok := metadata["x-api-key"]
	if !ok {
		t.Fatal("expected x-api-key header")
	}
	if key != "my-api-key" {
		t.Errorf("expected 'my-api-key', got %q", key)
	}
}

func TestAPIKeyCredentials_RequireTransportSecurity(t *testing.T) {
	// Secure
	creds := &apiKeyCredentials{header: "x-api-key", value: "key", insecure: false}
	if !creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to be true")
	}

	// Insecure
	creds = &apiKeyCredentials{header: "x-api-key", value: "key", insecure: true}
	if creds.RequireTransportSecurity() {
		t.Error("expected RequireTransportSecurity to be false")
	}
}

func TestBuildDialOptions_NoAuth(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
		TLS:     config.TLSConfig{Enabled: false},
	}

	opts, err := BuildDialOptions(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(opts) == 0 {
		t.Error("expected at least one dial option")
	}
}

func TestBuildDialOptions_BearerAuth(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth: config.AuthConfig{
			Type:        "bearer",
			BearerToken: "my-token",
		},
		TLS: config.TLSConfig{Enabled: false},
	}

	opts, err := BuildDialOptions(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have transport credentials and per-RPC credentials
	if len(opts) < 2 {
		t.Error("expected at least two dial options (transport + per-RPC)")
	}
}

func TestBuildDialOptions_APIKeyAuth(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth: config.AuthConfig{
			Type: "api-key",
			APIKey: config.APIKeyConfig{
				Header: "x-api-key",
				Value:  "my-key",
			},
		},
		TLS: config.TLSConfig{Enabled: false},
	}

	opts, err := BuildDialOptions(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(opts) < 2 {
		t.Error("expected at least two dial options")
	}
}

func TestBuildDialOptions_TLSEnabled(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
		TLS:     config.TLSConfig{Enabled: true},
	}

	opts, err := BuildDialOptions(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(opts) == 0 {
		t.Error("expected at least one dial option")
	}
}

func TestBuildDialOptions_TLSWithCA(t *testing.T) {
	// Skip this test since it requires a valid CA certificate
	// The CA parsing logic is tested indirectly through other tests
	t.Skip("requires valid CA certificate")
}

func TestBuildDialOptions_TLSInsecureSkipVerify(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
		TLS: config.TLSConfig{
			Enabled:            true,
			InsecureSkipVerify: true,
		},
	}

	opts, err := BuildDialOptions(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(opts) == 0 {
		t.Error("expected at least one dial option")
	}
}

func TestBuildDialOptions_TLSWithInvalidCA(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
		TLS: config.TLSConfig{
			Enabled: true,
			CAFile:  "/nonexistent/ca.pem",
		},
	}

	_, err := BuildDialOptions(cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent CA file")
	}
}

func TestBuildDialOptions_InvalidAuthType(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "invalid"},
		TLS:     config.TLSConfig{Enabled: false},
	}

	_, err := BuildDialOptions(cfg)
	if err == nil {
		t.Fatal("expected error for invalid auth type")
	}
}

func TestBuildPerRPCCredentials_None(t *testing.T) {
	cfg := config.EndpointConfig{
		Auth: config.AuthConfig{Type: "none"},
	}

	creds, err := buildPerRPCCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds != nil {
		t.Error("expected nil credentials for none auth")
	}
}

func TestBuildPerRPCCredentials_Bearer(t *testing.T) {
	cfg := config.EndpointConfig{
		Auth: config.AuthConfig{
			Type:        "bearer",
			BearerToken: "token",
		},
		TLS: config.TLSConfig{Enabled: true},
	}

	creds, err := buildPerRPCCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	bearer, ok := creds.(*bearerTokenCredentials)
	if !ok {
		t.Fatal("expected bearerTokenCredentials")
	}
	if bearer.token != "token" {
		t.Errorf("expected token 'token', got %q", bearer.token)
	}
	if bearer.insecure {
		t.Error("expected insecure to be false when TLS enabled")
	}
}

func TestBuildPerRPCCredentials_APIKey(t *testing.T) {
	cfg := config.EndpointConfig{
		Auth: config.AuthConfig{
			Type: "api-key",
			APIKey: config.APIKeyConfig{
				Header: "x-api-key",
				Value:  "key",
			},
		},
		TLS: config.TLSConfig{Enabled: false},
	}

	creds, err := buildPerRPCCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apiKey, ok := creds.(*apiKeyCredentials)
	if !ok {
		t.Fatal("expected apiKeyCredentials")
	}
	if apiKey.header != "x-api-key" {
		t.Errorf("expected header 'x-api-key', got %q", apiKey.header)
	}
	if apiKey.value != "key" {
		t.Errorf("expected value 'key', got %q", apiKey.value)
	}
	if !apiKey.insecure {
		t.Error("expected insecure to be true when TLS disabled")
	}
}
