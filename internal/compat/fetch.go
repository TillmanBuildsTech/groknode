package compat

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Extraction bounds, guarding against decompression bombs. A stock Electron
// linux zip is a few hundred MB; these caps are generous but finite.
const (
	maxFileBytes  = 1 << 30 // 1 GiB per entry
	maxTotalBytes = 4 << 30 // 4 GiB across the whole archive
)

// ElectronReleaseURL returns the official stock Electron linux-x64 zip URL for a
// full version like "42.11.16".
func ElectronReleaseURL(version string) string {
	return fmt.Sprintf("https://github.com/electron/electron/releases/download/v%s/electron-v%s-linux-x64.zip", version, version)
}

// FetchElectron downloads and extracts the stock Electron matching version into
// cacheDir/electron-<major>/, returning the path to the electron binary. Stock
// Electron ships with the run_as_node fuse ENABLED, unlike the vendor binary.
func FetchElectron(version string, major int, cacheDir string) (string, error) {
	dest := filepath.Join(cacheDir, fmt.Sprintf("electron-%d", major))
	bin := filepath.Join(dest, "electron")
	if _, err := os.Stat(bin); err == nil {
		return bin, nil // already present
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return "", err
	}

	url := ElectronReleaseURL(version)
	tmp, err := os.CreateTemp(cacheDir, "electron-*.zip")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	size, err := io.Copy(tmp, resp.Body)
	if err != nil {
		return "", fmt.Errorf("write zip: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	if err := unzip(tmp.Name(), size, dest); err != nil {
		return "", err
	}
	if _, err := os.Stat(bin); err != nil {
		return "", fmt.Errorf("electron binary missing after extract: %w", err)
	}
	// The electron binary must be executable; 0o750 keeps it off world access.
	if err := os.Chmod(bin, 0o750); err != nil { // #nosec G302 -- runtime binary must carry the exec bit
		return "", err
	}
	return bin, nil
}

func unzip(zipPath string, size int64, dest string) error {
	f, err := os.Open(zipPath) // #nosec G304 -- zipPath is our own temp file created above
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zr, err := zip.NewReader(f, size)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	var total int64
	for _, zf := range zr.File {
		// Zip-slip guard: the resolved path must stay within dest.
		target := filepath.Join(dest, zf.Name) // #nosec G305 -- validated to stay within dest immediately below
		rel, err := filepath.Rel(dest, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe zip path: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		n, err := extractFile(zf, target)
		if err != nil {
			return err
		}
		total += n
		if total > maxTotalBytes {
			return fmt.Errorf("archive exceeds %d bytes; refusing to extract", maxTotalBytes)
		}
	}
	return nil
}

// extractFile writes a single zip entry to target, bounded by maxFileBytes, and
// returns the number of bytes written.
func extractFile(zf *zip.File, target string) (int64, error) {
	rc, err := zf.Open()
	if err != nil {
		return 0, err
	}
	defer func() { _ = rc.Close() }()

	// Drop setuid/setgid/sticky and world/group-write bits from the archived
	// mode; keep the exec bit so Electron's helper binaries stay runnable.
	mode := zf.Mode().Perm() & 0o755
	if mode == 0 {
		mode = 0o644
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode) // #nosec G304 -- target is validated against dest by the zip-slip guard
	if err != nil {
		return 0, err
	}
	// Bound the copy so a crafted entry cannot exhaust disk (decompression bomb).
	n, err := io.Copy(out, io.LimitReader(rc, maxFileBytes+1))
	if err != nil {
		_ = out.Close()
		return 0, err
	}
	if err := out.Close(); err != nil {
		return 0, err
	}
	if n > maxFileBytes {
		return 0, fmt.Errorf("zip entry %s exceeds %d bytes; refusing to extract", zf.Name, maxFileBytes)
	}
	return n, nil
}
