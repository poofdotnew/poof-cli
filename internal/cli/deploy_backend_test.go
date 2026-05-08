package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"
)

func makeBackendArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, body := range files {
		content := []byte(body)
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(content); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestValidateBackendArchive(t *testing.T) {
	validManifest := `{"entrypoint":"worker.js","wranglerVersion":"4.45.2","apiSpecPath":"generated/api-spec.json"}`

	tests := []struct {
		name    string
		files   map[string]string
		wantErr bool
	}{
		{
			name: "valid",
			files: map[string]string{
				"poof-backend-artifact.json": validManifest,
				"worker.js":                  "export default { fetch() { return new Response('ok') } }",
				"generated/api-spec.json":    `{"routes":[]}`,
			},
		},
		{
			name: "missing manifest",
			files: map[string]string{
				"worker.js": "export default {}",
			},
			wantErr: true,
		},
		{
			name: "malformed manifest",
			files: map[string]string{
				"poof-backend-artifact.json": `{not-json`,
				"worker.js":                  "export default {}",
			},
			wantErr: true,
		},
		{
			name: "missing entrypoint",
			files: map[string]string{
				"poof-backend-artifact.json": validManifest,
			},
			wantErr: true,
		},
		{
			name: "unsafe entrypoint",
			files: map[string]string{
				"poof-backend-artifact.json": `{"entrypoint":"../worker.js","wranglerVersion":"4.45.2"}`,
				"worker.js":                  "export default {}",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBackendArchive(makeBackendArchive(t, tt.files))
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestValidateBackendArchiveRejectsNonGzip(t *testing.T) {
	if err := validateBackendArchive([]byte("not a gzip archive")); err == nil {
		t.Fatal("expected non-gzip archive to fail")
	}
}

func TestValidateBackendArchiveRejectsSpecialEntries(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	manifest := []byte(`{"entrypoint":"worker.js","wranglerVersion":"4.45.2"}`)
	if err := tw.WriteHeader(&tar.Header{
		Name: "poof-backend-artifact.json",
		Mode: 0o600,
		Size: int64(len(manifest)),
	}); err != nil {
		t.Fatalf("write manifest header: %v", err)
	}
	if _, err := tw.Write(manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:     "worker.js",
		Typeflag: tar.TypeSymlink,
		Linkname: "outside.js",
		Mode:     0o777,
	}); err != nil {
		t.Fatalf("write symlink header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}

	if err := validateBackendArchive(buf.Bytes()); err == nil {
		t.Fatal("expected archive with symlink to fail")
	}
}
