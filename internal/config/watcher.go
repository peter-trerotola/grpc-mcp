package config

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchCallback is called when the config file changes.
type WatchCallback func(cfg *Config, err error)

// Watcher watches a config file for changes and reloads it.
type Watcher struct {
	mu sync.Mutex

	path           string
	fsWatcher      *fsnotify.Watcher
	callback       WatchCallback
	debounceDelay  time.Duration
	debounceTimer  *time.Timer
	lastConfig     *Config
	stopCh         chan struct{}
	running        bool
}

// NewWatcher creates a new config watcher.
func NewWatcher(path string, callback WatchCallback) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		path:          path,
		fsWatcher:     fsWatcher,
		callback:      callback,
		debounceDelay: 100 * time.Millisecond,
		stopCh:        make(chan struct{}),
	}, nil
}

// SetDebounceDelay sets the debounce delay for config reloads.
func (w *Watcher) SetDebounceDelay(delay time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.debounceDelay = delay
}

// Start begins watching the config file.
func (w *Watcher) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return nil
	}
	w.running = true
	w.mu.Unlock()

	// Load initial config
	cfg, err := Load(w.path)
	if err != nil {
		return err
	}

	w.mu.Lock()
	w.lastConfig = cfg
	w.mu.Unlock()

	// Add the config file to the watcher
	if err := w.fsWatcher.Add(w.path); err != nil {
		return err
	}

	// Start watching
	go w.watch(ctx)

	return nil
}

// Stop stops watching the config file.
func (w *Watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	w.running = false
	close(w.stopCh)

	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	return w.fsWatcher.Close()
}

// LastConfig returns the last loaded configuration.
func (w *Watcher) LastConfig() *Config {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.lastConfig
}

// watch handles file system events.
func (w *Watcher) watch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			// Report error via callback
			w.callback(nil, err)
		}
	}
}

// handleEvent processes a file system event with debouncing.
func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Only handle write and create events
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Cancel any pending reload
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	// Schedule a new reload with debouncing
	w.debounceTimer = time.AfterFunc(w.debounceDelay, func() {
		w.reload()
	})
}

// reload loads the config and calls the callback if changed.
func (w *Watcher) reload() {
	cfg, err := Load(w.path)
	if err != nil {
		w.callback(nil, err)
		return
	}

	w.mu.Lock()
	lastCfg := w.lastConfig
	w.lastConfig = cfg
	w.mu.Unlock()

	// Check if config actually changed
	if lastCfg != nil && configsEqual(lastCfg, cfg) {
		return
	}

	// Notify callback
	w.callback(cfg, nil)
}

// configsEqual compares two configs for equality.
func configsEqual(a, b *Config) bool {
	if a == nil || b == nil {
		return a == b
	}

	// Compare server config
	if a.Server.Name != b.Server.Name ||
		a.Server.Version != b.Server.Version ||
		a.Server.Transport != b.Server.Transport {
		return false
	}

	// Compare endpoints
	if len(a.Endpoints) != len(b.Endpoints) {
		return false
	}

	aEndpoints := make(map[string]EndpointConfig)
	for _, ep := range a.Endpoints {
		aEndpoints[ep.Name] = ep
	}

	for _, ep := range b.Endpoints {
		aEp, exists := aEndpoints[ep.Name]
		if !exists {
			return false
		}
		if !endpointConfigsEqual(aEp, ep) {
			return false
		}
	}

	return true
}

// endpointConfigsEqual compares two endpoint configs.
func endpointConfigsEqual(a, b EndpointConfig) bool {
	if a.Name != b.Name || a.Address != b.Address {
		return false
	}

	// Compare auth
	if a.Auth.Type != b.Auth.Type ||
		a.Auth.BearerToken != b.Auth.BearerToken ||
		a.Auth.APIKey.Header != b.Auth.APIKey.Header ||
		a.Auth.APIKey.Value != b.Auth.APIKey.Value ||
		a.Auth.MTLS.CertFile != b.Auth.MTLS.CertFile ||
		a.Auth.MTLS.KeyFile != b.Auth.MTLS.KeyFile ||
		a.Auth.MTLS.CAFile != b.Auth.MTLS.CAFile {
		return false
	}

	// Compare TLS
	if a.TLS.Enabled != b.TLS.Enabled ||
		a.TLS.InsecureSkipVerify != b.TLS.InsecureSkipVerify ||
		a.TLS.CAFile != b.TLS.CAFile {
		return false
	}

	// Compare exclude patterns
	if len(a.Exclude) != len(b.Exclude) {
		return false
	}
	for i := range a.Exclude {
		if a.Exclude[i] != b.Exclude[i] {
			return false
		}
	}

	// Compare health check
	if a.HealthCheck.Enabled != b.HealthCheck.Enabled ||
		a.HealthCheck.Interval != b.HealthCheck.Interval {
		return false
	}

	return true
}

// ConfigDiff represents changes between two configurations.
type ConfigDiff struct {
	Added   []EndpointConfig
	Removed []string
	Updated []EndpointConfig
}

// DiffConfigs computes the difference between two configurations.
func DiffConfigs(oldCfg, newCfg *Config) *ConfigDiff {
	diff := &ConfigDiff{}

	if oldCfg == nil {
		// All endpoints are new
		diff.Added = newCfg.Endpoints
		return diff
	}

	if newCfg == nil {
		// All endpoints are removed
		for _, ep := range oldCfg.Endpoints {
			diff.Removed = append(diff.Removed, ep.Name)
		}
		return diff
	}

	// Build maps for comparison
	oldEndpoints := make(map[string]EndpointConfig)
	for _, ep := range oldCfg.Endpoints {
		oldEndpoints[ep.Name] = ep
	}

	newEndpoints := make(map[string]EndpointConfig)
	for _, ep := range newCfg.Endpoints {
		newEndpoints[ep.Name] = ep
	}

	// Find removed endpoints
	for name := range oldEndpoints {
		if _, exists := newEndpoints[name]; !exists {
			diff.Removed = append(diff.Removed, name)
		}
	}

	// Find added and updated endpoints
	for name, newEp := range newEndpoints {
		oldEp, exists := oldEndpoints[name]
		if !exists {
			diff.Added = append(diff.Added, newEp)
		} else if !endpointConfigsEqual(oldEp, newEp) {
			diff.Updated = append(diff.Updated, newEp)
		}
	}

	return diff
}

// HasChanges returns true if the diff contains any changes.
func (d *ConfigDiff) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Removed) > 0 || len(d.Updated) > 0
}
