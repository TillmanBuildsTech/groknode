//go:build nightly

// Nightly smoke test against the LATEST Grok Bot release. The workflow resolves
// the current version from Cursor's update feed, extracts the package, and
// points GROK_BOT_BINARY / GROK_BOT_ASAR at it. Unlike the guarded integration
// tests (which t.Skip when the vendor package is absent), this test FAILS when
// an expected path or structure is missing — so a packaging change from Cursor
// is caught instead of silently passing.
package compat

import (
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

var semverRe = regexp.MustCompile(`^\d+\.\d+\.\d+`)

func TestLatestPackageStructure(t *testing.T) {
	appBinary := envOr("GROK_BOT_BINARY", "/opt/Grok Bot/grok-bot")
	asar := envOr("GROK_BOT_ASAR", "/opt/Grok Bot/resources/app.asar")

	// 1. The package extracted to the expected layout.
	if _, err := os.Stat(appBinary); err != nil {
		t.Fatalf("vendor binary missing at %q: %v -- Cursor may have changed the package layout", appBinary, err)
	}
	if _, err := os.Stat(asar); err != nil {
		t.Fatalf("app.asar missing at %q: %v -- Cursor may have changed resources layout", asar, err)
	}

	// 2. Version is readable from app.asar/package.json.
	ver, err := InstalledVersion(asar)
	if err != nil {
		t.Fatalf("InstalledVersion: %v", err)
	}
	if !semverRe.MatchString(ver) {
		t.Fatalf("installed version not semver-like: %q", ver)
	}
	t.Logf("installed Grok Bot version: %s", ver)

	// 3. The daemon entry we bootstrap over IPC still exists inside the asar.
	daemon, err := ReadAsarFile(asar, "dist/local-exec-daemon/main.cjs")
	if err != nil {
		t.Fatalf("daemon entry dist/local-exec-daemon/main.cjs missing: %v -- the path we launch and bootstrap", err)
	}
	if len(daemon) < 1000 {
		t.Fatalf("daemon main.cjs suspiciously small: %d bytes", len(daemon))
	}
	t.Logf("daemon main.cjs: %d bytes", len(daemon))

	// 4. The embedded Electron version is still discoverable by byte-scan (we
	//    cannot exec the fused binary), and determines the ABI we must match.
	major, err := RequiredElectronMajor(appBinary)
	if err != nil {
		t.Fatalf("RequiredElectronMajor: %v -- no embedded Electron/x.y.z string in vendor binary", err)
	}
	if major <= 0 {
		t.Fatalf("implausible Electron major: %d", major)
	}
	full, err := RequiredElectronVersion(appBinary)
	if err != nil {
		t.Fatalf("RequiredElectronVersion: %v", err)
	}
	if !semverRe.MatchString(full) {
		t.Fatalf("electron version not semver-like: %q", full)
	}
	t.Logf("required stock Electron: v%s (major %d)", full, major)

	// 5. The core premise: a matching stock Electron of that exact version must
	//    still be published upstream (groknode fetch-runtime downloads it).
	assertElectronPublished(t, full)
}

func assertElectronPublished(t *testing.T, version string) {
	t.Helper()
	url := ElectronReleaseURL(version)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request for %s: %v", url, err)
	}
	// Ask for a single byte instead of the ~100MB asset; GitHub release assets
	// honor Range and reply 206, or 200 if ignored.
	req.Header.Set("Range", "bytes=0-0")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("matching stock Electron v%s not downloadable: HTTP %d (%s)", version, resp.StatusCode, url)
	}
	t.Logf("matching stock Electron available: %s", url)
}
