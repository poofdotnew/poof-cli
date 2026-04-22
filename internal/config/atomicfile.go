package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a same-directory temp file + rename.
//
// The rename is atomic on POSIX, so concurrent readers always observe either
// the old contents or the new contents — never a partial write. This matters
// for files in ~/.poof/ that two CLI invocations may touch simultaneously
// (tokens.json, tarobase-sessions.json): without this, a reader hitting the
// file mid-write sees truncated JSON and fails to parse.
//
// The in-process sync.Mutex that serializes read-modify-write in each cache
// stays important for correctness (preventing a lost update within one
// process); WriteFileAtomic is the cross-process half.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir, name := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	// If anything below fails, best-effort remove the orphan temp file. A
	// successful Rename makes this a no-op.
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}
