// Package config resolves all paths, URLs, and runtime locations from env with
// sane defaults, supporting both user-mode and system-service layouts.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultAPIURL     = "https://api2.cursor.sh"
	defaultWebsiteURL = "https://cursor.com"
	defaultClientID   = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB"
	defaultAppBinary  = "/opt/Grok Bot/grok-bot"
	defaultDaemon     = "/opt/Grok Bot/resources/app.asar/dist/local-exec-daemon/main.cjs"
)

// Config holds resolved runtime configuration.
type Config struct {
	APIURL       string
	WebsiteURL   string
	AuthClientID string

	AppBinary    string // vendor Electron binary (code source only; NOT executed)
	DaemonScript string // path to dist/local-exec-daemon/main.cjs
	AsarPath     string // app.asar path (for version/compat reads)

	ElectronOverride string // GROKBOT_ELECTRON, explicit runtime path
	RuntimeCacheDir  string // where managed stock Electron is fetched

	ConfigDir string
	DataRoot  string
}

func env(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// Load resolves configuration from the environment.
func Load() (*Config, error) {
	home, _ := os.UserHomeDir()

	cfgDir := env("GROKBOT_CONFIG_DIR", "GROK_BOT_HEADLESS_CONFIG_DIR")
	if cfgDir == "" {
		cfgDir = filepath.Join(home, ".config", "groknode")
	}
	dataRoot := env("GROKBOT_DATA_ROOT", "SAND_DATA_ROOT")
	if dataRoot == "" {
		dataRoot = filepath.Join(home, ".local", "share", "groknode")
	}

	daemon := env("GROK_BOT_DAEMON_SCRIPT")
	if daemon == "" {
		daemon = defaultDaemon
	}
	asar := env("GROK_BOT_ASAR")
	if asar == "" {
		if i := strings.Index(daemon, ".asar/"); i >= 0 {
			asar = daemon[:i+len(".asar")]
		} else {
			asar = "/opt/Grok Bot/resources/app.asar"
		}
	}

	api, err := validateServiceURL(firstNonEmpty(env("SAND_BACKEND_URL", "CURSOR_API_BASE_URL"), defaultAPIURL), "backend")
	if err != nil {
		return nil, err
	}
	web, err := validateServiceURL(firstNonEmpty(env("SAND_CURSOR_WEBSITE_URL", "CURSOR_WEBSITE_URL"), defaultWebsiteURL), "website")
	if err != nil {
		return nil, err
	}

	cacheDir := env("GROKBOT_RUNTIME_CACHE")
	if cacheDir == "" {
		cacheDir = filepath.Join(dataRoot, "runtime")
	}

	return &Config{
		APIURL:           api,
		WebsiteURL:       web,
		AuthClientID:     firstNonEmpty(env("SAND_AUTH_CLIENT_ID"), defaultClientID),
		AppBinary:        firstNonEmpty(env("GROK_BOT_BINARY"), defaultAppBinary),
		DaemonScript:     daemon,
		AsarPath:         asar,
		ElectronOverride: env("GROKBOT_ELECTRON"),
		RuntimeCacheDir:  cacheDir,
		ConfigDir:        cfgDir,
		DataRoot:         dataRoot,
	}, nil
}

// Derived file paths. Names mirror the daemon's expectations where they are
// shared (the daemon reads the credential descriptor and writes discovery).
func (c *Config) CredentialsPath() string { return filepath.Join(c.ConfigDir, "credentials.json") }
func (c *Config) MachinePath() string     { return filepath.Join(c.ConfigDir, "machine.json") }
func (c *Config) DescriptorPath() string {
	return filepath.Join(c.DataRoot, "local-exec-daemon-credential.json")
}
func (c *Config) HeartbeatPath() string {
	return filepath.Join(c.DataRoot, "local-exec-supervisor.json")
}
func (c *Config) DiscoveryPath() string { return filepath.Join(c.DataRoot, "local-exec-daemon.json") }
func (c *Config) SettingsPath() string  { return filepath.Join(c.DataRoot, "settings.json") }

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func validateServiceURL(value, label string) (string, error) {
	u, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid %s URL", label)
	}
	if u.User != nil {
		return "", fmt.Errorf("%s URL cannot contain credentials", label)
	}
	insecureOK := os.Getenv("GROK_BOT_ALLOW_INSECURE_LOCALHOST") == "1" &&
		u.Scheme == "http" &&
		(u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
	if u.Scheme != "https" && !insecureOK {
		return "", fmt.Errorf("%s URL must use HTTPS", label)
	}
	return strings.TrimRight(u.String(), "/"), nil
}
