package config

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewWatcher(t *testing.T) {
	// Create a temp config file
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	w, err := NewWatcher(cfgPath, func(cfg *Config, err error) {})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Stop()

	if w.path != cfgPath {
		t.Error("unexpected path")
	}
}

func TestWatcher_Start(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var mu sync.Mutex
	var callbacks int

	w, err := NewWatcher(cfgPath, func(cfg *Config, err error) {
		mu.Lock()
		callbacks++
		mu.Unlock()
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Stop()

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Verify initial config is loaded
	cfg := w.LastConfig()
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
	if cfg.Server.Name != "test-server" {
		t.Errorf("unexpected server name: %s", cfg.Server.Name)
	}
}

func TestWatcher_FileChange(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	var mu sync.Mutex
	var receivedCfg *Config
	var receivedErr error
	callbackCh := make(chan struct{}, 10)

	w, err := NewWatcher(cfgPath, func(cfg *Config, err error) {
		mu.Lock()
		receivedCfg = cfg
		receivedErr = err
		mu.Unlock()
		callbackCh <- struct{}{}
	})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}
	defer w.Stop()

	// Reduce debounce delay for testing
	w.SetDebounceDelay(10 * time.Millisecond)

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Modify the config file
	newConfig := `
server:
  name: "updated-server"
  version: "2.0.0"
  transport: "stdio"

endpoints:
  - name: "new-endpoint"
    address: "localhost:60061"
    auth:
      type: "none"
`
	if err := os.WriteFile(cfgPath, []byte(newConfig), 0644); err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	// Wait for callback
	select {
	case <-callbackCh:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for config change callback")
	}

	mu.Lock()
	cfg := receivedCfg
	cbErr := receivedErr
	mu.Unlock()

	if cbErr != nil {
		t.Errorf("unexpected error: %v", cbErr)
	}
	if cfg == nil {
		t.Fatal("expected config in callback")
	}
	if cfg.Server.Name != "updated-server" {
		t.Errorf("unexpected server name: %s", cfg.Server.Name)
	}
	if len(cfg.Endpoints) != 1 {
		t.Errorf("expected 1 endpoint, got %d", len(cfg.Endpoints))
	}
}

func TestWatcher_Stop(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(cfgPath, []byte(testConfigYAML), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	w, err := NewWatcher(cfgPath, func(cfg *Config, err error) {})
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	ctx := context.Background()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should not error
	if err := w.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}

	// Double stop should be safe
	if err := w.Stop(); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}

func TestDiffConfigs(t *testing.T) {
	ep1 := EndpointConfig{Name: "ep1", Address: "localhost:50051"}
	ep2 := EndpointConfig{Name: "ep2", Address: "localhost:50052"}
	ep3 := EndpointConfig{Name: "ep3", Address: "localhost:50053"}
	ep2Updated := EndpointConfig{Name: "ep2", Address: "localhost:60062"}

	tests := []struct {
		name    string
		oldCfg  *Config
		newCfg  *Config
		added   []string
		removed []string
		updated []string
	}{
		{
			name:    "nil old config",
			oldCfg:  nil,
			newCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			added:   []string{"ep1", "ep2"},
			removed: nil,
			updated: nil,
		},
		{
			name:    "nil new config",
			oldCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			newCfg:  nil,
			added:   nil,
			removed: []string{"ep1", "ep2"},
			updated: nil,
		},
		{
			name:    "add endpoint",
			oldCfg:  &Config{Endpoints: []EndpointConfig{ep1}},
			newCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			added:   []string{"ep2"},
			removed: nil,
			updated: nil,
		},
		{
			name:    "remove endpoint",
			oldCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			newCfg:  &Config{Endpoints: []EndpointConfig{ep1}},
			added:   nil,
			removed: []string{"ep2"},
			updated: nil,
		},
		{
			name:    "update endpoint",
			oldCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			newCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2Updated}},
			added:   nil,
			removed: nil,
			updated: []string{"ep2"},
		},
		{
			name:    "mixed changes",
			oldCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			newCfg:  &Config{Endpoints: []EndpointConfig{ep2Updated, ep3}},
			added:   []string{"ep3"},
			removed: []string{"ep1"},
			updated: []string{"ep2"},
		},
		{
			name:    "no changes",
			oldCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			newCfg:  &Config{Endpoints: []EndpointConfig{ep1, ep2}},
			added:   nil,
			removed: nil,
			updated: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := DiffConfigs(tt.oldCfg, tt.newCfg)

			// Check added
			if len(diff.Added) != len(tt.added) {
				t.Errorf("added: expected %d, got %d", len(tt.added), len(diff.Added))
			} else {
				for i, name := range tt.added {
					if diff.Added[i].Name != name {
						t.Errorf("added[%d]: expected %s, got %s", i, name, diff.Added[i].Name)
					}
				}
			}

			// Check removed
			if len(diff.Removed) != len(tt.removed) {
				t.Errorf("removed: expected %d, got %d", len(tt.removed), len(diff.Removed))
			}

			// Check updated
			if len(diff.Updated) != len(tt.updated) {
				t.Errorf("updated: expected %d, got %d", len(tt.updated), len(diff.Updated))
			} else {
				for i, name := range tt.updated {
					if diff.Updated[i].Name != name {
						t.Errorf("updated[%d]: expected %s, got %s", i, name, diff.Updated[i].Name)
					}
				}
			}
		})
	}
}

