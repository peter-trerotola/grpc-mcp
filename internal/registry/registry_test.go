package registry

import (
	"context"
	"testing"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/grpc-mcp/grpc-mcp/internal/config"
	"github.com/grpc-mcp/grpc-mcp/internal/mcp"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	if len(r.ListEndpoints()) != 0 {
		t.Error("expected empty endpoint list")
	}
}

func TestRegistry_AddRemoveEndpoint(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	cfg := config.EndpointConfig{
		Name:    "test-endpoint",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
	}

	// Add endpoint
	err := r.AddEndpoint(ctx, cfg)
	if err != nil {
		t.Fatalf("AddEndpoint failed: %v", err)
	}

	// Verify it exists
	endpoints := r.ListEndpoints()
	if len(endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(endpoints))
	}

	ep, exists := r.GetEndpoint("test-endpoint")
	if !exists {
		t.Error("endpoint not found")
	}
	if ep.Name() != "test-endpoint" {
		t.Errorf("unexpected name: %s", ep.Name())
	}

	// Try adding duplicate
	err = r.AddEndpoint(ctx, cfg)
	if err == nil {
		t.Error("expected error for duplicate endpoint")
	}

	// Remove endpoint
	err = r.RemoveEndpoint("test-endpoint")
	if err != nil {
		t.Fatalf("RemoveEndpoint failed: %v", err)
	}

	// Verify it's gone
	if len(r.ListEndpoints()) != 0 {
		t.Error("expected empty endpoint list")
	}

	// Try removing non-existent
	err = r.RemoveEndpoint("non-existent")
	if err == nil {
		t.Error("expected error for non-existent endpoint")
	}
}

func TestRegistry_OnChange(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	var events []RegistryEvent
	r.OnChange(func(event RegistryEvent) {
		events = append(events, event)
	})

	cfg := config.EndpointConfig{
		Name:    "callback-test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
	}

	// Add endpoint
	_ = r.AddEndpoint(ctx, cfg)

	// Remove endpoint (should trigger callback)
	_ = r.RemoveEndpoint("callback-test")

	// Give time for async operations
	time.Sleep(50 * time.Millisecond)

	// Should have at least the remove event
	found := false
	for _, e := range events {
		if e.Type == EventEndpointRemoved && e.EndpointName == "callback-test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected EventEndpointRemoved callback")
	}
}

func TestRegistry_ApplyConfig(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	// Apply initial config
	cfg1 := &config.Config{
		Endpoints: []config.EndpointConfig{
			{Name: "ep1", Address: "localhost:50051", Auth: config.AuthConfig{Type: "none"}},
			{Name: "ep2", Address: "localhost:50052", Auth: config.AuthConfig{Type: "none"}},
		},
	}

	err := r.ApplyConfig(ctx, cfg1)
	if err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	if len(r.ListEndpoints()) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(r.ListEndpoints()))
	}

	// Apply config that removes one and adds one
	cfg2 := &config.Config{
		Endpoints: []config.EndpointConfig{
			{Name: "ep1", Address: "localhost:50051", Auth: config.AuthConfig{Type: "none"}},
			{Name: "ep3", Address: "localhost:50053", Auth: config.AuthConfig{Type: "none"}},
		},
	}

	err = r.ApplyConfig(ctx, cfg2)
	if err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	if len(r.ListEndpoints()) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(r.ListEndpoints()))
	}

	// Verify ep2 removed, ep3 added
	if _, exists := r.GetEndpoint("ep2"); exists {
		t.Error("ep2 should be removed")
	}
	if _, exists := r.GetEndpoint("ep3"); !exists {
		t.Error("ep3 should exist")
	}
}

func TestRegistry_Close(t *testing.T) {
	r := NewRegistry()
	ctx := context.Background()

	cfg := config.EndpointConfig{
		Name:    "close-test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
	}

	_ = r.AddEndpoint(ctx, cfg)

	err := r.Close()
	if err != nil {
		// May fail if connection wasn't established, which is expected
	}

	if len(r.ListEndpoints()) != 0 {
		t.Error("expected empty endpoint list after close")
	}
}

func TestRegistry_GetAllTools(t *testing.T) {
	r := NewRegistry()

	// Initially no tools
	tools := r.GetAllTools()
	if len(tools) != 0 {
		t.Error("expected no tools initially")
	}

	// Add some mock tools
	r.mu.Lock()
	r.tools["ep1"] = []mcp.ToolRegistration{
		{Tool: mcplib.Tool{Name: "tool1"}},
		{Tool: mcplib.Tool{Name: "tool2"}},
	}
	r.tools["ep2"] = []mcp.ToolRegistration{
		{Tool: mcplib.Tool{Name: "tool3"}},
	}
	r.mu.Unlock()

	tools = r.GetAllTools()
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}
}

func TestEndpoint_NewEndpoint(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
	}

	ep := NewEndpoint(cfg)

	if ep.Name() != "test" {
		t.Errorf("unexpected name: %s", ep.Name())
	}
	if ep.State() != StateDisconnected {
		t.Errorf("unexpected state: %v", ep.State())
	}
	if ep.IsConnected() {
		t.Error("should not be connected")
	}
}

