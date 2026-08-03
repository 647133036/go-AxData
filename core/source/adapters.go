package source

import (
	"context"
	"sync"
)

// Adapter is the interface all source adapters must implement.
type Adapter interface {
	Name() string
	Description() string
	Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error)
}

// SourceAdapters holds all registered adapters.
var SourceAdapters = make(map[string]Adapter)

var adaptersMu sync.RWMutex

// Register registers an adapter globally.
func Register(a Adapter) {
	if a == nil {
		return
	}
	adaptersMu.Lock()
	defer adaptersMu.Unlock()
	SourceAdapters[a.Name()] = a
}

// Lookup returns an adapter by name, or nil.
func Lookup(name string) Adapter {
	adaptersMu.RLock()
	defer adaptersMu.RUnlock()
	return SourceAdapters[name]
}

// List returns all registered adapter names.
func List() []string {
	adaptersMu.RLock()
	defer adaptersMu.RUnlock()
	var names []string
	for name := range SourceAdapters {
		names = append(names, name)
	}
	return names
}
