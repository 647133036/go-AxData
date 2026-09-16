package plugin

import "context"

type mockProvider struct {
	id, code, zh, version string
	interfaces            []SourceInterface
}

func (m *mockProvider) ProviderID() string            { return m.id }
func (m *mockProvider) SourceCode() string            { return m.code }
func (m *mockProvider) SourceNameZh() string          { return m.zh }
func (m *mockProvider) Version() string               { return m.version }
func (m *mockProvider) Interfaces() []SourceInterface { return m.interfaces }
func (m *mockProvider) CreateAdapter(options map[string]interface{}) SourceAdapter {
	return &mockAdapter{name: m.code}
}

type mockAdapter struct {
	name string
}

func (m *mockAdapter) Name() string        { return m.name }
func (m *mockAdapter) Description() string { return "mock adapter" }
func (m *mockAdapter) Request(ctx context.Context, params map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, nil
}

func newMockProvider(id, code string) *mockProvider {
	return &mockProvider{
		id: id, code: code, zh: "mock", version: "1.0.0",
		interfaces: []SourceInterface{{Name: "test_iface", SourceCode: code}},
	}
}
