package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/grpc-mcp/grpc-mcp/internal/config"
)

// Client manages a gRPC connection to a single endpoint.
type Client struct {
	cfg  config.EndpointConfig
	conn *grpc.ClientConn

	mu        sync.RWMutex
	connected bool
	lastError error
}

// NewClient creates a new gRPC client for the given endpoint configuration.
func NewClient(cfg config.EndpointConfig) *Client {
	return &Client{
		cfg: cfg,
	}
}

// Connect establishes a connection to the gRPC server.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // Already connected
	}

	dialOpts, err := BuildDialOptions(c.cfg)
	if err != nil {
		c.lastError = err
		return fmt.Errorf("building dial options: %w", err)
	}

	// Add default dial options
	dialOpts = append(dialOpts,
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)), // 64MB max message
	)

	conn, err := grpc.NewClient(c.cfg.Address, dialOpts...)
	if err != nil {
		c.lastError = err
		return fmt.Errorf("creating client for %s: %w", c.cfg.Address, err)
	}

	c.conn = conn
	c.connected = true
	c.lastError = nil
	return nil
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}

	err := c.conn.Close()
	c.conn = nil
	c.connected = false
	return err
}

// Conn returns the underlying gRPC connection.
// Returns nil if not connected.
func (c *Client) Conn() *grpc.ClientConn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// IsConnected returns true if the client is connected.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected && c.conn != nil
}

// State returns the current connection state.
func (c *Client) State() connectivity.State {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil {
		return connectivity.Shutdown
	}
	return c.conn.GetState()
}

// LastError returns the last connection error, if any.
func (c *Client) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

// WaitForReady waits for the connection to become ready.
func (c *Client) WaitForReady(ctx context.Context) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if state == connectivity.Shutdown {
			return fmt.Errorf("connection shutdown")
		}

		if !conn.WaitForStateChange(ctx, state) {
			return ctx.Err()
		}
	}
}

// Config returns the endpoint configuration.
func (c *Client) Config() config.EndpointConfig {
	return c.cfg
}

// HealthCheck performs a health check on the connection.
func (c *Client) HealthCheck(ctx context.Context) error {
	conn := c.Conn()
	if conn == nil {
		return fmt.Errorf("not connected")
	}

	state := conn.GetState()
	switch state {
	case connectivity.Ready:
		return nil
	case connectivity.Idle:
		// Trigger a connection attempt
		conn.Connect()
		// Wait briefly for connection
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return c.WaitForReady(timeoutCtx)
	case connectivity.Connecting:
		timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return c.WaitForReady(timeoutCtx)
	default:
		return fmt.Errorf("connection in state: %s", state)
	}
}

// Reconnect closes and re-establishes the connection.
func (c *Client) Reconnect(ctx context.Context) error {
	if err := c.Close(); err != nil {
		return fmt.Errorf("closing connection: %w", err)
	}
	return c.Connect(ctx)
}
