package compat

import (
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

var lddNotFoundRe = regexp.MustCompile(`(?m)^\s*(\S+)\s*=>\s*not found`)

// MissingSharedLibs runs ldd on an Electron binary and returns the sonames of
// libraries the dynamic loader cannot resolve. The Electron binary links
// GTK/X11/NSS at load even in run_as_node mode, so these must exist on disk
// (no display server needed, just the libraries). Returns nil if ldd is
// unavailable (cannot diagnose) or nothing is missing.
func MissingSharedLibs(binary string) []string {
	ldd, err := exec.LookPath("ldd")
	if err != nil {
		return nil
	}
	// ldd exits non-zero when libs are missing; we still want its stdout.
	out, _ := exec.Command(ldd, binary).CombinedOutput()
	seen := map[string]bool{}
	for _, m := range lddNotFoundRe.FindAllStringSubmatch(string(out), -1) {
		seen[m[1]] = true
	}
	var missing []string
	for s := range seen {
		missing = append(missing, s)
	}
	sort.Strings(missing)
	return missing
}

// soname prefix -> suggested packages per distro family.
var libPackageHints = []struct {
	prefix string
	apt    string
	pacman string
}{
	{"libgtk-3", "libgtk-3-0", "gtk3"},
	{"libgtk-x11", "libgtk2.0-0", "gtk2"},
	{"libnss3", "libnss3", "nss"},
	{"libnssutil3", "libnss3", "nss"},
	{"libsmime3", "libnss3", "nss"},
	{"libatk-1", "libatk1.0-0", "atk"},
	{"libatk-bridge", "libatk-bridge2.0-0", "at-spi2-atk"},
	{"libatspi", "libatspi2.0-0", "at-spi2-core"},
	{"libgbm", "libgbm1", "mesa"},
	{"libdrm", "libdrm2", "libdrm"},
	{"libasound", "libasound2", "alsa-lib"},
	{"libcups", "libcups2", "libcups"},
	{"libxkbcommon", "libxkbcommon0", "libxkbcommon"},
	{"libpango", "libpango-1.0-0", "pango"},
	{"libcairo", "libcairo2", "cairo"},
	{"libX", "libx11-6 libxext6 libxrandr2 libxdamage1 libxcomposite1 libxfixes3 libxtst6 libxcb1", "libx11 libxext libxrandr libxdamage libxcomposite libxfixes libxtst libxcb"},
	{"libexpat", "libexpat1", "expat"},
	{"libglib", "libglib2.0-0", "glib2"},
	{"libdbus", "libdbus-1-3", "dbus"},
}

// PackageHints maps missing sonames to a deduplicated set of suggested packages
// for apt- and pacman-based distros.
func PackageHints(missing []string) (apt, pacman []string) {
	aptSet, pacSet := map[string]bool{}, map[string]bool{}
	for _, so := range missing {
		for _, h := range libPackageHints {
			if strings.HasPrefix(so, h.prefix) {
				for _, p := range strings.Fields(h.apt) {
					aptSet[p] = true
				}
				for _, p := range strings.Fields(h.pacman) {
					pacSet[p] = true
				}
			}
		}
	}
	for p := range aptSet {
		apt = append(apt, p)
	}
	for p := range pacSet {
		pacman = append(pacman, p)
	}
	sort.Strings(apt)
	sort.Strings(pacman)
	return apt, pacman
}
