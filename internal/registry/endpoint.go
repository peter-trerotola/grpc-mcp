// Package registry manages gRPC endpoint connections and tool registration.
package registry

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/peter-trerotola/grpc-mcp/internal/config"
	grpcclient "github.com/peter-trerotola/grpc-mcp/internal/grpc"
)

// ErrNotConnected is returned when an operation requires a connected endpoint.
var ErrNotConnected = errors.New("endpoint not connected")

// EndpointState represents the current state of an endpoint.
type EndpointState string

const (
	StateDisconnected EndpointState = "disconnected"
	StateConnecting   EndpointState = "connecting"
	StateConnected    EndpointState = "connected"
	StateError        EndpointState = "error"
)

// Endpoint manages a single gRPC endpoint connection.
type Endpoint struct {
	mu sync.RWMutex

	name   string
	config config.EndpointConfig
	state  EndpointState

	client     *grpcclient.Client
	reflection *grpcclient.ReflectionClient
	invoker    *grpcclient.Invoker
	services   []*grpcclient.ServiceInfo

	lastError     error
	lastConnected time.Time
	lastRefreshed time.Time
}

// NewEndpoint creates a new endpoint instance.
func NewEndpoint(cfg config.EndpointConfig) *Endpoint {
	return &Endpoint{
		name:   cfg.Name,
		config: cfg,
		state:  StateDisconnected,
	}
}

// Name returns the endpoint name.
func (e *Endpoint) Name() string {
	return e.name
}

// Config returns a copy of the endpoint configuration.
func (e *Endpoint) Config() config.EndpointConfig {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.config.Clone()
}

// State returns the current endpoint state.
func (e *Endpoint) State() EndpointState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state
}

// LastError returns the last error that occurred.
func (e *Endpoint) LastError() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastError
}

// Services returns the discovered services.
func (e *Endpoint) Services() []*grpcclient.ServiceInfo {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.services
}

// Invoker returns the gRPC invoker for this endpoint.
func (e *Endpoint) Invoker() *grpcclient.Invoker {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.invoker
}

// Connect establishes a connection to the gRPC server.
func (e *Endpoint) Connect(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.state = StateConnecting
	e.lastError = nil

	// Create client
	client := grpcclient.NewClient(e.config)
	if err := client.Connect(ctx); err != nil {
		e.state = StateError
		e.lastError = err
		return err
	}

	e.client = client
	e.reflection = grpcclient.NewReflectionClient(client.Conn())
	e.invoker = grpcclient.NewInvoker(client.Conn())
	e.state = StateConnected
	e.lastConnected = time.Now()

	return nil
}

// Disconnect closes the connection to the gRPC server.
func (e *Endpoint) Disconnect() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.client != nil {
		err := e.client.Close()
		e.client = nil
		e.reflection = nil
		e.invoker = nil
		e.services = nil
		e.state = StateDisconnected
		return err
	}

	return nil
}

// Refresh discovers services from the gRPC server using reflection.
func (e *Endpoint) Refresh(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != StateConnected || e.reflection == nil {
		return ErrNotConnected
	}

	// List services
	serviceNames, err := e.reflection.ListServices(ctx)
	if err != nil {
		e.lastError = err
		return err
	}

	// Filter excluded services
	filteredNames := filterServices(serviceNames, e.config.Exclude)

	// Describe each service
	services := make([]*grpcclient.ServiceInfo, 0, len(filteredNames))
	for _, name := range filteredNames {
		info, err := e.reflection.DescribeService(ctx, name)
		if err != nil {
			// Log warning but continue with other services
			continue
		}
		services = append(services, info)
	}

	e.services = services
	e.lastRefreshed = time.Now()
	e.lastError = nil

	return nil
}

// IsConnected returns true if the endpoint is connected.
func (e *Endpoint) IsConnected() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.state == StateConnected
}

// HealthCheck performs a health check on the connection.
func (e *Endpoint) HealthCheck(ctx context.Context) error {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()

	if client == nil {
		return ErrNotConnected
	}

	return client.HealthCheck(ctx)
}

// UpdateConfig updates the endpoint configuration.
func (e *Endpoint) UpdateConfig(cfg config.EndpointConfig) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.config = cfg
}

// filterServices removes services matching exclusion patterns.
func filterServices(services []string, excludePatterns []string) []string {
	if len(excludePatterns) == 0 {
		return services
	}

	var filtered []string
	for _, svc := range services {
		if !matchesAnyPattern(svc, excludePatterns) {
			filtered = append(filtered, svc)
		}
	}
	return filtered
}

// matchesAnyPattern checks if a service name matches any exclusion pattern.
func matchesAnyPattern(service string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(service, pattern) {
			return true
		}
	}
	return false
}

// matchPattern matches a service name against a pattern.
// Supports * as a wildcard.
func matchPattern(service, pattern string) bool {
	// Simple wildcard matching
	if pattern == "*" {
		return true
	}

	// Check for suffix wildcard (e.g., "grpc.reflection.*")
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(service) >= len(prefix) && service[:len(prefix)] == prefix
	}

	// Check for prefix wildcard (e.g., "*.internal.Service")
	if len(pattern) > 0 && pattern[0] == '*' {
		suffix := pattern[1:]
		return len(service) >= len(suffix) && service[len(service)-len(suffix):] == suffix
	}

	// Exact match
	return service == pattern
}
