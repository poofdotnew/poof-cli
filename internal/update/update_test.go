package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		wantCmp int
		wantOK  bool
	}{
		{"older", "v0.1.0", "v0.2.0", -1, true},
		{"same without v", "0.2.0", "v0.2.0", 0, true},
		{"newer", "v1.0.0", "v0.9.9", 1, true},
		{"git describe suffix", "v0.2.0-3-gabc123", "v0.2.0", 0, true},
		{"dev", "dev", "v0.2.0", 0, false},
		{"commit hash", "abc123", "v0.2.0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmp, gotOK := CompareVersions(tt.current, tt.latest)
			if gotCmp != tt.wantCmp || gotOK != tt.wantOK {
				t.Fatalf("CompareVersions(%q, %q) = (%d, %v), want (%d, %v)", tt.current, tt.latest, gotCmp, gotOK, tt.wantCmp, tt.wantOK)
			}
		})
	}
}

func TestSelectAsset(t *testing.T) {
	assets := []Asset{
		{Name: "poof-cli_0.2.0_darwin_amd64.tar.gz"},
		{Name: "poof-cli_0.2.0_linux_arm64.tar.gz"},
		{Name: "poof-cli_0.2.0_windows_amd64.zip"},
	}

	asset, err := SelectAsset(assets, "linux", "arm64")
	if err != nil {
		t.Fatalf("SelectAsset returned error: %v", err)
	}
	if asset.Name != "poof-cli_0.2.0_linux_arm64.tar.gz" {
		t.Fatalf("asset name = %q", asset.Name)
	}

	if _, err := SelectAsset(assets, "freebsd", "amd64"); err == nil {
		t.Fatal("expected missing asset error")
	}
}

func TestVerifyChecksum(t *testing.T) {
	body := []byte("archive")
	sum := sha256.Sum256(body)
	checksums := fmt.Sprintf("%x  poof-cli_0.2.0_linux_arm64.tar.gz\n", sum[:])

	if err := VerifyChecksum("poof-cli_0.2.0_linux_arm64.tar.gz", body, checksums); err != nil {
		t.Fatalf("VerifyChecksum returned error: %v", err)
	}
	if err := VerifyChecksum("poof-cli_0.2.0_linux_arm64.tar.gz", []byte("bad"), checksums); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	archive := makeTarGz(t, "poof-cli/poof", []byte("binary"))

	got, err := ExtractBinary("poof-cli_0.2.0_linux_arm64.tar.gz", archive, "poof")
	if err != nil {
		t.Fatalf("ExtractBinary returned error: %v", err)
	}
	if string(got) != "binary" {
		t.Fatalf("binary = %q", string(got))
	}
}

func TestExtractBinaryZip(t *testing.T) {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, err := zw.Create("poof-cli/poof.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("binary")); err != nil {
		t.Fatalf("failed to write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	got, err := ExtractBinary("poof-cli_0.2.0_windows_amd64.zip", buf.Bytes(), "poof.exe")
	if err != nil {
		t.Fatalf("ExtractBinary returned error: %v", err)
	}
	if string(got) != "binary" {
		t.Fatalf("binary = %q", string(got))
	}
}

func TestClientCheck(t *testing.T) {
	assetName := fmt.Sprintf("poof-cli_0.2.0_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName = fmt.Sprintf("poof-cli_0.2.0_%s_%s.zip", runtime.GOOS, runtime.GOARCH)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/releases/latest" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		fmt.Fprintf(w, `{"tag_name":"v0.2.0","html_url":"https://example.com/release","assets":[{"name":%q,"browser_download_url":"https://example.com/archive"},{"name":"checksums.txt","browser_download_url":"https://example.com/checksums"}]}`, assetName)
	}))
	defer srv.Close()

	client := &Client{APIURL: srv.URL, HTTPClient: srv.Client()}
	result, err := client.Check(context.Background(), "v0.1.0")
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !result.UpdateAvailable {
		t.Fatal("expected update to be available")
	}
	if result.AssetName != assetName {
		t.Fatalf("asset name = %q", result.AssetName)
	}
}

func TestCheckWithCache(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "update_cache.json")
	cache := Cache{
		CheckedAt:       time.Now(),
		CurrentVersion:  "v0.1.0",
		LatestVersion:   "v0.2.0",
		UpdateAvailable: true,
		Comparable:      true,
		ReleaseURL:      "https://example.com/release",
		AssetName:       "poof-cli_0.2.0_linux_arm64.tar.gz",
	}
	data := []byte(fmt.Sprintf(`{"checkedAt":%q,"currentVersion":%q,"latestVersion":%q,"updateAvailable":true,"comparable":true,"releaseUrl":%q,"assetName":%q}`,
		cache.CheckedAt.Format(time.RFC3339Nano), cache.CurrentVersion, cache.LatestVersion, cache.ReleaseURL, cache.AssetName))
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		t.Fatalf("failed to write cache: %v", err)
	}

	client := &Client{
		APIURL: "http://127.0.0.1:1",
		HTTPClient: &http.Client{
			Timeout: time.Millisecond,
		},
	}
	result, err := CheckWithCache(context.Background(), client, "v0.1.0", cachePath, time.Hour)
	if err != nil {
		t.Fatalf("CheckWithCache returned error: %v", err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "v0.2.0" {
		t.Fatalf("unexpected cached result: %+v", result)
	}
}

func TestNotificationDueAndMarkNotified(t *testing.T) {
	cachePath := filepath.Join(t.TempDir(), "update_cache.json")
	result := &CheckResult{
		CurrentVersion:  "v0.1.0",
		LatestVersion:   "v0.2.0",
		UpdateAvailable: true,
		Comparable:      true,
		ReleaseURL:      "https://example.com/release",
		AssetName:       "poof-cli_0.2.0_linux_arm64.tar.gz",
	}

	if !NotificationDue(cachePath, result, time.Hour) {
		t.Fatal("expected missing cache to be notification due")
	}
	if err := MarkNotified(cachePath, result); err != nil {
		t.Fatalf("MarkNotified returned error: %v", err)
	}
	if NotificationDue(cachePath, result, time.Hour) {
		t.Fatal("expected recent notification to suppress notice")
	}

	result.LatestVersion = "v0.3.0"
	if !NotificationDue(cachePath, result, time.Hour) {
		t.Fatal("expected new latest version to be notification due")
	}
}

func makeTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("failed to close gzip: %v", err)
	}
	return buf.Bytes()
}

func TestIsHomebrewPath(t *testing.T) {
	if !isHomebrewPath("/opt/homebrew/Cellar/poof/0.1.0/bin/poof") {
		t.Fatal("expected Homebrew path")
	}
	if isHomebrewPath("/usr/local/bin/poof") {
		t.Fatal("did not expect Homebrew path")
	}
}

func TestExtractBinaryMissing(t *testing.T) {
	archive := makeTarGz(t, "poof-cli/not-poof", []byte("binary"))
	_, err := ExtractBinary("poof-cli_0.2.0_linux_arm64.tar.gz", archive, "poof")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
