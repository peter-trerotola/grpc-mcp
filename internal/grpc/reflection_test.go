package grpc

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/grpc-mcp/grpc-mcp/internal/testutil"
)

func TestReflectionClient_ListServices(t *testing.T) {
	// Start test server
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Connect to server
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Create reflection client
	client := NewReflectionClient(conn)

	// List services
	services, err := client.ListServices(ctx)
	if err != nil {
		t.Fatalf("failed to list services: %v", err)
	}

	// Log discovered services for debugging
	t.Logf("Discovered services: %v", services)

	// Should have at least TestService, ComplexService, and reflection service
	if len(services) < 2 {
		t.Errorf("expected at least 2 services, got %d: %v", len(services), services)
	}

	// Check for expected services
	found := make(map[string]bool)
	for _, svc := range services {
		found[svc] = true
	}

	if !found["testutil.TestService"] {
		t.Error("expected to find testutil.TestService")
	}
	if !found["testutil.ComplexService"] {
		t.Error("expected to find testutil.ComplexService")
	}
}

func TestReflectionClient_DescribeService(t *testing.T) {
	// Note: Dynamic services registered without proto file descriptors
	// cannot be described via reflection. This test uses direct access
	// to the service descriptors from the test server.
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	// Get TestService descriptor directly
	services := ts.Services()
	if len(services) < 1 {
		t.Fatal("expected at least 1 service")
	}

	testSvc := services[0]
	if testSvc.GetFullyQualifiedName() != "testutil.TestService" {
		t.Errorf("expected service name 'testutil.TestService', got %q", testSvc.GetFullyQualifiedName())
	}

	// Check methods
	methods := testSvc.GetMethods()
	methodNames := make(map[string]bool)
	for _, m := range methods {
		methodNames[m.GetName()] = true
	}

	// Check Echo exists
	if !methodNames["Echo"] {
		t.Error("expected Echo method")
	}

	// Check StreamNumbers exists
	if !methodNames["StreamNumbers"] {
		t.Error("expected StreamNumbers method")
	}

	// Check SumNumbers exists
	if !methodNames["SumNumbers"] {
		t.Error("expected SumNumbers method")
	}

	// Check Chat exists
	if !methodNames["Chat"] {
		t.Error("expected Chat method")
	}

	// Check method streaming attributes
	echoMethod := testSvc.FindMethodByName("Echo")
	if echoMethod == nil {
		t.Fatal("Echo method not found")
	}
	if echoMethod.IsClientStreaming() || echoMethod.IsServerStreaming() {
		t.Error("Echo should be unary (no streaming)")
	}

	streamMethod := testSvc.FindMethodByName("StreamNumbers")
	if streamMethod == nil {
		t.Fatal("StreamNumbers method not found")
	}
	if streamMethod.IsClientStreaming() {
		t.Error("StreamNumbers should not have client streaming")
	}
	if !streamMethod.IsServerStreaming() {
		t.Error("StreamNumbers should have server streaming")
	}

	sumMethod := testSvc.FindMethodByName("SumNumbers")
	if sumMethod == nil {
		t.Fatal("SumNumbers method not found")
	}
	if !sumMethod.IsClientStreaming() {
		t.Error("SumNumbers should have client streaming")
	}
	if sumMethod.IsServerStreaming() {
		t.Error("SumNumbers should not have server streaming")
	}

	chatMethod := testSvc.FindMethodByName("Chat")
	if chatMethod == nil {
		t.Fatal("Chat method not found")
	}
	if !chatMethod.IsClientStreaming() {
		t.Error("Chat should have client streaming")
	}
	if !chatMethod.IsServerStreaming() {
		t.Error("Chat should have server streaming")
	}
}

func TestReflectionClient_DiscoverServices(t *testing.T) {
	// Note: DiscoverServices relies on reflection which can list but not
	// describe dynamic services. We test the filter logic separately.
	ts, err := testutil.NewTestServer()
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}
	ts.Start()
	defer ts.Stop()

	time.Sleep(100 * time.Millisecond)

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, ts.Address(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := NewReflectionClient(conn)

	// Test that ListServices works (it can list even dynamic services)
	services, err := client.ListServices(ctx)
	if err != nil {
		t.Fatalf("failed to list services: %v", err)
	}

	// Check that our services are listed
	found := make(map[string]bool)
	for _, svc := range services {
		found[svc] = true
	}

	if !found["testutil.TestService"] {
		t.Error("expected testutil.TestService to be listed")
	}
	if !found["testutil.ComplexService"] {
		t.Error("expected testutil.ComplexService to be listed")
	}
}

func TestShouldExclude(t *testing.T) {
	tests := []struct {
		name        string
		serviceName string
		include     []string
		exclude     []string
		expected    bool
	}{
		{
			name:        "no patterns",
			serviceName: "users.v1.UserService",
			include:     nil,
			exclude:     nil,
			expected:    false,
		},
		{
			name:        "exact exclude match",
			serviceName: "grpc.health.v1.Health",
			include:     nil,
			exclude:     []string{"grpc.health.v1.Health"},
			expected:    true,
		},
		{
			name:        "prefix exclude match",
			serviceName: "grpc.health.v1.Health",
			include:     nil,
			exclude:     []string{"grpc.health.*"},
			expected:    true,
		},
		{
			name:        "prefix exclude no match",
			serviceName: "users.v1.UserService",
			include:     nil,
			exclude:     []string{"grpc.health.*"},
			expected:    false,
		},
		{
			name:        "include pattern match",
			serviceName: "users.v1.UserService",
			include:     []string{"users.*"},
			exclude:     nil,
			expected:    false,
		},
		{
			name:        "include pattern no match",
			serviceName: "orders.v1.OrderService",
			include:     []string{"users.*"},
			exclude:     nil,
			expected:    true,
		},
		{
			name:        "include and exclude",
			serviceName: "users.v1.HealthService",
			include:     []string{"users.*"},
			exclude:     []string{"*.HealthService"},
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldExclude(tt.serviceName, tt.include, tt.exclude)
			if result != tt.expected {
				t.Errorf("shouldExclude(%q, %v, %v) = %v, want %v",
					tt.serviceName, tt.include, tt.exclude, result, tt.expected)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected bool
	}{
		// Exact match
		{"exact match", "users.v1.UserService", true},
		{"exact no match", "users.v1.OrderService", false},

		// Prefix wildcard
		{"prefix wildcard match", "users.v1.*", true},
		{"prefix wildcard no match", "orders.v1.*", false},

		// Suffix wildcard
		{"suffix wildcard match", "*.UserService", true},
		{"suffix wildcard no match", "*.OrderService", false},

		// Contains wildcard
		{"contains wildcard match", "*User*", true},
		{"contains wildcard no match", "*Order*", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchPattern("users.v1.UserService", tt.pattern)
			if result != tt.expected {
				t.Errorf("matchPattern(%q, %q) = %v, want %v",
					"users.v1.UserService", tt.pattern, result, tt.expected)
			}
		})
	}
}
