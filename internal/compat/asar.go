// Package compat reads facts about the installed vendor package and locates a
// compatible Electron runtime, without executing the (hardened) vendor binary.
package compat

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// asarFile is one entry in the asar index.
type asarFile struct {
	Offset   string              `json:"offset"`
	Size     int64               `json:"size"`
	Unpacked bool                `json:"unpacked"`
	Files    map[string]asarFile `json:"files"`
}

type asarHeader struct {
	Files map[string]asarFile `json:"files"`
}

// openAsar parses an asar archive header and returns the index plus the byte
// offset where the file data region begins.
//
// The asar header is nested chromium Pickles:
//
//	[0:4]=u32(4)  [4:8]=u32 headerSize  [8:12]=u32 jsonPickleSize
//	[12:16]=u32 jsonStrLen  [16:16+jsonStrLen]=JSON
//
// The file data region begins at 8 + headerSize (4-byte aligned).
func openAsar(path string) (asarHeader, int64, []byte, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is the config-controlled vendor asar location, not external input
	if err != nil {
		return asarHeader{}, 0, nil, fmt.Errorf("read asar: %w", err)
	}
	if len(data) < 16 {
		return asarHeader{}, 0, nil, fmt.Errorf("asar too small")
	}
	headerSize := binary.LittleEndian.Uint32(data[4:8])
	jsonLen := binary.LittleEndian.Uint32(data[12:16])
	if 16+int(jsonLen) > len(data) {
		return asarHeader{}, 0, nil, fmt.Errorf("asar json length out of range")
	}
	var hdr asarHeader
	if err := json.Unmarshal(data[16:16+jsonLen], &hdr); err != nil {
		return asarHeader{}, 0, nil, fmt.Errorf("parse asar header: %w", err)
	}
	dataOff := int64(8) + int64(headerSize)
	return hdr, dataOff, data, nil
}

// ReadAsarFile returns the bytes of an internal path like "package.json" or
// "dist/local-exec-daemon/main.cjs".
func ReadAsarFile(asarPath, internalPath string) ([]byte, error) {
	hdr, dataOff, raw, err := openAsar(asarPath)
	if err != nil {
		return nil, err
	}
	node := asarFile{Files: hdr.Files}
	for _, part := range strings.Split(strings.Trim(internalPath, "/"), "/") {
		next, ok := node.Files[part]
		if !ok {
			return nil, fmt.Errorf("asar path not found: %s", internalPath)
		}
		node = next
	}
	if node.Files != nil {
		return nil, fmt.Errorf("asar path is a directory: %s", internalPath)
	}
	var off int64
	if _, err := fmt.Sscan(node.Offset, &off); err != nil {
		return nil, fmt.Errorf("bad offset for %s: %w", internalPath, err)
	}
	start := dataOff + off
	end := start + node.Size
	if start < 0 || end > int64(len(raw)) {
		return nil, fmt.Errorf("file region out of range for %s", internalPath)
	}
	return raw[start:end], nil
}

// InstalledVersion reads the vendor app version from app.asar/package.json.
func InstalledVersion(asarPath string) (string, error) {
	b, err := ReadAsarFile(asarPath, "package.json")
	if err != nil {
		return "", err
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &pkg); err != nil {
		return "", fmt.Errorf("parse package.json: %w", err)
	}
	if pkg.Version == "" {
		return "", fmt.Errorf("no version in package.json")
	}
	return pkg.Version, nil
}
