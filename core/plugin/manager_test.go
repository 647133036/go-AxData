package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewPluginManager(t *testing.T) {
	pm := NewPluginManager("/tmp/test-plugins")
	if pm.plugins == nil {
		t.Fatal("plugins map should not be nil")
	}
	if pm.providers == nil {
		t.Fatal("providers map should not be nil")
	}
	if pm.installDir != "/tmp/test-plugins" {
		t.Fatalf("installDir: got %s, want /tmp/test-plugins", pm.installDir)
	}
}

func TestPluginManager_RegisterProvider(t *testing.T) {
	pm := NewPluginManager("/tmp/test-plugins")
	p := newMockProvider("axdata.source.mock", "mock")
	pm.RegisterProvider(p)

	_, ok := pm.GetProvider("axdata.source.mock")
	if !ok {
		t.Fatal("Provider not found after registration")
	}

	plugin, ok := pm.Get("axdata.source.mock")
	if !ok {
		t.Fatal("Plugin not found after registration")
	}
	if plugin.Name != "mock" {
		t.Errorf("Name: got %s, want mock", plugin.Name)
	}
	if !plugin.Enabled {
		t.Error("Plugin should be enabled by default")
	}
	if len(plugin.Interfaces) != 1 || plugin.Interfaces[0] != "test_iface" {
		t.Errorf("Interfaces: got %v", plugin.Interfaces)
	}
}

func TestPluginManager_Get(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	p := newMockProvider("axdata.source.mock", "mock")
	pm.RegisterProvider(p)

	plugin, ok := pm.Get("axdata.source.mock")
	if !ok {
		t.Fatal("Plugin should be found")
	}
	if plugin.Version != "1.0.0" {
		t.Errorf("Version: got %s, want 1.0.0", plugin.Version)
	}

	_, ok = pm.Get("nonexistent")
	if ok {
		t.Fatal("Nonexistent plugin should return false")
	}
}

func TestPluginManager_List(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	pm.RegisterProvider(newMockProvider("p1", "src1"))
	pm.RegisterProvider(newMockProvider("p2", "src2"))

	plugins := pm.List()
	if len(plugins) != 2 {
		t.Fatalf("Expected 2 plugins, got %d", len(plugins))
	}
}

func TestPluginManager_ListEnabled(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	pm.RegisterProvider(newMockProvider("p1", "src1"))
	pm.RegisterProvider(newMockProvider("p2", "src2"))

	_ = pm.Disable("p2")

	enabled := pm.ListEnabled()
	if len(enabled) != 1 {
		t.Fatalf("Expected 1 enabled plugin, got %d", len(enabled))
	}
}

func TestPluginManager_InstallAndUninstall(t *testing.T) {
	tmpDir := t.TempDir()
	pluginDir := filepath.Join(tmpDir, "myplugin")
	os.MkdirAll(pluginDir, 0755)

	manifest := map[string]interface{}{
		"id":       "test.plugin",
		"name":     "Test Plugin",
		"version":  "1.0.0",
		"interfaces": []string{"iface1", "iface2"},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0644)

	pm := NewPluginManager(tmpDir)

	err := pm.Install(pluginDir)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	plugin, ok := pm.Get("test.plugin")
	if !ok {
		t.Fatal("Plugin not found after install")
	}
	if plugin.Version != "1.0.0" {
		t.Errorf("Version: got %s, want 1.0.0", plugin.Version)
	}

	err = pm.Install(pluginDir)
	if err == nil {
		t.Fatal("Double install should fail")
	}

	err = pm.Uninstall("test.plugin")
	if err != nil {
		t.Fatalf("Uninstall failed: %v", err)
	}
	_, ok = pm.Get("test.plugin")
	if ok {
		t.Fatal("Plugin should not exist after uninstall")
	}

	err = pm.Uninstall("nonexistent")
	if err == nil {
		t.Fatal("Uninstall of nonexistent plugin should fail")
	}
}

func TestPluginManager_EnableDisable(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewPluginManager(tmpDir)
	pm.RegisterProvider(newMockProvider("p1", "src1"))

	plugin, _ := pm.Get("p1")
	if !plugin.Enabled {
		t.Fatal("Should be enabled by default")
	}

	err := pm.Disable("p1")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	plugin, _ = pm.Get("p1")
	if plugin.Enabled {
		t.Fatal("Should be disabled")
	}

	err = pm.Enable("p1")
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	plugin, _ = pm.Get("p1")
	if !plugin.Enabled {
		t.Fatal("Should be enabled again")
	}

	err = pm.Enable("nonexistent")
	if err == nil {
		t.Fatal("Enable nonexistent should fail")
	}
	err = pm.Disable("nonexistent")
	if err == nil {
		t.Fatal("Disable nonexistent should fail")
	}
}

func TestPluginManager_GetInterface(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	pm.RegisterProvider(newMockProvider("p1", "src1"))

	iface, ok := pm.GetInterface("test_iface")
	if !ok {
		t.Fatal("Interface should be found")
	}
	if iface.Name != "test_iface" {
		t.Errorf("Name: got %s, want test_iface", iface.Name)
	}

	_, ok = pm.GetInterface("nonexistent")
	if ok {
		t.Fatal("Nonexistent interface should not be found")
	}
}

func TestPluginManager_GetEnabledAdapter(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	pm.RegisterProvider(newMockProvider("p1", "src1"))

	adapter := pm.GetEnabledAdapter("src1")
	if adapter == nil {
		t.Fatal("Adapter should not be nil for enabled provider")
	}
	if adapter.Name() != "src1" {
		t.Errorf("Adapter name: got %s, want src1", adapter.Name())
	}

	// Disable and check
	_ = pm.Disable("p1")
	adapter = pm.GetEnabledAdapter("src1")
	if adapter != nil {
		t.Fatal("Adapter should be nil for disabled provider")
	}

	adapter = pm.GetEnabledAdapter("nonexistent")
	if adapter != nil {
		t.Fatal("Adapter should be nil for nonexistent provider")
	}
}

func TestPluginManager_ListProviders(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	pm.RegisterProvider(newMockProvider("p1", "src1"))
	pm.RegisterProvider(newMockProvider("p2", "src2"))

	providers := pm.ListProviders()
	if len(providers) != 2 {
		t.Fatalf("Expected 2 providers, got %d", len(providers))
	}
}

func TestPluginManager_GetProvider(t *testing.T) {
	pm := NewPluginManager("/tmp/test")
	pm.RegisterProvider(newMockProvider("p1", "src1"))

	provider, ok := pm.GetProvider("p1")
	if !ok {
		t.Fatal("Provider should be found")
	}
	if provider.ProviderID() != "p1" {
		t.Errorf("ProviderID: got %s, want p1", provider.ProviderID())
	}

	_, ok = pm.GetProvider("nonexistent")
	if ok {
		t.Fatal("Nonexistent provider should return false")
	}
}

func TestPluginManager_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewPluginManager(tmpDir)

	provider := newMockProvider("p1", "src1")
	pm.RegisterProvider(provider)

	// Disable to verify Load round-trip
	_ = pm.Disable("p1")

	pm2 := NewPluginManager(tmpDir)
	err := pm2.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	plugin, ok := pm2.Get("p1")
	if !ok {
		t.Fatal("Plugin should be loaded from disk")
	}
	if plugin.Enabled {
		t.Fatal("Plugin should be disabled after Load")
	}
	if plugin.Name != "src1" {
		t.Errorf("Name: got %s, want src1", plugin.Name)
	}
}
