package plugin

import "context"

// SourceInterface defines the metadata for one callable interface exposed by a provider.
// This is the single source of truth for interface documentation, examples, and parameters.
type SourceInterface struct {
	Name              string                `json:"name"`               // Global unique interface name
	DisplayNameZh     string                `json:"display_name_zh"`    // Chinese display name
	SourceCode        string                `json:"source_code"`        // Data source namespace
	AssetClass        string                `json:"asset_class"`        // stock, index, fund, etc.
	MenuPath          []string              `json:"menu_path"`          // Full menu path in Web catalog
	SummaryZh         string                `json:"summary_zh"`         // Short summary for interface page header
	DescriptionZh     string                `json:"description_zh"`     // Full Chinese description body
	ParamsNoteZh      string                `json:"params_note_zh"`     // Chinese supplement for parameters
	ParamsExampleZh   string                `json:"params_example_zh"`  // Static SDK call example
	Parameters        []ParameterDefinition `json:"parameters"`         // Parameter definitions
	Fields            []FieldDefinition     `json:"fields"`             // Return field definitions
	Example           *RequestExample       `json:"example"`            // Single main example for runtime display
	ReferenceSections []ReferenceSection    `json:"reference_sections"` // Static reference tables (enums, category codes)
}

// ParameterDefinition describes one parameter of an interface.
type ParameterDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

// FieldDefinition describes one field returned by an interface.
type FieldDefinition struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Unit        string `json:"unit,omitempty"`
}

// RequestExample is a static sample response from a real request.
type RequestExample struct {
	Params map[string]interface{}   `json:"params"`
	Result []map[string]interface{} `json:"result"`
}

// ReferenceSection is a static reference table for the interface page.
type ReferenceSection struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Entries     []map[string]string `json:"entries"`
}

// SourceProvider is the bridge between a plugin package and the AxData source_request system.
// A plugin implementing this interface exposes interface metadata and a factory that produces
// an Adapter for the actual HTTP/TCP requests.
type SourceProvider interface {
	// ProviderID returns a globally unique identifier, e.g. "axdata.source.tencent"
	ProviderID() string
	// SourceCode returns the short source key, e.g. "tencent"
	SourceCode() string
	// SourceNameZh returns the Chinese display name, e.g. "腾讯财经"
	SourceNameZh() string
	// Version returns the provider version
	Version() string
	// Interfaces returns the full interface catalog.
	Interfaces() []SourceInterface
	// CreateAdapter returns an instance of the source adapter.
	CreateAdapter(options map[string]interface{}) SourceAdapter
}

// SourceAdapter is the Go-side contract for a source request adapter.
// It is called by the Collector and the HTTP API.
type SourceAdapter interface {
	Name() string
	Description() string
	// Request executes a single source request.
	// params["interface"] selects the sub-interface.
	// Additional params are passed through to the upstream.
	// Returns records as []map[string]interface{}; caller is responsible for schema mapping.
	Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error)
}
