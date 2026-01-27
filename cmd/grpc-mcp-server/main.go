// Package main provides the entry point for the grpc-mcp server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/spf13/cobra"

	"github.com/peter-trerotola/grpc-mcp/internal/config"
	"github.com/peter-trerotola/grpc-mcp/internal/mcp"
	"github.com/peter-trerotola/grpc-mcp/internal/registry"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		configPath string
		transport  string
		address    string
		watch      bool
	)

	rootCmd := &cobra.Command{
		Use:   "grpc-mcp-server",
		Short: "A dynamic gRPC to MCP tool bridge",
		Long: `grpc-mcp-server dynamically exposes gRPC services as MCP tools using server reflection.
No proto files required - services are discovered at runtime.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
		RunE: func(cmd *cobra.Command, args []string) error {
			return serve(cmd.Context(), configPath, transport, address, watch)
		},
	}

	rootCmd.Flags().StringVarP(&configPath, "config", "c", "grpc-mcp.yaml", "path to configuration file")
	rootCmd.Flags().StringVarP(&transport, "transport", "t", "", "transport type (stdio or sse), overrides config")
	rootCmd.Flags().StringVarP(&address, "address", "a", "", "address to listen on (for SSE transport), overrides config")
	rootCmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch config file for changes and hot-reload")

	return rootCmd.Execute()
}

func serve(ctx context.Context, configPath, transportOverride, addressOverride string, watchConfig bool) error {
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply command-line overrides
	if transportOverride != "" {
		cfg.Server.Transport = transportOverride
	}
	if addressOverride != "" {
		cfg.Server.Address = addressOverride
	}

	// Re-validate after overrides
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			fmt.Fprintf(os.Stderr, "Received signal %v, shutting down...\n", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	// Start the server
	return startServer(ctx, cfg, configPath, watchConfig)
}

func startServer(ctx context.Context, cfg *config.Config, configPath string, watchConfig bool) error {
	fmt.Fprintf(os.Stderr, "Starting %s v%s\n", cfg.Server.Name, cfg.Server.Version)
	fmt.Fprintf(os.Stderr, "Transport: %s\n", cfg.Server.Transport)
	fmt.Fprintf(os.Stderr, "Endpoints: %d configured\n", len(cfg.Endpoints))

	for _, ep := range cfg.Endpoints {
		fmt.Fprintf(os.Stderr, "  - %s (%s)\n", ep.Name, ep.Address)
	}

	// Create the MCP server
	mcpServer := mcp.NewServer(cfg.Server)

	// Create the endpoint registry
	reg := registry.NewRegistry()
	defer reg.Close()

	// Register built-in tools
	registerBuiltinTools(ctx, mcpServer, reg)

	// Register callback to update MCP tools when endpoints change
	reg.OnChange(func(event registry.RegistryEvent) {
		switch event.Type {
		case registry.EventToolsChanged, registry.EventEndpointRemoved:
			// Update MCP server tools
			updateMCPTools(mcpServer, reg)
		}
	})

	// Apply initial configuration
	if err := reg.ApplyConfig(ctx, cfg); err != nil {
		return fmt.Errorf("applying initial config: %w", err)
	}

	// Start health checks
	healthInterval := 30 * time.Second
	for _, ep := range cfg.Endpoints {
		if ep.HealthCheck.Enabled && ep.HealthCheck.Interval > 0 {
			healthInterval = ep.HealthCheck.Interval
			break
		}
	}
	reg.StartHealthChecks(ctx, healthInterval)

	// Start config watcher if enabled
	if watchConfig {
		watcher, err := config.NewWatcher(configPath, func(newCfg *config.Config, err error) {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Config reload error: %v\n", err)
				return
			}
			fmt.Fprintf(os.Stderr, "Config reloaded, applying changes...\n")
			if err := reg.ApplyConfig(ctx, newCfg); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to apply config: %v\n", err)
			}
		})
		if err != nil {
			return fmt.Errorf("creating config watcher: %w", err)
		}
		if err := watcher.Start(ctx); err != nil {
			return fmt.Errorf("starting config watcher: %w", err)
		}
		defer watcher.Stop()
		fmt.Fprintf(os.Stderr, "Config watcher started\n")
	}

	// Wait a moment for initial connections
	time.Sleep(500 * time.Millisecond)

	// Update tools with what we have
	updateMCPTools(mcpServer, reg)

	fmt.Fprintf(os.Stderr, "Server ready\n")

	// Start the MCP server based on transport
	errCh := make(chan error, 1)
	go func() {
		var err error
		switch cfg.Server.Transport {
		case "stdio":
			err = mcpServer.ServeStdio(ctx)
		case "sse":
			err = mcpServer.ServeSSE(ctx, cfg.Server.Address)
		default:
			err = fmt.Errorf("unknown transport: %s", cfg.Server.Transport)
		}
		errCh <- err
	}()

	// Wait for completion
	select {
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "Shutting down...\n")
		return nil
	case err := <-errCh:
		return err
	}
}

// updateMCPTools updates the MCP server with tools from all endpoints.
func updateMCPTools(mcpServer *mcp.Server, reg *registry.Registry) {
	// Clear existing tools
	mcpServer.ClearTools()

	// Register all tools from the registry
	tools := reg.GetAllTools()
	mcpServer.RegisterTools(tools)

	// Notify clients of the change
	mcpServer.NotifyToolsChanged()

	fmt.Fprintf(os.Stderr, "Registered %d tools\n", len(tools))
}

// registerBuiltinTools registers built-in management tools.
func registerBuiltinTools(ctx context.Context, mcpServer *mcp.Server, reg *registry.Registry) {
	// Register the rebuild-tools tool
	rebuildTool := mcplib.NewTool(
		"grpc-mcp.rebuild-tools",
		mcplib.WithDescription("Rebuild the list of available tools by re-discovering all gRPC services. Use this after adding new methods to your gRPC servers."),
	)

	mcpServer.RegisterBuiltinTool(rebuildTool, func(_ context.Context, _ mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		fmt.Fprintf(os.Stderr, "Rebuilding tools...\n")

		// Refresh all endpoints
		reg.RefreshAll(ctx)

		// Update MCP tools
		updateMCPTools(mcpServer, reg)

		// Get the count of tools
		tools := reg.GetAllTools()
		return mcplib.NewToolResultText(fmt.Sprintf("Rebuilt tool list. Found %d tools from %d endpoints.", len(tools), len(reg.ListEndpoints()))), nil
	})
}
