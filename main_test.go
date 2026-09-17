package main

import "testing"

func TestPrescanDataRoot(t *testing.T) {
	const fallback = "./axdata_data"

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no flag", []string{"query", "daily"}, fallback},
		{"no args", nil, fallback},
		{"flag first", []string{"--data-root", "/tmp/a", "api", "--port", "18080"}, "/tmp/a"},
		{"flag last", []string{"api", "--port", "18080", "--data-root", "/tmp/b"}, "/tmp/b"},
		{"equals form", []string{"api", "--data-root=/tmp/c"}, "/tmp/c"},
		{"equals form after unknown", []string{"api", "--port", "18080", "--data-root=/tmp/d"}, "/tmp/d"},
		// The last value wins, matching how cobra rebinds a repeated flag.
		{"repeated flag", []string{"--data-root", "/tmp/first", "--data-root", "/tmp/second"}, "/tmp/second"},
		{"flag at the end with no value", []string{"api", "--data-root"}, fallback},
		{"value that looks like a flag", []string{"--data-root", "--something", "api"}, fallback},
		{"short form", []string{"-data-root", "/tmp/short", "api"}, "/tmp/short"},
		{"unknown flag before the real one", []string{"api", "--port", "1", "--unknown", "x", "--data-root", "/tmp/e"}, "/tmp/e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prescanDataRoot(fallback, tt.args); got != tt.want {
				t.Errorf("prescanDataRoot(%q, %v) = %q, want %q", fallback, tt.args, got, tt.want)
			}
		})
	}
}
