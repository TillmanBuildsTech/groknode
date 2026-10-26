package compat

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
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
	if err := os.MkdirAll(dest, 0o755); err != nil {
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
	if err := os.Chmod(bin, 0o755); err != nil {
		return "", err
	}
	return bin, nil
}

func unzip(zipPath string, size int64, dest string) error {
	f, err := os.Open(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	zr, err := zip.NewReader(f, size)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	for _, zf := range zr.File {
		target := filepath.Join(dest, zf.Name) //nolint:gosec // dest is ours; see guard below
		// Zip-slip guard.
		if rel, err := filepath.Rel(dest, target); err != nil || len(rel) >= 2 && rel[0] == '.' && rel[1] == '.' {
			return fmt.Errorf("unsafe zip path: %s", zf.Name)
		}
		if zf.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		mode := zf.Mode()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil { //nolint:gosec // trusted vendor release
			_ = rc.Close()
			_ = out.Close()
			return err
		}
		_ = rc.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}
