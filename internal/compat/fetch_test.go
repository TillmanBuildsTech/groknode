package compat

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeZip builds a zip at path from name->content entries and returns its size.
func writeZip(t *testing.T, path string, entries map[string]string) int64 {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("file close: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat zip: %v", err)
	}
	return fi.Size()
}

func TestUnzipExtractsWithinDest(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "a.zip")
	size := writeZip(t, zipPath, map[string]string{
		"electron":           "#!/bin/true\n",
		"resources/app.file": "data",
	})
	dest := filepath.Join(dir, "out")
	if err := unzip(zipPath, size, dest); err != nil {
		t.Fatalf("unzip: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "resources", "app.file")); err != nil || string(b) != "data" {
		t.Fatalf("extracted file mismatch: %q err=%v", b, err)
	}
}

func TestUnzipRejectsZipSlip(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "evil.zip")
	size := writeZip(t, zipPath, map[string]string{
		"../escape.txt": "pwned",
	})
	dest := filepath.Join(dir, "out")
	err := unzip(zipPath, size, dest)
	if err == nil || !strings.Contains(err.Error(), "unsafe zip path") {
		t.Fatalf("expected zip-slip rejection, got %v", err)
	}
	// The traversal target must not have been written.
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("zip-slip wrote outside dest: %v", err)
	}
}
