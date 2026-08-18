package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		have, want string
		newer      bool
	}{
		{"v0.1.1", "v0.1.2", true},
		{"0.1.1", "v0.1.2", true},
		{"v0.1.2", "v0.1.2", false},
		{"v0.1.2", "v0.1.1", false},
		{"v0.9.0", "v0.10.0", true},
		{"v1.0.0", "v0.99.99", false},
		{"v0.1.2-beta.1", "v0.1.2", true},
		{"v0.1.2", "v0.1.2-beta.1", false},
		// A build with no version compiled in has nothing to compare, so it is
		// never nagged.
		{"dev", "v9.9.9", false},
		{"", "v9.9.9", false},
	}
	for _, c := range cases {
		if got := Newer(c.have, c.want); got != c.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.have, c.want, got, c.newer)
		}
	}
}

func TestAssetForMatchesOnPiecesNotFilename(t *testing.T) {
	release := Release{Assets: map[string]string{
		"cli_0.1.2_Darwin_arm64.tar.gz": "darwin-arm64",
		"cli_0.1.2_Linux_amd64.tar.gz":  "linux-amd64",
		"cli_0.1.2_Windows_amd64.zip":   "windows",
		"checksums.txt":                 "sums",
	}}

	if _, url, ok := release.AssetFor("darwin", "arm64"); !ok || url != "darwin-arm64" {
		t.Fatalf("darwin/arm64 resolved to %q (ok=%v)", url, ok)
	}
	if _, url, ok := release.AssetFor("linux", "amd64"); !ok || url != "linux-amd64" {
		t.Fatalf("linux/amd64 resolved to %q (ok=%v)", url, ok)
	}
	// The zip is not a thing this can install, and a platform with no build is
	// a clean miss rather than the wrong archive.
	if _, _, ok := release.AssetFor("windows", "amd64"); ok {
		t.Fatal("a .zip should not be selected")
	}
	if _, _, ok := release.AssetFor("linux", "arm64"); ok {
		t.Fatal("a platform with no build should not resolve")
	}
}

// tarball builds a .tar.gz holding a gryt binary with the given contents.
func tarball(t *testing.T, contents string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "gryt", Mode: 0o755, Size: int64(len(contents)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// serve stands up a release: the archive for this platform, and a checksums.txt
// whose contents the caller controls so the mismatch path can be exercised.
func serve(t *testing.T, archive []byte, sum string) (*httptest.Server, Release) {
	t.Helper()
	name := "cli_9.9.9_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"

	mux := http.NewServeMux()
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) { w.Write(archive) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(sum + "  " + name + "\n"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return server, Release{Tag: "v9.9.9", Assets: map[string]string{
		name:            server.URL + "/archive",
		"checksums.txt": server.URL + "/sums",
	}}
}

func TestApplyReplacesTheBinary(t *testing.T) {
	archive := tarball(t, "new binary")
	sum := sha256.Sum256(archive)
	_, release := serve(t, archive, hex.EncodeToString(sum[:]))

	path := filepath.Join(t.TempDir(), "gryt")
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Apply(context.Background(), http.DefaultClient, release, path); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("binary is %q after the update", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatal("the replacement is not executable")
	}
}

// The point of verifying: a bad download must not reach the binary.
func TestApplyLeavesTheBinaryAloneOnAChecksumMismatch(t *testing.T) {
	_, release := serve(t, tarball(t, "tampered"), "0000000000000000000000000000000000000000000000000000000000000000")

	path := filepath.Join(t.TempDir(), "gryt")
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Apply(context.Background(), http.DefaultClient, release, path); err == nil {
		t.Fatal("a checksum mismatch must fail")
	}

	got, _ := os.ReadFile(path)
	if string(got) != "old binary" {
		t.Fatalf("the existing binary was replaced anyway: %q", got)
	}
	// And nothing half-written left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("expected only the binary, found %d entries", len(entries))
	}
}

func TestApplyRefusesAPlatformWithNoBuild(t *testing.T) {
	release := Release{Tag: "v9.9.9", Assets: map[string]string{"cli_9.9.9_Plan9_386.tar.gz": "nope"}}
	if err := Apply(context.Background(), http.DefaultClient, release, filepath.Join(t.TempDir(), "gryt")); err == nil {
		t.Fatal("a release with no build for this platform must fail")
	}
}

func TestCheckParsesARelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"v9.9.9","assets":[
			{"name":"cli_9.9.9_Darwin_arm64.tar.gz","browser_download_url":"http://x/a"},
			{"name":"checksums.txt","browser_download_url":"http://x/sums"}]}`))
	}))
	defer server.Close()

	original := releasesURL
	releasesURL = server.URL
	t.Cleanup(func() { releasesURL = original })

	release, err := Check(context.Background(), http.DefaultClient)
	if err != nil {
		t.Fatal(err)
	}
	if release.Tag != "v9.9.9" {
		t.Fatalf("tag = %q", release.Tag)
	}
	if release.Assets["checksums.txt"] != "http://x/sums" {
		t.Fatalf("assets not collected: %#v", release.Assets)
	}
	if _, url, ok := release.AssetFor("darwin", "arm64"); !ok || url != "http://x/a" {
		t.Fatalf("darwin/arm64 resolved to %q (ok=%v)", url, ok)
	}
}

func TestCheckReportsAFailedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	original := releasesURL
	releasesURL = server.URL
	t.Cleanup(func() { releasesURL = original })

	if _, err := Check(context.Background(), http.DefaultClient); err == nil {
		t.Fatal("a 403 should be reported, not treated as no update")
	}
}