func TestEndpoint_Config(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "config-test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "bearer", BearerToken: "secret"},
	}

	ep := NewEndpoint(cfg)

	// Get config (should be a copy)
	gotCfg := ep.Config()
	if gotCfg.Name != "config-test" {
		t.Error("unexpected config name")
	}

	// Update config
	newCfg := cfg
	newCfg.Address = "localhost:60061"
	ep.UpdateConfig(newCfg)

	// Verify update
	gotCfg = ep.Config()
	if gotCfg.Address != "localhost:60061" {
		t.Error("config not updated")
	}
}

func TestFilterServices(t *testing.T) {
	services := []string{
		"grpc.reflection.v1alpha.ServerReflection",
		"grpc.health.v1.Health",
		"myapp.UserService",
		"myapp.OrderService",
		"internal.AdminService",
	}

	tests := []struct {
		name     string
		exclude  []string
		expected []string
	}{
		{
			name:     "no exclusions",
			exclude:  nil,
			expected: services,
		},
		{
			name:    "exclude reflection",
			exclude: []string{"grpc.reflection.*"},
			expected: []string{
				"grpc.health.v1.Health",
				"myapp.UserService",
				"myapp.OrderService",
				"internal.AdminService",
			},
		},
		{
			name:    "exclude multiple patterns",
			exclude: []string{"grpc.reflection.*", "grpc.health.*"},
			expected: []string{
				"myapp.UserService",
				"myapp.OrderService",
				"internal.AdminService",
			},
		},
		{
			name:    "exact match",
			exclude: []string{"internal.AdminService"},
			expected: []string{
				"grpc.reflection.v1alpha.ServerReflection",
				"grpc.health.v1.Health",
				"myapp.UserService",
				"myapp.OrderService",
			},
		},
		{
			name:     "exclude all",
			exclude:  []string{"*"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterServices(services, tt.exclude)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d services, got %d", len(tt.expected), len(result))
				return
			}
			for i, svc := range result {
				if svc != tt.expected[i] {
					t.Errorf("expected %s at index %d, got %s", tt.expected[i], i, svc)
				}
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		service string
		pattern string
		match   bool
	}{
		{"grpc.reflection.v1alpha.ServerReflection", "grpc.reflection.*", true},
		{"grpc.health.v1.Health", "grpc.reflection.*", false},
		{"myapp.UserService", "*.UserService", true},
		{"myapp.UserService", "*.OrderService", false},
		{"myapp.UserService", "myapp.UserService", true},
		{"myapp.UserService", "other.UserService", false},
		{"anything", "*", true},
	}

	for _, tt := range tests {
		t.Run(tt.service+"_"+tt.pattern, func(t *testing.T) {
			result := matchPattern(tt.service, tt.pattern)
			if result != tt.match {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.service, tt.pattern, result, tt.match)
			}
		})
	}
}

func TestToolsEqual(t *testing.T) {
	tool1 := mcp.ToolRegistration{Tool: mcplib.Tool{Name: "tool1"}}
	tool2 := mcp.ToolRegistration{Tool: mcplib.Tool{Name: "tool2"}}
	tool3 := mcp.ToolRegistration{Tool: mcplib.Tool{Name: "tool3"}}

	tests := []struct {
		name  string
		a     []mcp.ToolRegistration
		b     []mcp.ToolRegistration
		equal bool
	}{
		{"both nil", nil, nil, true},
		{"both empty", []mcp.ToolRegistration{}, []mcp.ToolRegistration{}, true},
		{"same tools", []mcp.ToolRegistration{tool1, tool2}, []mcp.ToolRegistration{tool1, tool2}, true},
		{"different order", []mcp.ToolRegistration{tool1, tool2}, []mcp.ToolRegistration{tool2, tool1}, true},
		{"different length", []mcp.ToolRegistration{tool1}, []mcp.ToolRegistration{tool1, tool2}, false},
		{"different tools", []mcp.ToolRegistration{tool1, tool2}, []mcp.ToolRegistration{tool1, tool3}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toolsEqual(tt.a, tt.b)
			if result != tt.equal {
				t.Errorf("toolsEqual() = %v, want %v", result, tt.equal)
			}
		})
	}
}

func TestAuthConfigsEqual(t *testing.T) {
	tests := []struct {
		name  string
		a     config.AuthConfig
		b     config.AuthConfig
		equal bool
	}{
		{
			name:  "both none",
			a:     config.AuthConfig{Type: "none"},
			b:     config.AuthConfig{Type: "none"},
			equal: true,
		},
		{
			name:  "different types",
			a:     config.AuthConfig{Type: "none"},
			b:     config.AuthConfig{Type: "bearer"},
			equal: false,
		},
		{
			name:  "same bearer",
			a:     config.AuthConfig{Type: "bearer", BearerToken: "token"},
			b:     config.AuthConfig{Type: "bearer", BearerToken: "token"},
			equal: true,
		},
		{
			name:  "different bearer",
			a:     config.AuthConfig{Type: "bearer", BearerToken: "token1"},
			b:     config.AuthConfig{Type: "bearer", BearerToken: "token2"},
			equal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := authConfigsEqual(tt.a, tt.b)
			if result != tt.equal {
				t.Errorf("authConfigsEqual() = %v, want %v", result, tt.equal)
			}
		})
	}
}