func TestConfigDiff_HasChanges(t *testing.T) {
	tests := []struct {
		name       string
		diff       *ConfigDiff
		hasChanges bool
	}{
		{
			name:       "empty diff",
			diff:       &ConfigDiff{},
			hasChanges: false,
		},
		{
			name:       "has added",
			diff:       &ConfigDiff{Added: []EndpointConfig{{Name: "ep1"}}},
			hasChanges: true,
		},
		{
			name:       "has removed",
			diff:       &ConfigDiff{Removed: []string{"ep1"}},
			hasChanges: true,
		},
		{
			name:       "has updated",
			diff:       &ConfigDiff{Updated: []EndpointConfig{{Name: "ep1"}}},
			hasChanges: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.diff.HasChanges() != tt.hasChanges {
				t.Errorf("HasChanges() = %v, want %v", tt.diff.HasChanges(), tt.hasChanges)
			}
		})
	}
}

func TestConfigsEqual(t *testing.T) {
	cfg1 := &Config{
		Server: ServerConfig{Name: "test", Version: "1.0", Transport: "stdio"},
		Endpoints: []EndpointConfig{
			{Name: "ep1", Address: "localhost:50051"},
		},
	}

	cfg2 := &Config{
		Server: ServerConfig{Name: "test", Version: "1.0", Transport: "stdio"},
		Endpoints: []EndpointConfig{
			{Name: "ep1", Address: "localhost:50051"},
		},
	}

	cfg3 := &Config{
		Server: ServerConfig{Name: "different", Version: "1.0", Transport: "stdio"},
		Endpoints: []EndpointConfig{
			{Name: "ep1", Address: "localhost:50051"},
		},
	}

	if !configsEqual(cfg1, cfg2) {
		t.Error("expected equal configs to be equal")
	}

	if configsEqual(cfg1, cfg3) {
		t.Error("expected different configs to not be equal")
	}

	if configsEqual(nil, cfg1) {
		t.Error("expected nil and non-nil to not be equal")
	}

	if !configsEqual(nil, nil) {
		t.Error("expected nil and nil to be equal")
	}
}

const testConfigYAML = `
server:
  name: "test-server"
  version: "1.0.0"
  transport: "stdio"

endpoints:
  - name: "local-api"
    address: "localhost:50051"
    auth:
      type: "none"
`
