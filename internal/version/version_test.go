package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestShort(t *testing.T) {
	got := Short()
	if got != Version {
		t.Errorf("Short() = %q, want %q", got, Version)
	}
}

func TestInfo(t *testing.T) {
	info := Info()

	if !strings.Contains(info, Version) {
		t.Error("Info() missing version")
	}
	if !strings.Contains(info, Commit) {
		t.Error("Info() missing commit")
	}
	if !strings.Contains(info, Date) {
		t.Error("Info() missing date")
	}
	if !strings.Contains(info, runtime.Version()) {
		t.Error("Info() missing Go version")
	}
	if !strings.Contains(info, runtime.GOOS) {
		t.Error("Info() missing OS")
	}
	if !strings.Contains(info, runtime.GOARCH) {
		t.Error("Info() missing arch")
	}
}

func TestInfo_Format(t *testing.T) {
	info := Info()
	if !strings.HasPrefix(info, "poof ") {
		t.Errorf("Info() should start with 'poof ', got: %s", info)
	}
	if !strings.Contains(info, "commit:") {
		t.Error("Info() missing 'commit:' label")
	}
	if !strings.Contains(info, "built:") {
		t.Error("Info() missing 'built:' label")
	}
	if !strings.Contains(info, "go:") {
		t.Error("Info() missing 'go:' label")
	}
	if !strings.Contains(info, "os/arch:") {
		t.Error("Info() missing 'os/arch:' label")
	}
}
