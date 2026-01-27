package registry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/peter-trerotola/grpc-mcp/internal/config"
	grpcclient "github.com/peter-trerotola/grpc-mcp/internal/grpc"
	"github.com/peter-trerotola/grpc-mcp/internal/mcp"
)

// Common errors
var (
	ErrEndpointNotFound = errors.New("endpoint not found")
	ErrAlreadyExists    = errors.New("endpoint already exists")
)

// RegistryCallback is called when endpoints or tools change.
type RegistryCallback func(event RegistryEvent)

// RegistryEvent represents a change in the registry.
type RegistryEvent struct {
	Type         EventType
	EndpointName string
	Tools        []mcp.ToolRegistration
}

// EventType represents the type of registry event.
type EventType int

const (
	EventEndpointAdded EventType = iota
	EventEndpointRemoved
	EventEndpointUpdated
	EventToolsChanged
)

// Registry manages multiple gRPC endpoints and their tools.
type Registry struct {
	mu sync.RWMutex

	endpoints map[string]*Endpoint
	tools     map[string][]mcp.ToolRegistration // endpointName -> tools
	generator *mcp.ToolGenerator
	callbacks []RegistryCallback

	healthCheckInterval time.Duration
	healthCheckStop     chan struct{}
}

// NewRegistry creates a new endpoint registry.
func NewRegistry() *Registry {
	return &Registry{
		endpoints:           make(map[string]*Endpoint),
		tools:               make(map[string][]mcp.ToolRegistration),
		generator:           mcp.NewToolGenerator(),
		healthCheckInterval: 30 * time.Second,
	}
}

// AddEndpoint adds a new endpoint to the registry.
func (r *Registry) AddEndpoint(ctx context.Context, cfg config.EndpointConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.endpoints[cfg.Name]; exists {
		return fmt.Errorf("%w: %s", ErrAlreadyExists, cfg.Name)
	}

	endpoint := NewEndpoint(cfg)
	r.endpoints[cfg.Name] = endpoint

	// Connect in background
	go r.connectEndpoint(ctx, endpoint)

	return nil
}

// RemoveEndpoint removes an endpoint from the registry.
func (r *Registry) RemoveEndpoint(name string) error {
	r.mu.Lock()

	endpoint, exists := r.endpoints[name]
	if !exists {
		r.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrEndpointNotFound, name)
	}

	// Disconnect
	_ = endpoint.Disconnect()

	// Remove from registry
	delete(r.endpoints, name)
	tools := r.tools[name]
	delete(r.tools, name)

	r.mu.Unlock()

	// Notify (outside lock to avoid deadlock)
	r.notifyCallbacks(RegistryEvent{
		Type:         EventEndpointRemoved,
		EndpointName: name,
		Tools:        tools,
	})

	return nil
}

// UpdateEndpoint updates an existing endpoint's configuration.
func (r *Registry) UpdateEndpoint(ctx context.Context, cfg config.EndpointConfig) error {
	r.mu.Lock()
	endpoint, exists := r.endpoints[cfg.Name]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("%w: %s", ErrEndpointNotFound, cfg.Name)
	}

	oldConfig := endpoint.Config()

	// Check if address or auth changed (requires reconnect)
	needsReconnect := oldConfig.Address != cfg.Address ||
		!authConfigsEqual(oldConfig.Auth, cfg.Auth) ||
		!tlsConfigsEqual(oldConfig.TLS, cfg.TLS)

	endpoint.UpdateConfig(cfg)

	if needsReconnect {
		// Disconnect and reconnect
		_ = endpoint.Disconnect()
		go r.connectEndpoint(ctx, endpoint)
	} else {
		// Just refresh services
		go r.refreshEndpoint(ctx, endpoint)
	}

	return nil
}

// GetEndpoint returns an endpoint by name.
func (r *Registry) GetEndpoint(name string) (*Endpoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	endpoint, exists := r.endpoints[name]
	return endpoint, exists
}

// ListEndpoints returns all endpoint names.
func (r *Registry) ListEndpoints() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.endpoints))
	for name := range r.endpoints {
		names = append(names, name)
	}
	return names
}

