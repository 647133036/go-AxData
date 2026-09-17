package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// Config holds all AxData configuration.
//
// Every field carries both a mapstructure tag and a json tag, and they must
// agree. config.json is decoded with encoding/json, which matches field names
// rather than mapstructure keys, so a json tag is what makes an on-disk
// "poll_interval_ms" reach PollIntervalMs. A missing json tag is a silent
// no-op: the key is dropped and the default quietly wins.
type Config struct {
	mu sync.RWMutex

	// DataRoot is the root directory for all data files.
	DataRoot string `mapstructure:"data_root" json:"data_root"`
	// LogLevel controls logging verbosity.
	LogLevel string `mapstructure:"log_level" json:"log_level"`
	// API port for the HTTP server.
	APIPort int `mapstructure:"api_port" json:"api_port"`
	// DataLayers defines the data layer directories.
	DataLayers struct {
		Raw     string `mapstructure:"raw" json:"raw"`
		Staging string `mapstructure:"staging" json:"staging"`
		Core    string `mapstructure:"core" json:"core"`
		Factor  string `mapstructure:"factor" json:"factor"`
	} `mapstructure:"data_layers" json:"data_layers"`
	// Storage config.
	Storage struct {
		Format string `mapstructure:"format" json:"format"` // parquet
		Engine string `mapstructure:"engine" json:"engine"` // duckdb
	} `mapstructure:"storage" json:"storage"`
	// Collector config.
	Collector struct {
		MaxConcurrentTasks int `mapstructure:"max_concurrent_tasks" json:"max_concurrent_tasks"`
		BatchSize          int `mapstructure:"batch_size" json:"batch_size"`
		RequestIntervalMs  int `mapstructure:"request_interval_ms" json:"request_interval_ms"`
		RetryCount         int `mapstructure:"retry_count" json:"retry_count"`
		TimeoutMs          int `mapstructure:"timeout_ms" json:"timeout_ms"`
	} `mapstructure:"collector" json:"collector"`
	// Scheduler config. Enabled is read only by `axdata api`, which is the one
	// long-lived process with a lifetime to own the poll loop; every other
	// command ignores it. `axdata collector scheduler run` starts a scheduler
	// in the foreground regardless of this flag.
	Scheduler struct {
		Enabled        bool `mapstructure:"enabled" json:"enabled"`
		PollIntervalMs int  `mapstructure:"poll_interval_ms" json:"poll_interval_ms"`
	} `mapstructure:"scheduler" json:"scheduler"`
	// Metadata paths.
	Metadata struct {
		DBPath        string `mapstructure:"db_path" json:"db_path"`
		CollectorPath string `mapstructure:"collector_path" json:"collector_path"`
		PluginsPath   string `mapstructure:"plugins_path" json:"plugins_path"`
	} `mapstructure:"metadata" json:"metadata"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(dataRoot string) *Config {
	c := &Config{
		DataRoot: dataRoot,
		LogLevel: "info",
		APIPort:  8080,
	}
	// The nested fields are assigned rather than set through anonymous struct
	// literals: a literal must repeat the field tags verbatim to be assignable,
	// so adding a tag to Config would silently break this function.
	c.Collector.MaxConcurrentTasks = 4
	c.Collector.BatchSize = 100
	c.Collector.RequestIntervalMs = 100
	c.Collector.RetryCount = 3
	c.Collector.TimeoutMs = 10000
	c.Scheduler.Enabled = false
	c.Scheduler.PollIntervalMs = 1000

	c.resolvePaths()
	return c
}

func (c *Config) resolvePaths() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DataLayers.Raw = filepath.Join(c.DataRoot, "data", "raw")
	c.DataLayers.Staging = filepath.Join(c.DataRoot, "data", "staging")
	c.DataLayers.Core = filepath.Join(c.DataRoot, "data", "core")
	c.DataLayers.Factor = filepath.Join(c.DataRoot, "data", "factor")
	c.Metadata.DBPath = filepath.Join(c.DataRoot, "metadata", "axdata.sqlite")
	c.Metadata.CollectorPath = filepath.Join(c.DataRoot, "metadata", "collector.json")
	c.Metadata.PluginsPath = filepath.Join(c.DataRoot, "metadata", "plugins.json")
}

// SetDataRoot changes the data root and re-derives every dependent path. Call
// this after the CLI flag is parsed, since paths are computed at construction.
func (c *Config) SetDataRoot(dataRoot string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DataRoot = dataRoot

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
		// Keys are normalised first so a config written before the json tags
		// existed (PascalCase) still loads instead of being dropped.
		if data, err = normalizeConfigKeys(data); err != nil {
			return nil, fmt.Errorf("normalise config: %w", err)
		}
		if err := json.Unmarshal(data, c); err != nil {
			return nil, fmt.Errorf("unmarshal config: %w", err)
		}
	}
	return c, c.Save()
}

// normalizeConfigKeys rewrites object keys as snake_case. snake_case keys pass
// through unchanged, so both the current on-disk format and the pre-json-tag
// PascalCase format decode to the same fields.
func normalizeConfigKeys(data []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	out, err := json.Marshal(mapKeysSnake(v))
	if err != nil {
		return nil, err
	}
	return out, nil
}

func mapKeysSnake(v interface{}) interface{} {
	m, ok := v.(map[string]interface{})
	if !ok {
		return v
	}
	out := make(map[string]interface{}, len(m))
	for k, val := range m {
		out[toSnakeCase(k)] = mapKeysSnake(val)
	}
	return out
}

// toSnakeCase converts CamelCase to snake_case, keeping already-snake keys and
// multi-letter acronyms intact: DataRoot -> data_root, APIPort -> api_port,
// PollIntervalMs -> poll_interval_ms, DBPath -> db_path, data_root unchanged.
func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i == 0 {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		prev := rune(s[i-1])
		next := '\x00'
		if i+1 < len(s) {
			next = rune(s[i+1])
		}
		if unicode.IsUpper(r) && (unicode.IsLower(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
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
	c.mu.RLock()
	defer c.mu.RUnlock()
	return filepath.Join(c.DataLayers.Core, table+".parquet")
}
