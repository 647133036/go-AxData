package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Plugin represents an installed plugin.
type Plugin struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Enabled      bool     `json:"enabled"`
	Path         string   `json:"path"`
	ProviderID   string   `json:"provider_id"`
	SourceCode   string   `json:"source_code"`
	SourceNameZh string   `json:"source_name_zh"`
	Interfaces   []string `json:"interfaces"`
	Collectors   []string `json:"collectors"`
	Dependencies []string `json:"dependencies"`
}

// PluginManager manages plugin lifecycle and provider discovery.
type PluginManager struct {
	mu           sync.RWMutex
	plugins      map[string]*Plugin
	metadataPath string
	providers    map[string]SourceProvider // provider_id -> SourceProvider
}

// NewPluginManager creates a new plugin manager.
// metadataPath is the full path to the plugins.json file. It is resolved from
// the data root, so callers that change the root after construction must call
// Reload to pick up the new location.
func NewPluginManager(metadataPath string) *PluginManager {
	return &PluginManager{
		plugins:      make(map[string]*Plugin),
		metadataPath: metadataPath,
		providers:    make(map[string]SourceProvider),
	}
}

// Load reads plugin metadata from disk.
func (pm *PluginManager) Load() error {
	data, err := os.ReadFile(pm.metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var plugins []Plugin
	if err := json.Unmarshal(data, &plugins); err != nil {
		return err
	}

	for _, p := range plugins {
		pm.plugins[p.ID] = &p
	}
	return nil
}

// Reload re-reads plugin metadata from the current metadata path. Called after
// the data root changes so plugins registered from a stale location do not leak
// into the correct one.
func (pm *PluginManager) Reload() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.loadLocked()
}

// loadLocked reads plugin metadata; callers must hold pm.mu.
func (pm *PluginManager) loadLocked() error {
	data, err := os.ReadFile(pm.metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var plugins []Plugin
	if err := json.Unmarshal(data, &plugins); err != nil {
		return err
	}

	for _, p := range plugins {
		pm.plugins[p.ID] = &p
	}
	return nil
}

// SetMetadataPath repoints the manager at a different plugins.json file.
func (pm *PluginManager) SetMetadataPath(path string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.metadataPath = path
}

// Save persists plugin metadata to disk.
func (pm *PluginManager) Save() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.saveLocked()
}

// saveLocked persists plugin metadata; callers must hold pm.mu. Mutating methods
// already hold the lock, so routing them through Save would deadlock.
func (pm *PluginManager) saveLocked() error {
	var plugins []Plugin
	for _, p := range pm.plugins {
		plugins = append(plugins, *p)
	}

	data, err := json.MarshalIndent(plugins, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(pm.metadataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}

	return os.WriteFile(pm.metadataPath, data, 0644)
}

// RegisterProvider registers a SourceProvider with the manager.
// It creates a Plugin entry and caches the provider for runtime use.
func (pm *PluginManager) RegisterProvider(p SourceProvider) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.providers[p.ProviderID()] = p

	// Create/update plugin entry from provider metadata
	plugin := &Plugin{
		ID:           p.ProviderID(),
		Name:         p.SourceCode(),
		Version:      p.Version(),
		Enabled:      true,
		ProviderID:   p.ProviderID(),
		SourceCode:   p.SourceCode(),
		SourceNameZh: p.SourceNameZh(),
	}

	for _, iface := range p.Interfaces() {
		plugin.Interfaces = append(plugin.Interfaces, iface.Name)
	}

	pm.plugins[plugin.ID] = plugin
}

// GetProvider returns a registered provider by provider_id.
func (pm *PluginManager) GetProvider(providerID string) (SourceProvider, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.providers[providerID]
	return p, ok
}

// ListProviders returns all registered providers.
func (pm *PluginManager) ListProviders() []SourceProvider {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var providers []SourceProvider
	for _, p := range pm.providers {
		providers = append(providers, p)
	}
	return providers
}

// GetPlugin retrieves a plugin by ID.
func (pm *PluginManager) Get(id string) (*Plugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, exists := pm.plugins[id]
	return p, exists
}

// List returns all plugins.
func (pm *PluginManager) List() []*Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var plugins []*Plugin
	for _, p := range pm.plugins {
		plugins = append(plugins, p)
	}
	return plugins
}

// ListEnabled returns only enabled plugins.
func (pm *PluginManager) ListEnabled() []*Plugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var plugins []*Plugin
	for _, p := range pm.plugins {
		if p.Enabled {
			plugins = append(plugins, p)
		}
	}
	return plugins
}

// GetEnabledAdapter returns the adapter for an enabled provider.
// Returns nil if provider not found or disabled.
func (pm *PluginManager) GetEnabledAdapter(sourceCode string) SourceAdapter {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, p := range pm.plugins {
		if p.SourceCode == sourceCode && p.Enabled {
			provider, ok := pm.providers[p.ProviderID]
			if ok {
				return provider.CreateAdapter(nil)
			}
		}
	}
	return nil
}

// ListInterfaces returns all interface definitions from enabled providers.
func (pm *PluginManager) ListInterfaces() []SourceInterface {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var interfaces []SourceInterface
	for _, p := range pm.plugins {
		if p.Enabled {
			provider, ok := pm.providers[p.ProviderID]
			if ok {
				interfaces = append(interfaces, provider.Interfaces()...)
			}
		}
	}
	return interfaces
}

// GetInterface returns a single interface definition by name.
func (pm *PluginManager) GetInterface(name string) (*SourceInterface, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, p := range pm.plugins {
		if p.Enabled {
			provider, ok := pm.providers[p.ProviderID]
			if ok {
				for _, iface := range provider.Interfaces() {
					if iface.Name == name {
						return &iface, true
					}
				}
			}
		}
	}
	return nil, false
}

// Install installs a plugin from a directory.
func (pm *PluginManager) Install(path string) error {
	manifestPath := filepath.Join(path, "plugin.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}

	var manifest struct {
		ID           string   `json:"id"`
		Name         string   `json:"name"`
		Version      string   `json:"version"`
		Interfaces   []string `json:"interfaces"`
		Collectors   []string `json:"collectors"`
		Dependencies []string `json:"dependencies"`
	}

	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.plugins[manifest.ID]; exists {
		return fmt.Errorf("plugin already installed: %s", manifest.ID)
	}

	pm.plugins[manifest.ID] = &Plugin{
		ID:           manifest.ID,
		Name:         manifest.Name,
		Version:      manifest.Version,
		Enabled:      true,
		Path:         path,
		Interfaces:   manifest.Interfaces,
		Collectors:   manifest.Collectors,
		Dependencies: manifest.Dependencies,
	}

	return pm.saveLocked()
}

// Uninstall removes a plugin.
func (pm *PluginManager) Uninstall(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.plugins[id]; !exists {
		return fmt.Errorf("plugin not found: %s", id)
	}

	delete(pm.plugins, id)
	return pm.saveLocked()
}

// Enable enables a plugin.
func (pm *PluginManager) Enable(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, exists := pm.plugins[id]
	if !exists {
		return fmt.Errorf("plugin not found: %s", id)
	}

	p.Enabled = true
	return pm.saveLocked()
}

// Disable disables a plugin.
func (pm *PluginManager) Disable(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, exists := pm.plugins[id]
	if !exists {
		return fmt.Errorf("plugin not found: %s", id)
	}

	p.Enabled = false
	return pm.saveLocked()
}