// GetTools returns all tools for an endpoint.
func (r *Registry) GetTools(endpointName string) []mcp.ToolRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.tools[endpointName]
}

// GetAllTools returns all registered tools.
func (r *Registry) GetAllTools() []mcp.ToolRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var all []mcp.ToolRegistration
	for _, tools := range r.tools {
		all = append(all, tools...)
	}
	return all
}

// OnChange registers a callback for registry changes.
func (r *Registry) OnChange(cb RegistryCallback) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, cb)
}

// ApplyConfig applies a full configuration, adding/removing/updating endpoints as needed.
func (r *Registry) ApplyConfig(ctx context.Context, cfg *config.Config) error {
	r.mu.Lock()

	// Build maps of current and new endpoints
	newEndpoints := make(map[string]config.EndpointConfig)
	for _, ep := range cfg.Endpoints {
		newEndpoints[ep.Name] = ep
	}

	// Find endpoints to add, update, remove
	var toAdd, toUpdate []config.EndpointConfig
	var toRemove []string

	for name := range r.endpoints {
		if _, exists := newEndpoints[name]; !exists {
			toRemove = append(toRemove, name)
		}
	}

	for name, newCfg := range newEndpoints {
		if existing, exists := r.endpoints[name]; exists {
			// Check if config changed
			if !endpointConfigsEqual(existing.Config(), newCfg) {
				toUpdate = append(toUpdate, newCfg)
			}
		} else {
			toAdd = append(toAdd, newCfg)
		}
	}

	r.mu.Unlock()

	// Apply changes
	for _, name := range toRemove {
		if err := r.RemoveEndpoint(name); err != nil {
			return err
		}
	}

	for _, cfg := range toAdd {
		if err := r.AddEndpoint(ctx, cfg); err != nil {
			return err
		}
	}

	for _, cfg := range toUpdate {
		if err := r.UpdateEndpoint(ctx, cfg); err != nil {
			return err
		}
	}

	return nil
}

// StartHealthChecks starts periodic health checks for all endpoints.
func (r *Registry) StartHealthChecks(ctx context.Context, interval time.Duration) {
	r.mu.Lock()
	r.healthCheckInterval = interval
	r.healthCheckStop = make(chan struct{})
	r.mu.Unlock()

	go r.healthCheckLoop(ctx)
}

// StopHealthChecks stops the health check loop.
func (r *Registry) StopHealthChecks() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.healthCheckStop != nil {
		close(r.healthCheckStop)
		r.healthCheckStop = nil
	}
}

// RefreshAll forces a refresh of all connected endpoints and their tools.
func (r *Registry) RefreshAll(ctx context.Context) {
	r.mu.RLock()
	endpoints := make([]*Endpoint, 0, len(r.endpoints))
	for _, ep := range r.endpoints {
		endpoints = append(endpoints, ep)
	}
	r.mu.RUnlock()

	for _, endpoint := range endpoints {
		if endpoint.IsConnected() {
			r.refreshEndpoint(ctx, endpoint)
		} else {
			// Try to connect if not connected
			r.connectEndpoint(ctx, endpoint)
		}
	}
}

// Close disconnects all endpoints and stops health checks.
func (r *Registry) Close() error {
	r.StopHealthChecks()

	r.mu.Lock()
	defer r.mu.Unlock()

	var lastErr error
	for _, endpoint := range r.endpoints {
		if err := endpoint.Disconnect(); err != nil {
			lastErr = err
		}
	}

	r.endpoints = make(map[string]*Endpoint)
	r.tools = make(map[string][]mcp.ToolRegistration)

	return lastErr
}

// connectEndpoint connects to an endpoint and discovers services.
func (r *Registry) connectEndpoint(ctx context.Context, endpoint *Endpoint) {
	if err := endpoint.Connect(ctx); err != nil {
		// Connection failed - will retry on health check
		return
	}

	r.refreshEndpoint(ctx, endpoint)
}

