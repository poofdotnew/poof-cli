package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic_WritesContentsAndMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub-does-not-exist-yet", "target.json")
	// Parent must exist; the helper only does temp-in-same-dir + rename, it
	// does not create missing parents. Mirror what callers actually do.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("hello\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello\n" {
		t.Errorf("content: got %q, want %q", got, "hello\n")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %o, want 0600", info.Mode().Perm())
	}
	// No leftover temp files in the directory.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected 1 entry, got %v", names)
	}
}

func TestWriteFileAtomic_OverwriteReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if err := WriteFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("atomic write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("content: got %q, want %q", got, "new")
	}
}
