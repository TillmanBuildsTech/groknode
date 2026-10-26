package compat

import (
	"os"
	"regexp"
	"testing"
)

const realAsar = "/opt/Grok Bot/resources/app.asar"

// Guarded integration test: runs only where the real vendor package is present.
func TestReadRealAsar(t *testing.T) {
	if _, err := os.Stat(realAsar); err != nil {
		t.Skip("vendor app.asar not installed")
	}
	ver, err := InstalledVersion(realAsar)
	if err != nil {
		t.Fatalf("InstalledVersion: %v", err)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(ver) {
		t.Fatalf("version does not look like semver: %q", ver)
	}
	t.Logf("installed version: %s", ver)

	b, err := ReadAsarFile(realAsar, "dist/local-exec-daemon/main.cjs")
	if err != nil {
		t.Fatalf("read daemon entry: %v", err)
	}
	if len(b) < 1000 {
		t.Fatalf("daemon entry suspiciously small: %d bytes", len(b))
	}
	t.Logf("daemon main.cjs: %d bytes", len(b))
}