// refreshEndpoint refreshes the services for an endpoint.
func (r *Registry) refreshEndpoint(ctx context.Context, endpoint *Endpoint) {
	if err := endpoint.Refresh(ctx); err != nil {
		return
	}

	// Generate tools
	services := endpoint.Services()
	invoker := endpoint.Invoker()

	var tools []mcp.ToolRegistration
	for _, svc := range services {
		svcTools := r.generator.GenerateTools(endpoint.Name(), svc, invoker)
		tools = append(tools, svcTools...)
	}

	r.mu.Lock()
	oldTools := r.tools[endpoint.Name()]
	r.tools[endpoint.Name()] = tools
	r.mu.Unlock()

	// Notify if tools changed
	if !toolsEqual(oldTools, tools) {
		r.notifyCallbacks(RegistryEvent{
			Type:         EventToolsChanged,
			EndpointName: endpoint.Name(),
			Tools:        tools,
		})
	}
}

// healthCheckLoop periodically checks endpoint health and reconnects if needed.
func (r *Registry) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(r.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.healthCheckStop:
			return
		case <-ticker.C:
			r.performHealthChecks(ctx)
		}
	}
}

// performHealthChecks checks all endpoints and reconnects if needed.
func (r *Registry) performHealthChecks(ctx context.Context) {
	r.mu.RLock()
	endpoints := make([]*Endpoint, 0, len(r.endpoints))
	for _, ep := range r.endpoints {
		endpoints = append(endpoints, ep)
	}
	r.mu.RUnlock()

	for _, endpoint := range endpoints {
		if !endpoint.IsConnected() {
			// Try to reconnect
			go r.connectEndpoint(ctx, endpoint)
			continue
		}

		// Check health
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := endpoint.HealthCheck(checkCtx)
		cancel()

		if err != nil {
			// Connection lost - disconnect and reconnect
			_ = endpoint.Disconnect()
			go r.connectEndpoint(ctx, endpoint)
		}
	}
}

// notifyCallbacks sends an event to all registered callbacks.
func (r *Registry) notifyCallbacks(event RegistryEvent) {
	r.mu.RLock()
	callbacks := make([]RegistryCallback, len(r.callbacks))
	copy(callbacks, r.callbacks)
	r.mu.RUnlock()

	for _, cb := range callbacks {
		cb(event)
	}
}

// Helper functions

func authConfigsEqual(a, b config.AuthConfig) bool {
	if a.Type != b.Type {
		return false
	}
	if a.BearerToken != b.BearerToken {
		return false
	}
	if a.APIKey.Header != b.APIKey.Header || a.APIKey.Value != b.APIKey.Value {
		return false
	}
	if a.MTLS.CertFile != b.MTLS.CertFile ||
		a.MTLS.KeyFile != b.MTLS.KeyFile ||
		a.MTLS.CAFile != b.MTLS.CAFile {
		return false
	}
	return true
}

func tlsConfigsEqual(a, b config.TLSConfig) bool {
	return a.Enabled == b.Enabled &&
		a.InsecureSkipVerify == b.InsecureSkipVerify &&
		a.CAFile == b.CAFile
}

func endpointConfigsEqual(a, b config.EndpointConfig) bool {
	if a.Name != b.Name || a.Address != b.Address {
		return false
	}
	if !authConfigsEqual(a.Auth, b.Auth) {
		return false
	}
	if !tlsConfigsEqual(a.TLS, b.TLS) {
		return false
	}
	if len(a.Exclude) != len(b.Exclude) {
		return false
	}
	for i := range a.Exclude {
		if a.Exclude[i] != b.Exclude[i] {
			return false
		}
	}
	return true
}

func toolsEqual(a, b []mcp.ToolRegistration) bool {
	if len(a) != len(b) {
		return false
	}
	aNames := make(map[string]bool)
	for _, t := range a {
		aNames[t.Tool.Name] = true
	}
	for _, t := range b {
		if !aNames[t.Tool.Name] {
			return false
		}
	}
	return true
}

// ServiceInfo is a convenience type for service information.
type ServiceInfo = grpcclient.ServiceInfo
