package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config holds all AxData configuration.
type Config struct {
	mu sync.RWMutex

	// DataRoot is the root directory for all data files.
	DataRoot string `mapstructure:"data_root"`
	// LogLevel controls logging verbosity.
	LogLevel string `mapstructure:"log_level"`
	// API port for the HTTP server.
	APIPort int `mapstructure:"api_port"`
	// DataLayers defines the data layer directories.
	DataLayers struct {
		Raw      string `mapstructure:"raw"`
		Staging  string `mapstructure:"staging"`
		Core     string `mapstructure:"core"`
		Factor   string `mapstructure:"factor"`
	} `mapstructure:"data_layers"`
	// Storage config.
	Storage struct {
		Format string `mapstructure:"format"` // parquet
		Engine string `mapstructure:"engine"` // duckdb
	} `mapstructure:"storage"`
	// Collector config.
	Collector struct {
		MaxConcurrentTasks int           `mapstructure:"max_concurrent_tasks"`
		BatchSize          int           `mapstructure:"batch_size"`
		RequestIntervalMs  int           `mapstructure:"request_interval_ms"`
		RetryCount         int           `mapstructure:"retry_count"`
		TimeoutMs          int           `mapstructure:"timeout_ms"`
	} `mapstructure:"collector"`
	// Metadata paths.
	Metadata struct {
		DBPath        string `mapstructure:"db_path"`
		CollectorPath string `mapstructure:"collector_path"`
		PluginsPath   string `mapstructure:"plugins_path"`
	} `mapstructure:"metadata"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(dataRoot string) *Config {
	c := &Config{
		DataRoot: dataRoot,
		LogLevel: "info",
		APIPort:  8080,
		Collector: struct {
			MaxConcurrentTasks int `mapstructure:"max_concurrent_tasks"`
			BatchSize          int `mapstructure:"batch_size"`
			RequestIntervalMs  int `mapstructure:"request_interval_ms"`
			RetryCount         int `mapstructure:"retry_count"`
			TimeoutMs          int `mapstructure:"timeout_ms"`
		}{
			MaxConcurrentTasks: 4,
			BatchSize:          100,
			RequestIntervalMs:  100,
			RetryCount:         3,
			TimeoutMs:          10000,
		},
	}
	c.resolvePaths()
	return c
}

func (c *Config) resolvePaths() {
	c.DataLayers.Raw = filepath.Join(c.DataRoot, "data", "raw")
	c.DataLayers.Staging = filepath.Join(c.DataRoot, "data", "staging")
	c.DataLayers.Core = filepath.Join(c.DataRoot, "data", "core")
	c.DataLayers.Factor = filepath.Join(c.DataRoot, "data", "factor")
	c.Metadata.DBPath = filepath.Join(c.DataRoot, "metadata", "axdata.sqlite")
	c.Metadata.CollectorPath = filepath.Join(c.DataRoot, "metadata", "collector.json")
	c.Metadata.PluginsPath = filepath.Join(c.DataRoot, "metadata", "plugins.json")
}

// Save writes the config to the data root.
func (c *Config) Save() error {
	dir := filepath.Join(c.DataRoot, "metadata")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create metadata dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)
}

// Load reads config from the data root, or returns a default config.
func Load(dataRoot string) (*Config, error) {
	cfgPath := filepath.Join(dataRoot, "metadata", "config.json")
	c := DefaultConfig(dataRoot)
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := json.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	return c, c.Save()
}

// DataDir returns the path to the data directory for a given layer.
func (c *Config) DataDir(layer string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	switch layer {
	case "raw":
		return c.DataLayers.Raw
	case "staging":
		return c.DataLayers.Staging
	case "factor":
		return c.DataLayers.Factor
	default:
		return c.DataLayers.Core
	}
}

// CorePath returns the Parquet file path for a core table.
func (c *Config) CorePath(table string) string {
	return filepath.Join(c.DataLayers.Core, table+".parquet")
}
