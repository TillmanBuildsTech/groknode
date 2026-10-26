package compat

import (
	"slices"
	"testing"
)

func TestPackageHints(t *testing.T) {
	apt, pac := PackageHints([]string{"libgtk-3.so.0", "libnss3.so", "libgbm.so.1"})
	for _, want := range []string{"libgtk-3-0", "libnss3", "libgbm1"} {
		if !slices.Contains(apt, want) {
			t.Errorf("apt hints missing %q: %v", want, apt)
		}
	}
	for _, want := range []string{"gtk3", "nss", "mesa"} {
		if !slices.Contains(pac, want) {
			t.Errorf("pacman hints missing %q: %v", want, pac)
		}
	}
}

func TestPackageHintsUnknownSoname(t *testing.T) {
	apt, pac := PackageHints([]string{"libtotallymadeup.so.9"})
	if len(apt) != 0 || len(pac) != 0 {
		t.Errorf("expected no hints for unknown soname, got apt=%v pac=%v", apt, pac)
	}
}
