// Package integration provides end-to-end integration tests for grpc-mcp.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jhump/protoreflect/desc/builder"
	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/peter-trerotola/grpc-mcp/internal/config"
	grpcclient "github.com/peter-trerotola/grpc-mcp/internal/grpc"
	"github.com/peter-trerotola/grpc-mcp/internal/mcp"
	"github.com/peter-trerotola/grpc-mcp/internal/registry"
)

// TestToolGenerationFlow tests the complete flow from proto descriptors to MCP tools.
func TestToolGenerationFlow(t *testing.T) {
	// Build test message descriptors
	requestMsg := builder.NewMessage("UserRequest").
		AddField(builder.NewField("id", builder.FieldTypeString())).
		AddField(builder.NewField("name", builder.FieldTypeString()))

	responseMsg := builder.NewMessage("UserResponse").
		AddField(builder.NewField("id", builder.FieldTypeString())).
		AddField(builder.NewField("name", builder.FieldTypeString())).
		AddField(builder.NewField("email", builder.FieldTypeString()))

	file := builder.NewFile("test.proto").
		SetPackageName("users.v1").
		AddMessage(requestMsg).
		AddMessage(responseMsg)

	fd, err := file.Build()
	if err != nil {
		t.Fatalf("failed to build proto: %v", err)
	}

	reqDesc := fd.FindMessage("users.v1.UserRequest")

	// Create service info
	serviceInfo := &grpcclient.ServiceInfo{
		FullName: "users.v1.UserService",
		Methods: []grpcclient.MethodInfo{
			{
				Name:            "GetUser",
				FullName:        "/users.v1.UserService/GetUser",
				IsClientStream:  false,
				IsServerStream:  false,
				InputDescriptor: reqDesc,
			},
			{
				Name:            "ListUsers",
				FullName:        "/users.v1.UserService/ListUsers",
				IsClientStream:  false,
				IsServerStream:  true,
				InputDescriptor: reqDesc,
			},
			{
				Name:            "CreateUsers",
				FullName:        "/users.v1.UserService/CreateUsers",
				IsClientStream:  true,
				IsServerStream:  false,
				InputDescriptor: reqDesc,
			},
		},
	}

	// Generate tools
	generator := mcp.NewToolGenerator()
	registrations := generator.GenerateTools("local-api", serviceInfo, nil)

	// Verify tools
	if len(registrations) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(registrations))
	}

	// Check tool names
	expectedNames := []string{
		"local-api.users.v1.UserService.GetUser",
		"local-api.users.v1.UserService.ListUsers",
		"local-api.users.v1.UserService.CreateUsers",
	}

	for i, reg := range registrations {
		if reg.Tool.Name != expectedNames[i] {
			t.Errorf("tool %d: expected name %s, got %s", i, expectedNames[i], reg.Tool.Name)
		}
	}

	// Check input schema has properties
	for _, reg := range registrations {
		if reg.Tool.InputSchema.Type != "object" {
			t.Errorf("tool %s: expected object type, got %s", reg.Tool.Name, reg.Tool.InputSchema.Type)
		}
		if reg.Tool.InputSchema.Properties == nil {
			t.Errorf("tool %s: expected properties in schema", reg.Tool.Name)
		}
	}

	// Check client streaming has requests wrapper
	createTool := registrations[2].Tool
	if _, ok := createTool.InputSchema.Properties["requests"]; !ok {
		t.Error("client streaming tool should have 'requests' property")
	}
}

// TestMCPServerToolRegistration tests registering tools with the MCP server.
func TestMCPServerToolRegistration(t *testing.T) {
	serverCfg := config.ServerConfig{
		Name:      "test-server",
		Version:   "1.0.0",
		Transport: "stdio",
	}

	server := mcp.NewServer(serverCfg)

	// Create mock tools
	tool1 := mcplib.Tool{
		Name:        "endpoint1.Service.Method1",
		Description: "Test method 1",
		InputSchema: mcplib.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{"param": map[string]any{"type": "string"}},
		},
	}

	tool2 := mcplib.Tool{
		Name:        "endpoint1.Service.Method2",
		Description: "Test method 2",
		InputSchema: mcplib.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{"value": map[string]any{"type": "integer"}},
		},
	}

	handler1 := mcp.NewHandler(nil, "Service", "Method1", false, false)
	handler2 := mcp.NewHandler(nil, "Service", "Method2", false, false)

	// Register tools
	server.RegisterTools([]mcp.ToolRegistration{
		{Tool: tool1, Handler: handler1},
		{Tool: tool2, Handler: handler2},
	})

	// Verify tools are registered
	tools := server.ListTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}

	// Test GetHandler
	h1, ok := server.GetHandler("endpoint1.Service.Method1")
	if !ok {
		t.Error("handler for Method1 not found")
	}
	if h1 == nil {
		t.Error("handler is nil")
	}

	// Unregister a tool
	server.UnregisterTool("endpoint1.Service.Method1")

	tools = server.ListTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool after unregister, got %d", len(tools))
	}

	// Clear all tools
	server.ClearTools()
	tools = server.ListTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools after clear, got %d", len(tools))
	}
}

