package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL = "https://api.github.com/repos/poofdotnew/poof-cli"
	userAgent     = "poof-cli-update"
)

var versionRE = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)`)

// ErrManagedInstall is returned when the current binary appears to be managed
// by a package manager that should perform the update itself.
var ErrManagedInstall = errors.New("poof is managed by Homebrew")

// Client fetches Poof CLI release metadata and assets.
type Client struct {
	APIURL     string
	HTTPClient *http.Client
}

// Asset is a downloadable GitHub release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is the subset of GitHub release metadata needed for updates.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// CheckResult describes whether the current binary can be updated.
type CheckResult struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Comparable      bool   `json:"comparable"`
	ReleaseURL      string `json:"releaseUrl"`
	AssetName       string `json:"assetName,omitempty"`
}

// InstallResult describes a completed self-update.
type InstallResult struct {
	PreviousVersion string `json:"previousVersion"`
	Version         string `json:"version"`
	Path            string `json:"path"`
	AssetName       string `json:"assetName"`
}

// NewClient returns a release client with conservative network timeouts.
func NewClient() *Client {
	return &Client{
		APIURL:     defaultAPIURL,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// LatestRelease fetches GitHub's latest Poof CLI release.
func (c *Client) LatestRelease(ctx context.Context) (*Release, error) {
	body, err := c.get(ctx, strings.TrimRight(c.apiURL(), "/")+"/releases/latest")
	if err != nil {
		return nil, err
	}

	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("failed to parse release metadata: %w", err)
	}
	if rel.TagName == "" {
		return nil, fmt.Errorf("release metadata did not include a tag name")
	}
	return &rel, nil
}

// Check fetches the latest release and compares it with currentVersion.
func (c *Client) Check(ctx context.Context, currentVersion string) (*CheckResult, error) {
	rel, err := c.LatestRelease(ctx)
	if err != nil {
		return nil, err
	}
	return CheckRelease(currentVersion, rel)
}

// CheckRelease compares currentVersion with rel and selects the local platform asset.
func CheckRelease(currentVersion string, rel *Release) (*CheckResult, error) {
	asset, err := SelectAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}

	cmp, comparable := CompareVersions(currentVersion, rel.TagName)
	return &CheckResult{
		CurrentVersion:  currentVersion,
		LatestVersion:   rel.TagName,
		UpdateAvailable: comparable && cmp < 0,
		Comparable:      comparable,
		ReleaseURL:      rel.HTMLURL,
		AssetName:       asset.Name,
	}, nil
}

// InstallRelease downloads, verifies, extracts, and installs rel over exePath.
func (c *Client) InstallRelease(ctx context.Context, rel *Release, exePath string) (*InstallResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("self-update is not supported on Windows yet; download the latest release from %s", rel.HTMLURL)
	}

	exePath, err := filepath.Abs(exePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve executable path: %w", err)
	}
	if target, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = target
	}
	if isHomebrewPath(exePath) {
		return nil, ErrManagedInstall
	}

	asset, err := SelectAsset(rel.Assets, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	checksums, err := SelectChecksumAsset(rel.Assets)
	if err != nil {
		return nil, err
	}

	archiveBytes, err := c.get(ctx, asset.BrowserDownloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", asset.Name, err)
	}
	checksumBytes, err := c.get(ctx, checksums.BrowserDownloadURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", checksums.Name, err)
	}
	if err := VerifyChecksum(asset.Name, archiveBytes, string(checksumBytes)); err != nil {
		return nil, err
	}

	binaryName := "poof"
	if runtime.GOOS == "windows" {
		binaryName = "poof.exe"
	}
	newBinary, err := ExtractBinary(asset.Name, archiveBytes, binaryName)
	if err != nil {
		return nil, err
	}

	if err := replaceExecutable(exePath, newBinary); err != nil {
		return nil, err
	}

	return &InstallResult{
		Version:   rel.TagName,
		Path:      exePath,
		AssetName: asset.Name,
	}, nil
}

// CompareVersions compares semantic version-like strings by major/minor/patch.
// It returns comparable=false when either version does not start with X.Y.Z.
func CompareVersions(current, latest string) (int, bool) {
	cur, ok := parseVersion(current)
	if !ok {
		return 0, false
	}
	lat, ok := parseVersion(latest)
	if !ok {
		return 0, false
	}
	for i := range cur {
		if cur[i] < lat[i] {
			return -1, true
		}
		if cur[i] > lat[i] {
			return 1, true
		}
	}
	return 0, true
}

// SelectAsset returns the archive asset for goos/goarch.
func SelectAsset(assets []Asset, goos, goarch string) (*Asset, error) {
	needle := "_" + goos + "_" + goarch
	for i := range assets {
		name := assets[i].Name
		if !strings.Contains(name, needle) {
			continue
		}
		if goos == "windows" && strings.HasSuffix(name, ".zip") {
			return &assets[i], nil
		}
		if goos != "windows" && strings.HasSuffix(name, ".tar.gz") {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("no release asset found for %s/%s", goos, goarch)
}

// SelectChecksumAsset returns the release checksum asset.
func SelectChecksumAsset(assets []Asset) (*Asset, error) {
	for i := range assets {
		if assets[i].Name == "checksums.txt" {
			return &assets[i], nil
		}
	}
	return nil, fmt.Errorf("checksums.txt not found in latest release")
}

// VerifyChecksum validates archiveBytes against a GoReleaser checksums.txt body.
func VerifyChecksum(assetName string, archiveBytes []byte, checksums string) error {
	want := ""
	for _, line := range strings.Split(checksums, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("checksum for %s not found", assetName)
	}

	sum := sha256.Sum256(archiveBytes)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s", assetName)
	}
	return nil
}

// ExtractBinary extracts binaryName from a .tar.gz or .zip archive.
func ExtractBinary(assetName string, archiveBytes []byte, binaryName string) ([]byte, error) {
	switch {
	case strings.HasSuffix(assetName, ".tar.gz"):
		return extractTarGz(archiveBytes, binaryName)
	case strings.HasSuffix(assetName, ".zip"):
		return extractZip(archiveBytes, binaryName)
	default:
		return nil, fmt.Errorf("unsupported release archive %q", assetName)
	}
}

func (c *Client) apiURL() string {
	if c.APIURL == "" {
		return defaultAPIURL
	}
	return c.APIURL
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}
	return body, nil
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	matches := versionRE.FindStringSubmatch(strings.TrimSpace(v))
	if matches == nil {
		return out, false
	}
	for i := 1; i <= 3; i++ {
		n, err := strconv.Atoi(matches[i])
		if err != nil {
			return out, false
		}
		out[i-1] = n
	}
	return out, true
}

func extractTarGz(archiveBytes []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != binaryName {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s from archive: %w", binaryName, err)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found in archive", binaryName)
}

func extractZip(archiveBytes []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip archive: %w", err)
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || filepath.Base(file.Name) != binaryName {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open %s in archive: %w", binaryName, err)
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, fmt.Errorf("failed to read %s from archive: %w", binaryName, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("failed to close %s from archive: %w", binaryName, closeErr)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found in archive", binaryName)
}

func replaceExecutable(exePath string, newBinary []byte) error {
	info, err := os.Stat(exePath)
	if err != nil {
		return fmt.Errorf("failed to stat current executable: %w", err)
	}

	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".poof-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary executable: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(newBinary); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temporary executable: %w", err)
	}
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary executable: %w", err)
	}
	if err := os.Rename(tmpPath, exePath); err != nil {
		return fmt.Errorf("failed to replace executable: %w", err)
	}
	return nil
}

func isHomebrewPath(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(lower, "/cellar/poof/")
}
