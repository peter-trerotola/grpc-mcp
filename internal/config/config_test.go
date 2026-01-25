package config

import (
	"strings"
	"testing"
	"time"
)

func TestConfig_Validate_Success(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Name:      "test-server",
			Version:   "1.0.0",
			Transport: "stdio",
		},
		Endpoints: []EndpointConfig{
			{
				Name:    "local-api",
				Address: "localhost:50051",
				Auth:    AuthConfig{Type: "none"},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid config, got error: %v", err)
	}
}

func TestConfig_Validate_AppliesDefaults(t *testing.T) {
	cfg := &Config{
		Endpoints: []EndpointConfig{
			{
				Name:    "local-api",
				Address: "localhost:50051",
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Name != DefaultServerName {
		t.Errorf("expected default name %q, got %q", DefaultServerName, cfg.Server.Name)
	}
	if cfg.Server.Version != DefaultServerVersion {
		t.Errorf("expected default version %q, got %q", DefaultServerVersion, cfg.Server.Version)
	}
	if cfg.Server.Transport != DefaultTransport {
		t.Errorf("expected default transport %q, got %q", DefaultTransport, cfg.Server.Transport)
	}
	if cfg.Endpoints[0].Auth.Type != DefaultAuthType {
		t.Errorf("expected default auth type %q, got %q", DefaultAuthType, cfg.Endpoints[0].Auth.Type)
	}
}

func TestConfig_Validate_NoEndpoints(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Transport: "stdio"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for no endpoints")
	}
	if !strings.Contains(err.Error(), "at least one endpoint is required") {
		t.Errorf("expected 'at least one endpoint' error, got: %v", err)
	}
}

func TestConfig_Validate_DuplicateEndpointNames(t *testing.T) {
	cfg := &Config{
		Endpoints: []EndpointConfig{
			{Name: "api", Address: "localhost:50051"},
			{Name: "api", Address: "localhost:50052"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate endpoint names")
	}
	if !strings.Contains(err.Error(), "duplicate endpoint name") {
		t.Errorf("expected 'duplicate endpoint name' error, got: %v", err)
	}
}

func TestServerConfig_Validate_InvalidTransport(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Transport: "invalid"},
		Endpoints: []EndpointConfig{
			{Name: "api", Address: "localhost:50051"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid transport")
	}
	if !strings.Contains(err.Error(), "transport must be") {
		t.Errorf("expected transport error, got: %v", err)
	}
}

func TestServerConfig_Validate_SSERequiresAddress(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Transport: "sse"},
		Endpoints: []EndpointConfig{
			{Name: "api", Address: "localhost:50051"},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for SSE without address")
	}
	if !strings.Contains(err.Error(), "address is required for SSE") {
		t.Errorf("expected SSE address error, got: %v", err)
	}
}

func TestServerConfig_Validate_SSEWithAddress(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Transport: "sse",
			Address:   ":8080",
		},
		Endpoints: []EndpointConfig{
			{Name: "api", Address: "localhost:50051"},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid SSE config, got error: %v", err)
	}
}

func TestEndpointConfig_Validate_EmptyName(t *testing.T) {
	ep := EndpointConfig{
		Name:    "",
		Address: "localhost:50051",
	}

	err := ep.Validate()
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name cannot be empty") {
		t.Errorf("expected empty name error, got: %v", err)
	}
}

func TestEndpointConfig_Validate_InvalidName(t *testing.T) {
	testCases := []string{
		"123abc", // starts with number
		"my api", // contains space
		"my.api", // contains dot
		"my@api", // contains special char
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			ep := EndpointConfig{
				Name:    name,
				Address: "localhost:50051",
			}

			err := ep.Validate()
			if err == nil {
				t.Fatal("expected error for invalid name")
			}
			if !strings.Contains(err.Error(), "invalid endpoint name") {
				t.Errorf("expected invalid name error, got: %v", err)
			}
		})
	}
}

func TestEndpointConfig_Validate_ValidNames(t *testing.T) {
	testCases := []string{
		"api",
		"my-api",
		"my_api",
		"MyAPI",
		"api123",
		"a",
	}

	for _, name := range testCases {
		t.Run(name, func(t *testing.T) {
			ep := EndpointConfig{
				Name:    name,
				Address: "localhost:50051",
				Auth:    AuthConfig{Type: "none"},
			}

			if err := ep.Validate(); err != nil {
				t.Errorf("expected valid name %q, got error: %v", name, err)
			}
		})
	}
}