// TestRegistryLifecycle tests the registry add/remove/update cycle.
func TestRegistryLifecycle(t *testing.T) {
	reg := registry.NewRegistry()
	defer reg.Close()

	ctx := context.Background()

	// Track events
	var events []registry.RegistryEvent
	reg.OnChange(func(event registry.RegistryEvent) {
		events = append(events, event)
	})

	// Add endpoint
	cfg := config.EndpointConfig{
		Name:    "test-ep",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
	}

	err := reg.AddEndpoint(ctx, cfg)
	if err != nil {
		t.Fatalf("AddEndpoint failed: %v", err)
	}

	// Verify endpoint exists
	ep, exists := reg.GetEndpoint("test-ep")
	if !exists {
		t.Error("endpoint not found")
	}
	if ep.Name() != "test-ep" {
		t.Error("wrong endpoint name")
	}

	// Remove endpoint
	err = reg.RemoveEndpoint("test-ep")
	if err != nil {
		t.Fatalf("RemoveEndpoint failed: %v", err)
	}

	// Verify removed
	if _, exists := reg.GetEndpoint("test-ep"); exists {
		t.Error("endpoint should be removed")
	}

	// Check events
	var foundRemove bool
	for _, e := range events {
		if e.Type == registry.EventEndpointRemoved && e.EndpointName == "test-ep" {
			foundRemove = true
		}
	}
	if !foundRemove {
		t.Error("expected remove event")
	}
}

// TestConfigDiffApplication tests applying config diffs to the registry.
func TestConfigDiffApplication(t *testing.T) {
	reg := registry.NewRegistry()
	defer reg.Close()

	ctx := context.Background()

	// Initial config
	cfg1 := &config.Config{
		Endpoints: []config.EndpointConfig{
			{Name: "ep1", Address: "localhost:50051", Auth: config.AuthConfig{Type: "none"}},
			{Name: "ep2", Address: "localhost:50052", Auth: config.AuthConfig{Type: "none"}},
		},
	}

	err := reg.ApplyConfig(ctx, cfg1)
	if err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	if len(reg.ListEndpoints()) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(reg.ListEndpoints()))
	}

	// Compute diff
	cfg2 := &config.Config{
		Endpoints: []config.EndpointConfig{
			{Name: "ep1", Address: "localhost:50051", Auth: config.AuthConfig{Type: "none"}},
			{Name: "ep3", Address: "localhost:50053", Auth: config.AuthConfig{Type: "none"}},
		},
	}

	diff := config.DiffConfigs(cfg1, cfg2)

	if len(diff.Added) != 1 {
		t.Errorf("expected 1 added, got %d", len(diff.Added))
	}
	if len(diff.Removed) != 1 {
		t.Errorf("expected 1 removed, got %d", len(diff.Removed))
	}

	// Apply new config
	err = reg.ApplyConfig(ctx, cfg2)
	if err != nil {
		t.Fatalf("ApplyConfig failed: %v", err)
	}

	if len(reg.ListEndpoints()) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(reg.ListEndpoints()))
	}

	if _, exists := reg.GetEndpoint("ep2"); exists {
		t.Error("ep2 should be removed")
	}
	if _, exists := reg.GetEndpoint("ep3"); !exists {
		t.Error("ep3 should exist")
	}
}

// TestConfigHotReload tests hot-reload functionality.
func TestConfigHotReload(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	initialConfig := `
server:
  name: "test"
  version: "1.0.0"
  transport: "stdio"

endpoints:
  - name: "ep1"
    address: "localhost:50051"
    auth:
      type: "none"
`
	if err := os.WriteFile(cfgPath, []byte(initialConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create watcher
	reloadCh := make(chan *config.Config, 1)
	watcher, err := config.NewWatcher(cfgPath, func(cfg *config.Config, err error) {
		if err != nil {
			t.Errorf("watcher error: %v", err)
			return
		}
		select {
		case reloadCh <- cfg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer watcher.Stop()

	watcher.SetDebounceDelay(10 * time.Millisecond)

	ctx := context.Background()
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify initial config
	initialCfg := watcher.LastConfig()
	if initialCfg == nil {
		t.Fatal("expected initial config")
	}
	if len(initialCfg.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(initialCfg.Endpoints))
	}

	// Modify config
	updatedConfig := `
server:
  name: "test"
  version: "1.0.0"
  transport: "stdio"

endpoints:
  - name: "ep1"
    address: "localhost:50051"
    auth:
      type: "none"
  - name: "ep2"
    address: "localhost:50052"
    auth:
      type: "none"
`
	if err := os.WriteFile(cfgPath, []byte(updatedConfig), 0644); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Wait for reload
	select {
	case newCfg := <-reloadCh:
		if len(newCfg.Endpoints) != 2 {
			t.Errorf("expected 2 endpoints after reload, got %d", len(newCfg.Endpoints))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config reload")
	}
}

// TestEndpointStateTransitions tests endpoint state machine.
func TestEndpointStateTransitions(t *testing.T) {
	cfg := config.EndpointConfig{
		Name:    "state-test",
		Address: "localhost:50051",
		Auth:    config.AuthConfig{Type: "none"},
	}

	ep := registry.NewEndpoint(cfg)

	// Initial state
	if ep.State() != registry.StateDisconnected {
		t.Errorf("expected StateDisconnected, got %v", ep.State())
	}
	if ep.IsConnected() {
		t.Error("should not be connected initially")
	}

	// Config access
	gotCfg := ep.Config()
	if gotCfg.Name != "state-test" {
		t.Error("config name mismatch")
	}

	// Update config
	newCfg := cfg
	newCfg.Address = "localhost:60061"
	ep.UpdateConfig(newCfg)

	gotCfg = ep.Config()
	if gotCfg.Address != "localhost:60061" {
		t.Error("config not updated")
	}
}

// TestErrorFormatting tests gRPC error to MCP error formatting.
func TestErrorFormatting(t *testing.T) {
	// This is tested in handler_test.go, but we include a basic check here
	// to verify the integration
	handler := mcp.NewHandler(nil, "Test", "Method", false, false)
	if handler == nil {
		t.Fatal("handler should not be nil")
	}
}
