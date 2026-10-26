package compat

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var electronVerRe = regexp.MustCompile(`Electron/(\d+)\.(\d+)\.(\d+)`)

// RequiredElectronMajor scans the vendor Electron binary for its embedded
// "Electron/<major>.<minor>.<patch>" version string and returns the major. We
// cannot ask the binary directly because run_as_node is fused off.
func RequiredElectronMajor(appBinary string) (int, error) {
	f, err := os.Open(appBinary)
	if err != nil {
		return 0, fmt.Errorf("open vendor binary: %w", err)
	}
	defer func() { _ = f.Close() }()

	const chunk = 1 << 20
	buf := make([]byte, chunk+64)
	var carry []byte
	for {
		n, err := f.Read(buf[:chunk])
		if n > 0 {
			window := make([]byte, 0, len(carry)+n)
			window = append(window, carry...)
			window = append(window, buf[:n]...)
			if m := electronVerRe.FindSubmatch(window); m != nil {
				major, _ := strconv.Atoi(string(m[1]))
				return major, nil
			}
			// keep a small tail so a match spanning a chunk boundary is caught
			if len(window) > 64 {
				carry = append(carry[:0], window[len(window)-64:]...)
			} else {
				carry = append(carry[:0], window...)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("scan vendor binary: %w", err)
		}
	}
	return 0, fmt.Errorf("no Electron version string found in %s", appBinary)
}

// RequiredElectronVersion scans the vendor binary for its full embedded
// "Electron/<major>.<minor>.<patch>" version string.
func RequiredElectronVersion(appBinary string) (string, error) {
	f, err := os.Open(appBinary)
	if err != nil {
		return "", fmt.Errorf("open vendor binary: %w", err)
	}
	defer func() { _ = f.Close() }()
	const chunk = 1 << 20
	buf := make([]byte, chunk+64)
	var carry []byte
	for {
		n, err := f.Read(buf[:chunk])
		if n > 0 {
			window := make([]byte, 0, len(carry)+n)
			window = append(window, carry...)
			window = append(window, buf[:n]...)
			if m := electronVerRe.FindSubmatch(window); m != nil {
				return fmt.Sprintf("%s.%s.%s", m[1], m[2], m[3]), nil
			}
			if len(window) > 64 {
				carry = append(carry[:0], window[len(window)-64:]...)
			} else {
				carry = append(carry[:0], window...)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("scan vendor binary: %w", err)
		}
	}
	return "", fmt.Errorf("no Electron version string found in %s", appBinary)
}

// ElectronMajorOf runs `<electron> --version` and parses the major version.
func ElectronMajorOf(electron string) (int, error) {
	out, err := exec.Command(electron, "--version").Output()
	if err != nil {
		return 0, fmt.Errorf("%s --version: %w", electron, err)
	}
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "v"))
	parts := strings.SplitN(v, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("parse electron version %q: %w", string(out), err)
	}
	return major, nil
}

// ElectronCandidates returns the ordered list of paths where a stock Electron
// runtime might live, given the wanted major, an optional override, and the
// managed cache dir.
func ElectronCandidates(wantMajor int, override, cacheDir string) []string {
	var candidates []string
	if override != "" {
		candidates = append(candidates, override)
	}
	if cacheDir != "" {
		candidates = append(candidates,
			filepath.Join(cacheDir, fmt.Sprintf("electron-%d", wantMajor), "electron"),
			filepath.Join(cacheDir, "electron", "electron"),
		)
	}
	candidates = append(candidates,
		fmt.Sprintf("/usr/lib/electron%d/electron", wantMajor),
		"/usr/lib/electron/electron",
		"/usr/bin/electron",
	)
	if p, err := exec.LookPath("electron"); err == nil {
		candidates = append(candidates, p)
	}
	return candidates
}

// FirstExistingElectron returns the first candidate that exists on disk,
// regardless of whether it can run. Useful for lib diagnostics.
func FirstExistingElectron(wantMajor int, override, cacheDir string) string {
	for _, c := range ElectronCandidates(wantMajor, override, cacheDir) {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// FindElectronRuntime locates a stock Electron binary whose major matches
// wantMajor. override (if set) is checked first and must match. Then the managed
// cache dir, distro locations, and PATH are tried. Returns the binary path.
func FindElectronRuntime(wantMajor int, override, cacheDir string) (string, error) {
	candidates := ElectronCandidates(wantMajor, override, cacheDir)
	var tried []string
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err != nil {
			continue
		}
		major, err := ElectronMajorOf(c)
		if err != nil {
			tried = append(tried, fmt.Sprintf("%s (version check failed: %v)", c, err))
			continue
		}
		if wantMajor == 0 || major == wantMajor {
			return c, nil
		}
		tried = append(tried, fmt.Sprintf("%s (Electron %d, want %d)", c, major, wantMajor))
	}
	return "", fmt.Errorf("no compatible Electron %d runtime found; tried: %s", wantMajor, strings.Join(tried, "; "))
}
