// Package claude provides constants and helpers for Claude API integration.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// VersionSyncer periodically fetches the latest Claude CLI and SDK versions
// from the npm registry and updates the runtime version values.
type VersionSyncer struct {
	mu       sync.Mutex
	client   *http.Client
	interval time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	// onChange is called when versions are updated (optional, for logging/notification)
	onChange func(cliVersion, sdkVersion string)
}

// npmPackageInfo is a minimal representation of npm package info.
type npmPackageInfo struct {
	Version string `json:"version"`
}

// NewVersionSyncer creates a new VersionSyncer that checks npm for updates
// at the given interval (recommended: 6 hours).
func NewVersionSyncer(interval time.Duration) *VersionSyncer {
	if interval < time.Minute {
		interval = 6 * time.Hour
	}
	return &VersionSyncer{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		interval: interval,
	}
}

// SetOnChange sets a callback that fires when versions are updated.
func (vs *VersionSyncer) SetOnChange(fn func(cliVersion, sdkVersion string)) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.onChange = fn
}

// Start begins the periodic version sync. Call Stop() to clean up.
func (vs *VersionSyncer) Start() {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.cancel != nil {
		return // already running
	}

	ctx, cancel := context.WithCancel(context.Background())
	vs.cancel = cancel
	vs.done = make(chan struct{})

	go vs.run(ctx)
}

// Stop halts the periodic sync.
func (vs *VersionSyncer) Stop() {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if vs.cancel != nil {
		vs.cancel()
		<-vs.done
		vs.cancel = nil
	}
}

func (vs *VersionSyncer) run(ctx context.Context) {
	defer close(vs.done)

	// Sync immediately on start
	vs.syncOnce(ctx)

	ticker := time.NewTicker(vs.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			vs.syncOnce(ctx)
		}
	}
}

func (vs *VersionSyncer) syncOnce(ctx context.Context) {
	cliVersion, err := vs.fetchNPMVersion(ctx, "@anthropic-ai/claude-code")
	if err != nil {
		// Silently continue with current version
		return
	}

	sdkVersion, err := vs.fetchNPMVersion(ctx, "@anthropic-ai/sdk")
	if err != nil {
		// Still update CLI version even if SDK fetch fails
		sdkVersion = ""
	}

	// Only update if we got valid semver-like versions
	if cliVersion != "" && isSemver(cliVersion) {
		oldCLI := GetCurrentCLIVersion()
		oldSDK := GetCurrentSDKVersion()

		UpdateVersions(cliVersion, sdkVersion, "")

		if cliVersion != oldCLI || (sdkVersion != "" && sdkVersion != oldSDK) {
			vs.mu.Lock()
			fn := vs.onChange
			vs.mu.Unlock()
			if fn != nil {
				fn(cliVersion, sdkVersion)
			}
		}
	}
}

// fetchNPMVersion fetches the latest version of an npm package.
func (vs *VersionSyncer) fetchNPMVersion(ctx context.Context, packageName string) (string, error) {
	// Use abbreviated metadata endpoint for minimal response size
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", packageName)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := vs.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry returned status %d for %s", resp.StatusCode, packageName)
	}

	// Read at most 64KB
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}

	var info npmPackageInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return "", err
	}

	return info.Version, nil
}

var semverRegex = regexp.MustCompile(`^\d+\.\d+\.\d+`)

// isSemver checks if a string looks like a semantic version (x.y.z...)
func isSemver(v string) bool {
	return semverRegex.MatchString(strings.TrimPrefix(v, "v"))
}