func TestEndpointConfig_Validate_EmptyAddress(t *testing.T) {
	ep := EndpointConfig{
		Name:    "api",
		Address: "",
	}

	err := ep.Validate()
	if err == nil {
		t.Fatal("expected error for empty address")
	}
	if !strings.Contains(err.Error(), "address cannot be empty") {
		t.Errorf("expected empty address error, got: %v", err)
	}
}

func TestAuthConfig_Validate_None(t *testing.T) {
	auth := AuthConfig{Type: "none"}
	if err := auth.Validate(); err != nil {
		t.Errorf("expected valid none auth, got error: %v", err)
	}
}

func TestAuthConfig_Validate_Bearer(t *testing.T) {
	// Missing token
	auth := AuthConfig{Type: "bearer"}
	err := auth.Validate()
	if err == nil {
		t.Fatal("expected error for missing bearer token")
	}
	if !strings.Contains(err.Error(), "bearer token is required") {
		t.Errorf("expected bearer token error, got: %v", err)
	}

	// Valid
	auth.BearerToken = "my-token"
	if err := auth.Validate(); err != nil {
		t.Errorf("expected valid bearer auth, got error: %v", err)
	}
}

func TestAuthConfig_Validate_APIKey(t *testing.T) {
	// Missing both
	auth := AuthConfig{Type: "api-key"}
	err := auth.Validate()
	if err == nil {
		t.Fatal("expected error for missing api-key fields")
	}
	if !strings.Contains(err.Error(), "header is required") {
		t.Errorf("expected header error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "value is required") {
		t.Errorf("expected value error, got: %v", err)
	}

	// Valid
	auth.APIKey = APIKeyConfig{
		Header: "x-api-key",
		Value:  "my-key",
	}
	if err := auth.Validate(); err != nil {
		t.Errorf("expected valid api-key auth, got error: %v", err)
	}
}

func TestAuthConfig_Validate_MTLS(t *testing.T) {
	// Missing all
	auth := AuthConfig{Type: "mtls"}
	err := auth.Validate()
	if err == nil {
		t.Fatal("expected error for missing mtls fields")
	}
	if !strings.Contains(err.Error(), "cert file is required") {
		t.Errorf("expected cert file error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "key file is required") {
		t.Errorf("expected key file error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ca file is required") {
		t.Errorf("expected ca file error, got: %v", err)
	}

	// Valid
	auth.MTLS = MTLSConfig{
		CertFile: "/certs/client.pem",
		KeyFile:  "/certs/client-key.pem",
		CAFile:   "/certs/ca.pem",
	}
	if err := auth.Validate(); err != nil {
		t.Errorf("expected valid mtls auth, got error: %v", err)
	}
}

func TestAuthConfig_Validate_InvalidType(t *testing.T) {
	auth := AuthConfig{Type: "invalid"}
	err := auth.Validate()
	if err == nil {
		t.Fatal("expected error for invalid auth type")
	}
	if !strings.Contains(err.Error(), "auth type must be") {
		t.Errorf("expected auth type error, got: %v", err)
	}
}

func TestConfig_Clone(t *testing.T) {
	original := &Config{
		Server: ServerConfig{
			Name:      "test",
			Version:   "1.0.0",
			Transport: "stdio",
		},
		Endpoints: []EndpointConfig{
			{
				Name:    "api",
				Address: "localhost:50051",
				Include: []string{"users.*"},
				Exclude: []string{"admin.*"},
				HealthCheck: HealthCheckConfig{
					Enabled:  true,
					Interval: 30 * time.Second,
				},
			},
		},
	}

	clone := original.Clone()

	// Verify deep copy
	if clone == original {
		t.Error("clone should be a different pointer")
	}

	// Modify clone and verify original unchanged
	clone.Server.Name = "modified"
	clone.Endpoints[0].Include[0] = "modified"

	if original.Server.Name == "modified" {
		t.Error("modifying clone should not affect original")
	}
	if original.Endpoints[0].Include[0] == "modified" {
		t.Error("modifying clone slice should not affect original")
	}
}

func TestConfig_Clone_Nil(t *testing.T) {
	var cfg *Config
	clone := cfg.Clone()
	if clone != nil {
		t.Error("cloning nil config should return nil")
	}
}
