package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	content := `
server:
  name: "test-server"
  version: "1.0.0"
  transport: "stdio"

endpoints:
  - name: "local-api"
    address: "localhost:50051"
    auth:
      type: "none"
    tls:
      enabled: false
`
	tmpFile := writeTestFile(t, content)

	cfg, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Name != "test-server" {
		t.Errorf("expected name 'test-server', got %q", cfg.Server.Name)
	}
	if len(cfg.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}
	if cfg.Endpoints[0].Name != "local-api" {
		t.Errorf("expected endpoint name 'local-api', got %q", cfg.Endpoints[0].Name)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "reading config file") {
		t.Errorf("expected reading error, got: %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	content := `
server:
  name: [invalid yaml
`
	tmpFile := writeTestFile(t, content)

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !strings.Contains(err.Error(), "parsing config") {
		t.Errorf("expected parsing error, got: %v", err)
	}
}

func TestLoad_ValidationError(t *testing.T) {
	content := `
server:
  transport: "invalid"
endpoints: []
`
	tmpFile := writeTestFile(t, content)

	_, err := Load(tmpFile)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "validating config") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestParse_EnvVarExpansion(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_TOKEN", "my-secret-token")
	os.Setenv("TEST_ADDRESS", "api.example.com:443")
	defer os.Unsetenv("TEST_TOKEN")
	defer os.Unsetenv("TEST_ADDRESS")

	content := `
server:
  name: "test"
endpoints:
  - name: "api"
    address: "${TEST_ADDRESS}"
    auth:
      type: "bearer"
      bearerToken: "${TEST_TOKEN}"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Endpoints[0].Address != "api.example.com:443" {
		t.Errorf("expected expanded address, got %q", cfg.Endpoints[0].Address)
	}
	if cfg.Endpoints[0].Auth.BearerToken != "my-secret-token" {
		t.Errorf("expected expanded token, got %q", cfg.Endpoints[0].Auth.BearerToken)
	}
}

func TestParse_EnvVarDefaultValue(t *testing.T) {
	// Ensure variable is not set
	os.Unsetenv("UNSET_VAR")

	content := `
server:
  name: "${UNSET_VAR:-default-name}"
endpoints:
  - name: "api"
    address: "localhost:50051"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Name != "default-name" {
		t.Errorf("expected default value, got %q", cfg.Server.Name)
	}
}

func TestParse_EnvVarDefaultValueWithSet(t *testing.T) {
	os.Setenv("SET_VAR", "actual-value")
	defer os.Unsetenv("SET_VAR")

	content := `
server:
  name: "${SET_VAR:-default-name}"
endpoints:
  - name: "api"
    address: "localhost:50051"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Name != "actual-value" {
		t.Errorf("expected actual value, got %q", cfg.Server.Name)
	}
}

func TestParse_EnvVarUnsetPreserved(t *testing.T) {
	os.Unsetenv("COMPLETELY_UNSET")

	content := `
server:
  name: "${COMPLETELY_UNSET}"
endpoints:
  - name: "api"
    address: "localhost:50051"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unset variables without defaults are preserved as-is
	if cfg.Server.Name != "${COMPLETELY_UNSET}" {
		t.Errorf("expected preserved var, got %q", cfg.Server.Name)
	}
}

func TestParse_MultipleEndpoints(t *testing.T) {
	content := `
endpoints:
  - name: "api1"
    address: "localhost:50051"
  - name: "api2"
    address: "localhost:50052"
    auth:
      type: "bearer"
      bearerToken: "token"
  - name: "api3"
    address: "localhost:50053"
    auth:
      type: "api-key"
      apiKey:
        header: "x-api-key"
        value: "my-key"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Endpoints) != 3 {
		t.Errorf("expected 3 endpoints, got %d", len(cfg.Endpoints))
	}
}

func TestParse_HealthCheckConfig(t *testing.T) {
	content := `
endpoints:
  - name: "api"
    address: "localhost:50051"
    healthCheck:
      enabled: true
      interval: 30s
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Endpoints[0].HealthCheck.Enabled {
		t.Error("expected healthCheck.enabled to be true")
	}
	if cfg.Endpoints[0].HealthCheck.Interval.Seconds() != 30 {
		t.Errorf("expected 30s interval, got %v", cfg.Endpoints[0].HealthCheck.Interval)
	}
}

func TestParse_IncludeExcludePatterns(t *testing.T) {
	content := `
endpoints:
  - name: "api"
    address: "localhost:50051"
    include:
      - "users.*"
      - "orders.*"
    exclude:
      - "grpc.reflection.*"
      - "grpc.health.*"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Endpoints[0].Include) != 2 {
		t.Errorf("expected 2 include patterns, got %d", len(cfg.Endpoints[0].Include))
	}
	if len(cfg.Endpoints[0].Exclude) != 2 {
		t.Errorf("expected 2 exclude patterns, got %d", len(cfg.Endpoints[0].Exclude))
	}
}

func TestParse_TLSConfig(t *testing.T) {
	content := `
endpoints:
  - name: "secure-api"
    address: "api.example.com:443"
    tls:
      enabled: true
      insecureSkipVerify: false
      caFile: "/certs/ca.pem"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Endpoints[0].TLS.Enabled {
		t.Error("expected TLS to be enabled")
	}
	if cfg.Endpoints[0].TLS.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be false")
	}
	if cfg.Endpoints[0].TLS.CAFile != "/certs/ca.pem" {
		t.Errorf("expected CA file path, got %q", cfg.Endpoints[0].TLS.CAFile)
	}
}

func TestParse_MTLSConfig(t *testing.T) {
	content := `
endpoints:
  - name: "mtls-api"
    address: "secure.internal:443"
    auth:
      type: "mtls"
      mtls:
        certFile: "/certs/client.pem"
        keyFile: "/certs/client-key.pem"
        caFile: "/certs/ca.pem"
`
	cfg, err := Parse([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Endpoints[0].Auth.Type != "mtls" {
		t.Errorf("expected mtls auth type, got %q", cfg.Endpoints[0].Auth.Type)
	}
	if cfg.Endpoints[0].Auth.MTLS.CertFile != "/certs/client.pem" {
		t.Errorf("expected cert file, got %q", cfg.Endpoints[0].Auth.MTLS.CertFile)
	}
}

func TestMustLoad_Panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid config")
		}
	}()

	MustLoad("/nonexistent/config.yaml")
}

// writeTestFile creates a temporary file with the given content and returns its path.
func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
	return tmpFile
}
