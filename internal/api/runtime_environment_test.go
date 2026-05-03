package api

import (
	"strings"
	"testing"
)

func TestNormalizeProjectRuntimeEnvironment(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"draft", "development"},
		{"development", "development"},
		{"dev", "development"},
		{"preview", "mainnet-preview"},
		{"mainnet-preview", "mainnet-preview"},
		{"production", "production"},
		{"prod", "production"},
	}

	for _, c := range cases {
		got, err := normalizeProjectRuntimeEnvironment(c.in)
		if err != nil {
			t.Errorf("normalizeProjectRuntimeEnvironment(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeProjectRuntimeEnvironment(%q): got %q want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeProjectRuntimeEnvironmentRejectsUnknown(t *testing.T) {
	_, err := normalizeProjectRuntimeEnvironment("local")
	if err == nil || !strings.Contains(err.Error(), "invalid environment") {
		t.Fatalf("expected invalid environment error, got %v", err)
	}
}
