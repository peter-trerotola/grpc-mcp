// Package mcp provides the MCP server implementation for grpc-mcp.
package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/grpc-mcp/grpc-mcp/internal/config"
)

// Server wraps the MCP server with grpc-mcp specific functionality.
type Server struct {
	mcpServer *server.MCPServer
	config    config.ServerConfig

	mu       sync.RWMutex
	handlers map[string]*Handler // toolName -> handler
}

// NewServer creates a new MCP server with the given configuration.
func NewServer(cfg config.ServerConfig) *Server {
	mcpServer := server.NewMCPServer(
		cfg.Name,
		cfg.Version,
		server.WithToolCapabilities(true),
	)

	return &Server{
		mcpServer: mcpServer,
		config:    cfg,
		handlers:  make(map[string]*Handler),
	}
}

// MCPServer returns the underlying MCP server.
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}

// RegisterTool registers a tool with the MCP server.
func (s *Server) RegisterTool(tool mcp.Tool, handler *Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.handlers[tool.Name] = handler
	s.mcpServer.AddTool(tool, handler.Handle)
}

// RegisterTools registers multiple tools with the MCP server.
func (s *Server) RegisterTools(tools []ToolRegistration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, reg := range tools {
		s.handlers[reg.Tool.Name] = reg.Handler
		s.mcpServer.AddTool(reg.Tool, reg.Handler.Handle)
	}
}

// UnregisterTool removes a tool from the MCP server.
func (s *Server) UnregisterTool(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.handlers, name)
	s.mcpServer.DeleteTools(name)
}

// UnregisterTools removes multiple tools from the MCP server.
func (s *Server) UnregisterTools(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, name := range names {
		delete(s.handlers, name)
	}
	s.mcpServer.DeleteTools(names...)
}

// ClearTools removes all tools from the MCP server.
func (s *Server) ClearTools() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get all tool names
	names := make([]string, 0, len(s.handlers))
	for name := range s.handlers {
		names = append(names, name)
	}

	// Clear handlers
	s.handlers = make(map[string]*Handler)

	// Remove from MCP server
	if len(names) > 0 {
		s.mcpServer.DeleteTools(names...)
	}
}

// ListTools returns all registered tool names.
func (s *Server) ListTools() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.handlers))
	for name := range s.handlers {
		names = append(names, name)
	}
	return names
}

// GetHandler returns the handler for a tool.
func (s *Server) GetHandler(name string) (*Handler, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h, ok := s.handlers[name]
	return h, ok
}

// NotifyToolsChanged sends a tools/list_changed notification to all clients.
func (s *Server) NotifyToolsChanged() {
	s.mcpServer.SendNotificationToAllClients("notifications/tools/list_changed", nil)
}

// ServeStdio starts the server using stdio transport.
func (s *Server) ServeStdio(ctx context.Context) error {
	return server.ServeStdio(s.mcpServer)
}

// ServeSSE starts the server using SSE transport.
func (s *Server) ServeSSE(ctx context.Context, address string) error {
	sseServer := server.NewSSEServer(s.mcpServer,
		server.WithBaseURL(fmt.Sprintf("http://%s", address)),
	)
	return sseServer.Start(address)
}

// ToolRegistration bundles a tool with its handler for registration.
type ToolRegistration struct {
	Tool    mcp.Tool
	Handler *Handler
}
