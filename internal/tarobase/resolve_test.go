package tarobase

import (
	"testing"
)

func TestParseEnvironment_AcceptsExpectedValues(t *testing.T) {
	cases := []struct {
		in   string
		want Environment
	}{
		{"", EnvDraft},
		{"draft", EnvDraft},
		{"preview", EnvPreview},
		{"production", EnvProduction},
		{"prod", EnvProduction},
	}
	for _, c := range cases {
		got, err := ParseEnvironment(c.in)
		if err != nil {
			t.Errorf("ParseEnvironment(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseEnvironment(%q): got %q want %q", c.in, got, c.want)
		}
	}
}

func TestParseEnvironment_RejectsUnknown(t *testing.T) {
	if _, err := ParseEnvironment("staging"); err == nil {
		t.Error("expected error for 'staging'")
	}
	if _, err := ParseEnvironment("local"); err == nil {
		t.Error("expected error for 'local'")
	}
}
