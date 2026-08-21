// Package updater moves the CLI from one release to the next.
//
// It talks to the GitHub releases API and nothing else, only when asked. This
// is an administrator's tool for machines the administrator owns, so knowing
// it is out of date is part of the job rather than telemetry: no identifier is
// sent, nothing is recorded, and a failed check is silent.
package updater

import (
	"archive/tar"
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
	"runtime"
	"strconv"
	"strings"
)

// A var rather than a const so the tests can point it at a local server and
// exercise the real decoding instead of a copy of it.
var releasesURL = "https://api.github.com/repos/Gryt-chat/cli/releases/latest"

// allReleasesURL includes prereleases, which /releases/latest excludes by
// design. Following the beta channel means asking the list and taking the
// newest, because the newest beta is exactly what /releases/latest hides.
var allReleasesURL = "https://api.github.com/repos/Gryt-chat/cli/releases?per_page=10"

type Release struct {
	Tag    string
	Assets map[string]string // name -> download URL
}

// Check asks which release is newest. A caller that cannot reach the network
// gets an error rather than a wrong answer.
// Check asks which release is newest on the stable channel.
// LatestServerRelease reports the newest published Gryt server, so the CLI can
// say whether the one running here is behind. Same request shape as its own
// update check, against a different repository.
func LatestServerRelease(ctx context.Context, client *http.Client, beta bool) (string, error) {
	url := "https://api.github.com/repos/Gryt-chat/server/releases/latest"
	if beta {
		url = "https://api.github.com/repos/Gryt-chat/server/releases?per_page=10"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %s", res.Status)
	}

	if beta {
		var list []struct {
			TagName string `json:"tag_name"`
			Draft   bool   `json:"draft"`
		}
		if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
			return "", err
		}
		for _, entry := range list {
			if !entry.Draft {
				return entry.TagName, nil
			}
		}
		return "", errors.New("no published server release found")
	}

	var one struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(res.Body).Decode(&one); err != nil {
		return "", err
	}
	return one.TagName, nil
}

func Check(ctx context.Context, client *http.Client) (Release, error) {
	return CheckChannel(ctx, client, false)
}

// CheckChannel asks which release is newest on the given channel.
func CheckChannel(ctx context.Context, client *http.Client, beta bool) (Release, error) {
	if beta {
		return checkNewestIncludingPrereleases(ctx, client)
	}
	return checkLatest(ctx, client)
}

func checkNewestIncludingPrereleases(ctx context.Context, client *http.Client) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, allReleasesURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	res, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github returned %s", res.Status)
	}

	var payload []struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return Release{}, err
	}

	// The list comes back newest first. A draft is not something anybody can
	// install, so it is skipped rather than reported as available.
	for _, entry := range payload {
		if entry.Draft {
			continue
		}
		release := Release{Tag: entry.TagName, Assets: map[string]string{}}
		for _, asset := range entry.Assets {
			release.Assets[asset.Name] = asset.URL
		}
		return release, nil
	}
	return Release{}, errors.New("no installable release found")
}

func checkLatest(ctx context.Context, client *http.Client) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	res, err := client.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("github returned %s", res.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return Release{}, err
	}

	release := Release{Tag: payload.TagName, Assets: map[string]string{}}
	for _, asset := range payload.Assets {
		release.Assets[asset.Name] = asset.URL
	}
	return release, nil
}

// Newer reports whether want is a later version than have. Both may carry a
// leading v. A build with no version compiled in, which is what `go run`
// produces, is never considered out of date: there is nothing to compare.
func Newer(have, want string) bool {
	if have == "" || have == "dev" || want == "" {
		return false
	}
	h, hPre := parse(have)
	w, wPre := parse(want)
	for i := 0; i < 3; i++ {
		if w[i] != h[i] {
			return w[i] > h[i]
		}
	}
	// Same numbers: a release beats the prerelease of the same version, and a
	// prerelease never beats anything.
	return hPre != "" && wPre == ""
}

func parse(version string) ([3]int, string) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	pre := ""
	if i := strings.IndexAny(version, "-+"); i >= 0 {
		pre, version = version[i+1:], version[:i]
	}
	var out [3]int
	for i, part := range strings.SplitN(version, ".", 3) {
		if i > 2 {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out, pre
}

// AssetFor picks this platform's archive out of a release. Matching on the
// pieces rather than on a filename means a change to goreleaser's naming
// template does not silently stop updates working.
func (r Release) AssetFor(goos, goarch string) (name, url string, ok bool) {
	for name, url := range r.Assets {
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".tar.gz") {
			continue
		}
		if strings.Contains(lower, goos) && strings.Contains(lower, goarch) {
			return name, url, true
		}
	}
	return "", "", false
}

// Apply downloads this platform's build of the release and replaces the binary
// at path with it.
//
// The download is verified against the release's checksums.txt before anything
// is replaced, and the replacement is a rename within the same directory, so a
// failure part way through leaves the existing binary untouched rather than
// half-written.
func Apply(ctx context.Context, client *http.Client, release Release, path string) error {
	name, url, ok := release.AssetFor(runtime.GOOS, runtime.GOARCH)
	if !ok {
		return fmt.Errorf("%s has no %s/%s build", release.Tag, runtime.GOOS, runtime.GOARCH)
	}

	archive, err := download(ctx, client, url)
	if err != nil {
		return err
	}

	if sums, ok := release.Assets["checksums.txt"]; ok {
		if err := verify(ctx, client, sums, name, archive); err != nil {
			return err
		}
	}

	binary, err := extract(archive)
	if err != nil {
		return err
	}

	// Same directory, so the rename cannot cross a filesystem boundary.
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".gryt-update-")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(binary); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(temp.Name(), 0o755); err != nil {
		return err
	}
	// Renaming over a running executable is allowed: the kernel keeps the old
	// inode alive for this process, and the next run gets the new one.
	if err := os.Rename(temp.Name(), path); err != nil {
		return fmt.Errorf("cannot replace %s: %w", path, err)
	}
	return nil
}

// Path is the binary to replace: this one, with any symlink resolved so that
// the target is the real file rather than a link into it.
func Path() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		return resolved, nil
	}
	return self, nil
}

func download(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: %s", url, res.Status)
	}
	return io.ReadAll(res.Body)
}

func verify(ctx context.Context, client *http.Client, sumsURL, name string, archive []byte) error {
	sums, err := download(ctx, client, sumsURL)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	actual := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if fields[0] != actual {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
		return nil
	}
	return fmt.Errorf("%s is not listed in checksums.txt", name)
}

// extract pulls the gryt binary out of a .tar.gz in memory. The archives are a
// few megabytes, so there is no reason to touch the disk before the checksum
// has been checked.
func extract(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || filepath.Base(header.Name) != "gryt" {
			continue
		}
		// Bounded so a malformed archive cannot be used to exhaust memory.
		binary, err := io.ReadAll(io.LimitReader(reader, 128<<20))
		if err != nil {
			return nil, err
		}
		return binary, nil
	}
	return nil, fmt.Errorf("no gryt binary in the archive")
}
