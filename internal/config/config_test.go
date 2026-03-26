package config

import (
	"testing"
)

func TestEnvironments(t *testing.T) {
	expected := []string{"production", "staging", "local"}
	for _, name := range expected {
		env, ok := Environments[name]
		if !ok {
			t.Errorf("missing environment: %s", name)
			continue
		}
		if env.AppID == "" {
			t.Errorf("%s: empty AppID", name)
		}
		if env.BaseURL == "" {
			t.Errorf("%s: empty BaseURL", name)
		}
		if env.AuthURL == "" {
			t.Errorf("%s: empty AuthURL", name)
		}
	}
}

func TestConfig_GetEnvironment(t *testing.T) {
	tests := []struct {
		env     string
		wantErr bool
	}{
		{"production", false},
		{"staging", false},
		{"local", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		cfg := &Config{PoofEnv: tt.env}
		_, err := cfg.GetEnvironment()
		if (err != nil) != tt.wantErr {
			t.Errorf("GetEnvironment(%q): err=%v, wantErr=%v", tt.env, err, tt.wantErr)
		}
	}
}

func TestCoalesce(t *testing.T) {
	tests := []struct {
		vals []string
		want string
	}{
		{[]string{"a", "b"}, "a"},
		{[]string{"", "b"}, "b"},
		{[]string{"", "", "c"}, "c"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}

	for _, tt := range tests {
		got := coalesce(tt.vals...)
		if got != tt.want {
			t.Errorf("coalesce(%v) = %q, want %q", tt.vals, got, tt.want)
		}
	}
}
