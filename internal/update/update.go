// Package update checks GitHub Releases for a newer version and (on desktop)
// downloads + verifies the platform installer before handing it off to run.
//
// The update channel is a supply-chain path into a credential vault, so a
// download is ALWAYS verified against the release's SHA256SUMS before it is ever
// executed. The check itself is best-effort: network errors, a private/missing
// repo, or a malformed response never fail the app — they just yield "no update".
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cacheTTL bounds how often we hit the GitHub API (its anon limit is 60/hr/IP).
const cacheTTL = 6 * time.Hour

// Status is the public result of a check (JSON-encoded for the frontend). Assets
// is kept server-side only (not serialized) for the apply step.
type Status struct {
	Current         string            `json:"current"`
	Latest          string            `json:"latest"`
	UpdateAvailable bool              `json:"updateAvailable"`
	URL             string            `json:"url"`
	Notes           string            `json:"notes"`
	Checked         time.Time         `json:"checked"`
	Assets          map[string]string `json:"-"`
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Checker caches the last successful check so navigation/polling doesn't stampede
// the GitHub API.
type Checker struct {
	client  *http.Client
	mu      sync.Mutex
	cached  *Status
	fetched time.Time
	repo    string
}

func New() *Checker {
	return &Checker{client: &http.Client{Timeout: 15 * time.Second}}
}

// Check returns the update status for repo ("owner/repo") against the current
// version. It serves a cached result unless stale, the repo changed, or force is
// set. On any fetch problem it returns a "no update" Status plus the error (for
// logging) — callers should treat that as non-fatal.
func (c *Checker) Check(ctx context.Context, repo, current string, force bool) (Status, error) {
	c.mu.Lock()
	if !force && c.cached != nil && c.repo == repo && time.Since(c.fetched) < cacheTTL {
		st := *c.cached
		c.mu.Unlock()
		return st, nil
	}
	c.mu.Unlock()

	none := Status{Current: current, UpdateAvailable: false, Checked: time.Now()}

	rel, err := c.fetchLatest(ctx, repo)
	if err != nil {
		return none, err
	}
	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	st := Status{
		Current:         current,
		Latest:          latest,
		UpdateAvailable: isNewer(latest, current),
		URL:             rel.HTMLURL,
		Notes:           rel.Body,
		Checked:         time.Now(),
		Assets:          map[string]string{},
	}
	for _, a := range rel.Assets {
		st.Assets[a.Name] = a.URL
	}

	c.mu.Lock()
	c.cached = &st
	c.fetched = st.Checked
	c.repo = repo
	c.mu.Unlock()
	return st, nil
}

// Cached returns the last cached status, if any (no network).
func (c *Checker) Cached() (Status, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cached == nil {
		return Status{}, false
	}
	return *c.cached, true
}

func (c *Checker) fetchLatest(ctx context.Context, repo string) (*ghRelease, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" || !strings.Contains(repo, "/") {
		return nil, fmt.Errorf("invalid update repo %q", repo)
	}
	url := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 404 = private/no release; 403 = rate-limited. All non-fatal.
		return nil, fmt.Errorf("github releases: HTTP %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return nil, err
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return nil, fmt.Errorf("github releases: no tag_name")
	}
	return &rel, nil
}

// isNewer reports whether latest is a strictly higher semver than current.
// Non-parseable inputs (e.g. a "dev" build) return false — we never nag when we
// can't be sure.
func isNewer(latest, current string) bool {
	lv, ok1 := parseSemver(latest)
	cv, ok2 := parseSemver(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if lv[i] != cv[i] {
			return lv[i] > cv[i]
		}
	}
	return false
}

func parseSemver(s string) ([3]int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	// Drop any pre-release/build metadata (X.Y.Z-rc1+meta → X.Y.Z).
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}

// DownloadVerified downloads assetName from the given Status, verifies its SHA256
// against the release's SHA256SUMS asset, and returns the path to the verified
// file in a temp dir. It refuses if either asset is missing or the hash mismatches.
func (s Status) DownloadVerified(ctx context.Context, client *http.Client, assetName string) (string, error) {
	assetURL, ok := s.Assets[assetName]
	if !ok {
		return "", fmt.Errorf("release has no asset %q", assetName)
	}
	sumsURL, ok := s.Assets["SHA256SUMS"]
	if !ok {
		return "", fmt.Errorf("release has no SHA256SUMS (cannot verify)")
	}

	sums, err := fetchText(ctx, client, sumsURL)
	if err != nil {
		return "", fmt.Errorf("fetch checksums: %w", err)
	}
	want := sumFor(sums, assetName)
	if want == "" {
		return "", fmt.Errorf("no checksum for %q in SHA256SUMS", assetName)
	}

	dir, err := os.MkdirTemp("", "am-update-")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(dir, assetName)
	got, err := downloadToFile(ctx, client, assetURL, dst)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(got, want) {
		_ = os.Remove(dst)
		return "", fmt.Errorf("checksum mismatch for %q: got %s want %s", assetName, got, want)
	}
	return dst, nil
}

func fetchText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return string(b), err
}

// sumFor parses a SHA256SUMS file ("<hex>  <filename>" per line) for filename.
func sumFor(sums, filename string) string {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) >= 2 {
			// filename may be the last field, optionally prefixed with '*'.
			name := strings.TrimPrefix(f[len(f)-1], "*")
			if name == filename {
				return f[0]
			}
		}
	}
	return ""
}

func downloadToFile(ctx context.Context, client *http.Client, url, dst string) (string, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	// Cap at 300MB as a sanity bound on a desktop installer/dmg.
	if _, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 300<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
